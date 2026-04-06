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

// 文件上传状态
const fileInputRef = ref(null)
const uploading = ref(false)
const attachedFile = ref(null) // { name, url }
const isDragging = ref(false)

// 对话持久化
const conversations = ref([])
const currentConversationId = ref(null)
const sidebarOpen = ref(true)

const STORAGE_KEY_CONVERSATIONS = 'agent-chat-conversations'
const STORAGE_KEY_SIDEBAR = 'agent-chat-sidebar-open'

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

onUnmounted(() => {
  stopTimer()
  document.removeEventListener('keydown', handleKeydown)
})

// ===== 对话持久化 =====

function loadConversations() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_CONVERSATIONS)
    if (raw) conversations.value = JSON.parse(raw)
  } catch {
    conversations.value = []
  }
}

function saveConversations() {
  try {
    localStorage.setItem(STORAGE_KEY_CONVERSATIONS, JSON.stringify(conversations.value))
  } catch {
    // localStorage 可能已满
  }
}

function loadSidebarState() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_SIDEBAR)
    if (raw !== null) sidebarOpen.value = JSON.parse(raw)
  } catch {
    sidebarOpen.value = true
  }
}

function saveSidebarState() {
  try {
    localStorage.setItem(STORAGE_KEY_SIDEBAR, JSON.stringify(sidebarOpen.value))
  } catch {
    // 忽略
  }
}

