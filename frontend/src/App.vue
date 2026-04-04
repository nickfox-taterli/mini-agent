<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref } from 'vue'
import { Marked } from 'marked'
import hljs from 'highlight.js/lib/core'
import DOMPurify from 'dompurify'
import 'highlight.js/styles/github-dark.css'

// 注册常用语言, 控制 bundle 大小
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import python from 'highlight.js/lib/languages/python'
import css from 'highlight.js/lib/languages/css'
import xml from 'highlight.js/lib/languages/xml'
import bash from 'highlight.js/lib/languages/bash'
import json from 'highlight.js/lib/languages/json'
import sql from 'highlight.js/lib/languages/sql'
import go from 'highlight.js/lib/languages/go'
import java from 'highlight.js/lib/languages/java'
import c from 'highlight.js/lib/languages/c'
import cpp from 'highlight.js/lib/languages/cpp'
import yaml from 'highlight.js/lib/languages/yaml'
import markdown from 'highlight.js/lib/languages/markdown'
import diff from 'highlight.js/lib/languages/diff'
import shell from 'highlight.js/lib/languages/shell'

hljs.registerLanguage('javascript', javascript)
hljs.registerLanguage('js', javascript)
hljs.registerLanguage('typescript', typescript)
hljs.registerLanguage('ts', typescript)
hljs.registerLanguage('python', python)
hljs.registerLanguage('py', python)
hljs.registerLanguage('css', css)
hljs.registerLanguage('xml', xml)
hljs.registerLanguage('html', xml)
hljs.registerLanguage('bash', bash)
hljs.registerLanguage('shell', shell)
hljs.registerLanguage('json', json)
hljs.registerLanguage('sql', sql)
hljs.registerLanguage('go', go)
hljs.registerLanguage('java', java)
hljs.registerLanguage('c', c)
hljs.registerLanguage('cpp', cpp)
hljs.registerLanguage('yaml', yaml)
hljs.registerLanguage('yml', yaml)
hljs.registerLanguage('markdown', markdown)
hljs.registerLanguage('md', markdown)
hljs.registerLanguage('diff', diff)

// DOMPurify 全局 hook: 为所有链接添加安全属性
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A') {
    node.setAttribute('rel', 'noopener noreferrer')
    node.setAttribute('target', '_blank')
  }
})

// 自定义 renderer: 代码块包含语言标签 + Copy 按钮
const renderer = {
  code({ text, lang }) {
    const language = lang || 'text'
    let highlighted
    if (lang && hljs.getLanguage(lang)) {
      highlighted = hljs.highlight(text, { language: lang }).value
    } else {
      highlighted = hljs.highlightAuto(text).value
    }
    return `<div class="code-block-wrapper">
  <div class="code-block-header">
    <span class="code-lang">${language}</span>
    <button class="copy-btn">Copy</button>
  </div>
  <pre><code class="hljs language-${language}">${highlighted}</code></pre>
</div>`
  }
}

const marked = new Marked({
  renderer,
  gfm: true,
  breaks: false
})

function renderMarkdown(text) {
  if (!text) return ''
  const rawHtml = marked.parse(text)
  return DOMPurify.sanitize(rawHtml, {
    ALLOWED_TAGS: [
      'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'p', 'br', 'hr', 'blockquote',
      'ul', 'ol', 'li', 'a', 'strong', 'em', 'del', 'code', 'pre',
      'table', 'thead', 'tbody', 'tr', 'th', 'td', 'img', 'span', 'div',
      'sup', 'sub', 'details', 'summary', 'button', 's'
    ],
    ALLOWED_ATTR: [
      'href', 'src', 'alt', 'title', 'class', 'target', 'rel',
      'data-language', 'data-lang'
    ]
  })
}

const apiBase = import.meta.env.VITE_API_BASE || 'http://127.0.0.1:18888'

const input = ref('')
const messages = ref([])
const loading = ref(false)
const backends = ref([])
const selectedBackendId = ref('')
const textareaRef = ref(null)
const expandedThinking = reactive(new Map())

// 实时计时器
const now = ref(Date.now())
let timerInterval = null

function startTimer() {
  now.value = Date.now()
  timerInterval = setInterval(() => { now.value = Date.now() }, 100)
}

function stopTimer() {
  if (timerInterval) {
    clearInterval(timerInterval)
    timerInterval = null
  }
}

onUnmounted(() => { stopTimer() })

function formatDuration(ms) {
  const totalSeconds = ms / 1000
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (minutes > 0) {
    return `${minutes} 分钟 ${seconds.toFixed(2)} 秒`
  }
  return `${seconds.toFixed(2)} 秒`
}

function getThinkingDuration(msg) {
  if (msg.thinkingDuration != null) {
    return formatDuration(msg.thinkingDuration * 1000)
  }
  if (msg.thinkingStartTime && loading.value) {
    return formatDuration(now.value - msg.thinkingStartTime)
  }
  return '...'
}

const canSend = computed(() => input.value.trim() !== '' && !loading.value)

function autoResize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const lineHeight = parseFloat(getComputedStyle(el).lineHeight) || 24
  const maxHeight = lineHeight * 10 + 4
  el.style.height = Math.min(el.scrollHeight, maxHeight) + 'px'
}

function toggleThinking(idx) {
  expandedThinking.set(idx, !expandedThinking.get(idx))
}

function isThinkingExpanded(idx) {
  return expandedThinking.get(idx) === true
}

onMounted(async () => {
  try {
    const res = await fetch(`${apiBase}/api/backends`)
    if (!res.ok) return
    const data = await res.json()
    backends.value = data.backends || []
    const enabled = backends.value.find((b) => b.enabled)
    if (enabled) {
      selectedBackendId.value = enabled.id
    }
  } catch {
    // Ignore initial backend loading errors to keep UI minimal.
  }
})

function toChatMessages() {
  return messages.value
    .filter((m) => m.role === 'user' || (m.role === 'assistant' && m.content.trim() !== ''))
    .map((m) => ({ role: m.role, content: m.content }))
}

function parseSSEBlock(block) {
  const lines = block.split('\n')
  let event = 'message'
  const dataLines = []

  for (const line of lines) {
    if (line.startsWith('event:')) {
      event = line.slice(6).trim()
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trim())
    }
  }

  const raw = dataLines.join('\n')
  if (!raw) {
    return { event, payload: null }
  }

  try {
    return { event, payload: JSON.parse(raw) }
  } catch {
    return { event, payload: { message: raw } }
  }
}

function takeNextSSEBlock(buffer) {
  const normalized = buffer.replace(/\r\n/g, '\n')
  const separatorIndex = normalized.indexOf('\n\n')
  if (separatorIndex < 0) {
    return { block: null, rest: normalized }
  }

  return {
    block: normalized.slice(0, separatorIndex),
    rest: normalized.slice(separatorIndex + 2)
  }
}

// 判断用户是否在页面底部附近
function isNearBottom() {
  const threshold = 120
  return window.innerHeight + window.scrollY >= document.body.scrollHeight - threshold
}

// 只在用户已到底部时自动滚动
async function scrollToBottom() {
  await nextTick()
  if (isNearBottom()) {
    window.scrollTo(0, document.body.scrollHeight)
  }
}

async function copyCode(event) {
  const btn = event.target.closest('.copy-btn')
  if (!btn) return
  const wrapper = btn.closest('.code-block-wrapper')
  if (!wrapper) return
  const code = wrapper.querySelector('code')
  if (!code) return
  try {
    await navigator.clipboard.writeText(code.textContent)
    btn.textContent = 'Copied!'
    setTimeout(() => { btn.textContent = 'Copy' }, 2000)
  } catch {
    // clipboard API 可能被浏览器阻止
  }
}