function generateId() {
  return `conv-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function extractTitle(msgs) {
  const first = msgs.find(m => m.role === 'user')
  if (!first) return '新对话'
  const text = first.content.trim().replace(/\n/g, ' ')
  return text.length > 30 ? text.slice(0, 30) + '...' : text
}

function createNewConversation() {
  // 如果当前对话仍为空, 直接复用, 不再创建
  const current = conversations.value.find(c => c.id === currentConversationId.value)
  if (current && current.messages.length === 0) return
  // 先保存当前对话
  saveCurrentMessages()
  const conv = {
    id: generateId(),
    title: '新对话',
    messages: [],
    createdAt: Date.now()
  }
  conversations.value.unshift(conv)
  currentConversationId.value = conv.id
  messages.value = []
  expandedThinking.clear()
  saveConversations()
}

function switchConversation(id) {
  if (id === currentConversationId.value) return
  saveCurrentMessages()
  currentConversationId.value = id
  const conv = conversations.value.find(c => c.id === id)
  if (conv) {
    messages.value = conv.messages.map(m => {
      if (m.role === 'assistant') {
        return reactive({ ...m, thinkingStartTime: null })
      }
      return { ...m }
    })
    expandedThinking.clear()
  }
}

function saveCurrentMessages() {
  if (!currentConversationId.value) return
  const conv = conversations.value.find(c => c.id === currentConversationId.value)
  if (!conv) return
  conv.messages = messages.value.map(m => ({
    role: m.role,
    content: m.content,
    reasoning: m.reasoning || '',
    reasoningDone: m.reasoningDone || false,
    thinkingDuration: m.thinkingDuration || null
  }))
  // 标题仍为默认值时先用首条消息做临时标题
  if (conv.title === '新对话' || !conv.title) {
    conv.title = extractTitle(conv.messages)
  }
  saveConversations()
}

// 异步调用后端模型生成对话标题
async function generateTitleAsync(convId) {
  const conv = conversations.value.find(c => c.id === convId)
  if (!conv) return
  // 只在标题仍是临时标题时才生成
  if (conv.title !== extractTitle(conv.messages)) return
  // 至少需要一条用户消息
  if (!conv.messages.some(m => m.role === 'user')) return

  try {
    const titleMessages = [
      {
        role: 'system',
        content: '你是一个标题生成器. 请根据用户的消息内容, 用中文生成一个简短的对话标题 (不超过20个字). 只输出标题文本, 不要加引号, 不要加句号, 不要加任何额外说明.'
      },
      {
        role: 'user',
        content: conv.messages
          .filter(m => m.role === 'user')
          .slice(0, 2)
          .map(m => m.content)
          .join('\n')
      }
    ]

    const res = await fetch(`${apiBase}/api/chat/stream`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        backend_id: selectedBackendId.value || undefined,
        messages: titleMessages
      })
    })

    if (!res.ok || !res.body) return

    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''
    let title = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      while (true) {
        const { block, rest } = takeNextSSEBlock(buffer)
        if (block === null) { buffer = rest; break }
        buffer = rest
        const { event, payload } = parseSSEBlock(block)
        if (!payload) continue
        if (event === 'content') {
          title += payload.delta || ''
        }
        if (event === 'done' || event === 'error') {
          await reader.cancel()
          break
        }
      }
    }

    title = title.trim().replace(/^["""']|["""']$/g, '').replace(/\n/g, ' ')
    if (title && title.length > 0 && title.length <= 50) {
      // 再次确认对话仍存在且标题没被用户手动改过
      const target = conversations.value.find(c => c.id === convId)
      if (target) {
        target.title = title
        saveConversations()
      }
    }
  } catch {
    // 标题生成失败不影响主流程
  }
}

function toggleSidebar() {
  sidebarOpen.value = !sidebarOpen.value
  saveSidebarState()
}

const sortedConversations = computed(() => {
  return [...conversations.value].sort((a, b) => b.createdAt - a.createdAt)
})

function handleKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault()
    createNewConversation()
  }
}

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

const canSend = computed(() => (input.value.trim() !== '' || attachedFile.value !== null) && !loading.value)
const isLoading = computed(() => loading.value)

// 用于中断流式请求的 AbortController
let streamController = null

function stopGeneration() {
  if (streamController) {
    streamController.abort()
    streamController = null
  }
}

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
  // 加载持久化状态
  loadSidebarState()
  loadConversations()
  document.addEventListener('keydown', handleKeydown)

  // 如果没有对话, 创建一个
  if (conversations.value.length === 0) {
    createNewConversation()
  } else {
    const latest = sortedConversations.value[0]
    currentConversationId.value = latest.id
    messages.value = latest.messages.map(m => {
      if (m.role === 'assistant') {
        return reactive({ ...m, thinkingStartTime: null })
      }
      return { ...m }
    })
  }

  // 加载后端列表
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

// ===== 文件上传 =====

function triggerFileInput() {
  if (fileInputRef.value) {
    fileInputRef.value.click()
  }
}

async function handleFileSelect(event) {
  const file = event.target.files?.[0]
  if (!file) return
  await uploadFile(file)
  // 重置 input 以便再次选择同一文件
  event.target.value = ''
}

async function uploadFile(file) {
  uploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)
    const res = await fetch(`${apiBase}/api/upload`, {
      method: 'POST',
      body: formData
    })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      alert(data.error || `上传失败: HTTP ${res.status}`)
      return
    }
    const data = await res.json()
    attachedFile.value = {
      name: file.name,
      url: data.url
    }
  } catch (err) {
    alert(`上传失败: ${err.message}`)
  } finally {
    uploading.value = false
  }
}

function removeAttachedFile() {
  attachedFile.value = null
}

// 拖放处理
function handleDragOver(event) {
  event.preventDefault()
  isDragging.value = true
}

function handleDragLeave(event) {
  event.preventDefault()
  isDragging.value = false
}

async function handleDrop(event) {
  event.preventDefault()
  isDragging.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) {
    await uploadFile(file)
  }
}

async function sendMessage() {
  // 正在输出时点击 = 中断
  if (loading.value) {
    stopGeneration()
    return
  }
  if (!canSend.value) return

  const userText = input.value.trim()
  input.value = ''
  await nextTick()
  if (textareaRef.value) textareaRef.value.style.height = 'auto'

  // 拼接附件 URL 到消息内容
  let finalText = userText
  if (attachedFile.value) {
    finalText += `\n\n[附件: ${attachedFile.value.name}](${attachedFile.value.url})`
    attachedFile.value = null
  }

  messages.value.push({ role: 'user', content: finalText })
  // 保存后立即异步生成标题
  saveCurrentMessages()
  generateTitleAsync(currentConversationId.value)
  const assistant = reactive({
    role: 'assistant',
    reasoning: '',
    content: '',
    thinkingDuration: null,
    thinkingStartTime: Date.now(),
    reasoningDone: false,
    retrying: null // { attempt, max_attempts, delay_seconds }
  })
  messages.value.push(assistant)
  // 新消息时总是滚到底部 (用户刚发送)
  await nextTick()
  window.scrollTo(0, document.body.scrollHeight)

  loading.value = true
  startTimer()
  streamController = new AbortController()

  try {
    const res = await fetch(`${apiBase}/api/chat/stream`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        backend_id: selectedBackendId.value || undefined,
        messages: toChatMessages()
      }),
      signal: streamController.signal
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
          assistant.retrying = null
          await scrollToBottom()
        }

        if (event === 'retrying') {
          assistant.retrying = {
            attempt: payload.attempt,
            maxAttempts: payload.max_attempts,
            delaySeconds: payload.delay_seconds
          }
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
    streamController = null
    await scrollToBottom()
    saveCurrentMessages()
  }
}

</script>

<template>
  <div class="app-layout">
    <!-- 侧边栏 -->
    <aside class="sidebar" :class="{ 'sidebar-collapsed': !sidebarOpen }">
      <div class="sidebar-header">
        <img src="/logo.png" alt="Logo" class="sidebar-logo" />
        <button class="sidebar-toggle" @click="toggleSidebar" title="收起侧边栏">
          <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
            <rect x="1" y="2" width="5" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
            <rect x="8" y="2" width="9" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
          </svg>
        </button>
      </div>

      <button class="new-chat-btn" @click="createNewConversation">
        <svg class="plus-icon" width="16" height="16" viewBox="0 0 16 16" fill="none">
          <circle cx="8" cy="8" r="7" stroke="currentColor" stroke-width="1.5"/>
          <line x1="8" y1="4.5" x2="8" y2="11.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
          <line x1="4.5" y1="8" x2="11.5" y2="8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
        </svg>
        <span class="new-chat-text">新对话</span>
        <span class="shortcut-hint">Ctrl K</span>
      </button>

      <div class="sidebar-history">
        <div class="history-header">
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
            <circle cx="7" cy="7" r="5.5" stroke="currentColor" stroke-width="1.2"/>
            <path d="M7 4v3.5l2.5 1.5" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
          </svg>
          <span>历史会话</span>
        </div>
        <div class="history-list">
          <button
            v-for="conv in sortedConversations"
            :key="conv.id"
            class="history-item"
            :class="{ active: conv.id === currentConversationId }"
            @click="switchConversation(conv.id)"
          >
            <span class="history-title">{{ conv.title }}</span>
          </button>
        </div>
      </div>

      <div class="sidebar-footer">
        <div class="user-profile">
          <img src="/avatar.png" alt="Avatar" class="user-avatar" />
          <div class="user-info">
            <span class="username">TaterLi</span>
            <span class="plan-badge">Plus</span>
          </div>
          <svg class="chevron-down" width="14" height="14" viewBox="0 0 14 14" fill="none">
            <path d="M3.5 5.25L7 8.75L10.5 5.25" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
      </div>
    </aside>

    <!-- 主内容区域 -->
    <div class="main-area">
      <div class="page">
        <div class="panel">
          <!-- 空状态 -->
          <div v-if="messages.length === 0" class="empty-state">
            <h2>有什么可以帮你的?</h2>
          </div>
          <!-- 聊天区域 -->
          <div v-else class="chat-area">
            <div v-for="(msg, idx) in messages" :key="idx" class="msg" :class="`msg-${msg.role}`">
              <template v-if="msg.role === 'user'">
                <div class="user-bubble markdown-body" v-html="renderMarkdown(msg.content)"></div>
              </template>
              <template v-else>
                <div class="assistant-message">
                  <!-- 排队重试提示 -->
                  <div v-if="msg.retrying" class="retrying-indicator">
                    <span class="retrying-spinner"></span>
                    <span>服务繁忙,正在排队重试 ({{ msg.retrying.attempt }}/{{ msg.retrying.maxAttempts }})...</span>
                  </div>
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

      <!-- 底部输入栏 -->
      <div class="bottom-bar">
        <div class="panel">
          <div
            class="composer"
            :class="{ 'composer-dragover': isDragging }"
            @dragover="handleDragOver"
            @dragleave="handleDragLeave"
            @drop="handleDrop"
          >
            <input
              ref="fileInputRef"
              type="file"
              class="file-input-hidden"
              @change="handleFileSelect"
            />
            <button class="attach-btn" @click="triggerFileInput" :disabled="loading || uploading" title="上传文件">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48"/>
              </svg>
            </button>
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
            <div class="send-wrapper">
              <div v-if="!input.trim() && !loading && !attachedFile" class="send-tooltip">请输入你的问题</div>
              <button
                class="send"
                :class="{
                  'send-active': (input.trim() || attachedFile) && !loading,
                  'send-loading': loading
                }"
                :disabled="!input.trim() && !attachedFile && !loading"
                @click="sendMessage"
              >
                <!-- 空状态: 暗淡箭头 -->
                <svg v-if="!input.trim() && !attachedFile && !loading" width="18" height="18" viewBox="0 0 18 18" fill="none">
                  <path d="M9 14V4M9 4L4.5 8.5M9 4L13.5 8.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
                <!-- 有输入: 点亮箭头 -->
                <svg v-else-if="(input.trim() || attachedFile) && !loading" width="18" height="18" viewBox="0 0 18 18" fill="none">
                  <path d="M9 14V4M9 4L4.5 8.5M9 4L13.5 8.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
                <!-- 加载中: 停止图标 -->
                <svg v-else width="14" height="14" viewBox="0 0 14 14" fill="currentColor">
                  <rect x="2" y="2" width="10" height="10" rx="2"/>
                </svg>
              </button>
            </div>
          </div>
          <!-- 附件预览 -->
          <div v-if="attachedFile || uploading" class="attachment-bar">
            <div v-if="uploading" class="attachment-chip">
              <span class="attachment-spinner"></span>
              <span>上传中...</span>
            </div>
            <div v-else class="attachment-chip">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M13 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V9z"/><polyline points="13 2 13 9 20 9"/>
              </svg>
              <span class="attachment-name">{{ attachedFile?.name }}</span>
              <button class="attachment-remove" @click="removeAttachedFile" title="移除附件">&times;</button>
            </div>
          </div>
          <div class="meta">
            后端: {{ selectedBackendId || '未连接' }} | API: {{ apiBase }}
          </div>
        </div>
      </div>
    </div>

    <!-- 侧边栏折叠后的展开按钮 -->
    <button
      v-if="!sidebarOpen"
      class="sidebar-open-btn"
      @click="toggleSidebar"
      title="展开侧边栏"
    >
      <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
        <rect x="1" y="2" width="5" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
        <rect x="8" y="2" width="9" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
      </svg>
    </button>
  </div>
</template>