async function sendMessage() {
  if (!canSend.value) return

  const userText = input.value.trim()
  input.value = ''
  await nextTick()
  if (textareaRef.value) textareaRef.value.style.height = 'auto'

  messages.value.push({ role: 'user', content: userText })
  const assistant = reactive({
    role: 'assistant',
    reasoning: '',
    content: '',
    thinkingDuration: null,
    thinkingStartTime: Date.now(),
    reasoningDone: false
  })
  messages.value.push(assistant)
  // 新消息时总是滚到底部 (用户刚发送)
  await nextTick()
  window.scrollTo(0, document.body.scrollHeight)

  loading.value = true
  startTimer()

  try {
    const res = await fetch(`${apiBase}/api/chat/stream`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        backend_id: selectedBackendId.value || undefined,
        messages: toChatMessages()
      })
    })

    if (!res.ok || !res.body) {
      throw new Error(`HTTP ${res.status}`)
    }

    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''
    let doneReceived = false

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })

      while (true) {
        const { block, rest } = takeNextSSEBlock(buffer)
        if (block === null) {
          buffer = rest
          break
        }
        buffer = rest

        const { event, payload } = parseSSEBlock(block)
        if (!payload) continue

        if (event === 'reasoning') {
          assistant.reasoning += payload.delta || ''
          await scrollToBottom()
        }

        if (event === 'content') {
          assistant.reasoningDone = true
          assistant.content += payload.delta || ''
          await scrollToBottom()
        }

        if (event === 'error') {
          const msg = payload.message || 'stream error'
          assistant.content += `\n[Error] ${msg}`
          assistant.reasoningDone = true
          doneReceived = true
          assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
          await scrollToBottom()
          break
        }

        if (event === 'done') {
          assistant.reasoningDone = true
          assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
          doneReceived = true
          break
        }
      }

      if (doneReceived) {
        await reader.cancel()
        break
      }
    }
  } catch (err) {
    assistant.content += `\n[Error] ${err.message || 'request failed'}`
    assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
  } finally {
    stopTimer()
    loading.value = false
    await scrollToBottom()
  }
}
</script>

<template>
  <div class="page">
    <div class="panel">
      <div class="chat-area">
        <div v-for="(msg, idx) in messages" :key="idx" class="msg" :class="`msg-${msg.role}`">
          <template v-if="msg.role === 'user'">
            <div class="user-bubble markdown-body" v-html="renderMarkdown(msg.content)"></div>
          </template>
          <template v-else>
            <div class="assistant-message">
              <!-- 思考块: 可折叠, 实时计时 -->
              <div v-if="msg.reasoning" class="thinking-block">
                <button class="thinking-toggle" @click="toggleThinking(idx)">
                  <span class="thinking-chevron" :class="{ expanded: isThinkingExpanded(idx) }">&#9654;</span>
                  <span class="thinking-label">
                    Thought for {{ getThinkingDuration(msg) }}
                  </span>
                </button>
                <div v-show="isThinkingExpanded(idx)" class="thinking-content markdown-body" v-html="renderMarkdown(msg.reasoning)"></div>
              </div>
              <!-- 回答内容: 思考完成后才显示 -->
              <div
                v-if="msg.reasoningDone || !msg.reasoning"
                class="markdown-body"
                v-html="renderMarkdown(msg.content)"
                @click="copyCode"
              ></div>
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>

  <!-- 固定底部输入栏 -->
  <div class="bottom-bar">
    <div class="panel">
      <div class="composer">
        <textarea
          ref="textareaRef"
          v-model="input"
          class="input"
          placeholder="尽管问..."
          rows="1"
          :disabled="loading"
          @input="autoResize"
          @keydown.enter.exact.prevent="sendMessage"
        />
        <button class="send" :disabled="!canSend" @click="sendMessage">
          {{ loading ? '...' : '↑' }}
        </button>
      </div>
      <div class="meta">
        后端: {{ selectedBackendId || '未连接' }} | API: {{ apiBase }}
      </div>
    </div>
  </div>
</template>
