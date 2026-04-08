<script setup>
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import Sidebar from './components/Sidebar.vue'
import ChatMessage from './components/ChatMessage.vue'
import Composer from './components/Composer.vue'
import EmptyState from './components/EmptyState.vue'
import { useMarkdown } from './composables/useMarkdown'
import { useConversations } from './composables/useConversations'
import { useChatStream } from './composables/useChatStream'

const apiBase = import.meta.env.VITE_API_BASE || 'http://127.0.0.1:18888'

// composables
const { renderMarkdown, copyCode } = useMarkdown()
const {
  conversations, currentConversationId, sidebarOpen, sortedConversations,
  loadConversations, loadSidebarState, createNewConversation,
  switchConversation, saveCurrentMessages, loadLatestConversation, toggleSidebar
} = useConversations(renderMarkdown)

const {
  loading, toolCalling, toolCallingName, workingHard, currentThinkingPhrase,
  backends, selectedBackendId, now, sendMessage, loadBackends, cleanup,
  getRandomThinkingDonePhrase, getThinkingDuration
} = useChatStream(apiBase, renderMarkdown)

// 本地状态
const input = ref('')
const messages = ref([])
const textareaRef = ref(null)
const expandedThinking = reactive(new Map())

// 文件上传
const fileInputRef = ref(null)
const uploading = ref(false)
const attachedFile = ref(null)
const isDragging = ref(false)

// 快捷键
function handleKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault()
    createNewConversation(messages, expandedThinking)
  }
}

// 思考展开/折叠
function toggleThinking(idx) { expandedThinking.set(idx, !expandedThinking.get(idx)) }
function isThinkingExpanded(idx) { return expandedThinking.get(idx) === true }

// 自动调整输入框高度
function autoResize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const lineHeight = parseFloat(getComputedStyle(el).lineHeight) || 24
  el.style.height = Math.min(el.scrollHeight, lineHeight * 10 + 4) + 'px'
}

// 发送消息包装
async function handleSend() {
  await sendMessage({
    input, messages, attachedFile, textareaRef, expandedThinking,
    saveCurrentMessages: () => saveCurrentMessages(messages),
    generateTitleAsync,
    currentConversationId
  })
}

// 文件上传
function triggerFileInput() { if (fileInputRef.value) fileInputRef.value.click() }

async function handleFileSelect(event) {
  const file = event.target.files?.[0]
  if (!file) return
  await uploadFile(file)
  event.target.value = ''
}

async function uploadFile(file) {
  uploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)
    const res = await fetch(`${apiBase}/api/upload`, { method: 'POST', body: formData })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      alert(data.error || `上传失败: HTTP ${res.status}`)
      return
    }
    const data = await res.json()
    attachedFile.value = { name: file.name, url: data.url }
  } catch (err) { alert(`上传失败: ${err.message}`) } finally { uploading.value = false }
}

function removeAttachedFile() { attachedFile.value = null }
function handleDragOver(e) { e.preventDefault(); isDragging.value = true }
function handleDragLeave(e) { e.preventDefault(); isDragging.value = false }
async function handleDrop(e) { e.preventDefault(); isDragging.value = false; const file = e.dataTransfer?.files?.[0]; if (file) await uploadFile(file) }

// 异步生成标题
async function generateTitleAsync(convId) {
  const conv = conversations.value.find(c => c.id === convId)
  if (!conv) return
  if (conv.title !== extractTitle(conv.messages)) return
  if (!conv.messages.some(m => m.role === 'user')) return

  try {
    const titleMessages = [
      { role: 'system', content: '你是一个标题生成器. 请根据用户的消息内容, 用中文生成一个简短的对话标题 (不超过20个字). 只输出标题文本, 不要加引号, 不要加句号, 不要加任何额外说明.' },
      { role: 'user', content: conv.messages.filter(m => m.role === 'user').slice(0, 2).map(m => m.content).join('\n') }
    ]

    const res = await fetch(`${apiBase}/api/chat/stream`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ backend_id: selectedBackendId.value || undefined, messages: titleMessages })
    })
    if (!res.ok || !res.body) return

    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = '', title = ''

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
        if (event === 'content') title += payload.delta || ''
        if (event === 'done' || event === 'error') { await reader.cancel(); break }
      }
    }

    title = title.trim().replace(/^["""']|["""']$/g, '').replace(/\n/g, ' ')
    if (title && title.length > 0 && title.length <= 50) {
      const target = conversations.value.find(c => c.id === convId)
      if (target) { target.title = title }
    }
  } catch {}
}

function extractTitle(msgs) {
  const first = msgs.find(m => m.role === 'user')
  if (!first) return '新对话'
  const text = first.content.trim().replace(/\n/g, ' ')
  return text.length > 30 ? text.slice(0, 30) + '...' : text
}

function takeNextSSEBlock(buffer) {
  const normalized = buffer.replace(/\r\n/g, '\n')
  const idx = normalized.indexOf('\n\n')
  if (idx < 0) return { block: null, rest: normalized }
  return { block: normalized.slice(0, idx), rest: normalized.slice(idx + 2) }
}

function parseSSEBlock(block) {
  const lines = block.split('\n')
  let event = 'message'
  const dataLines = []
  for (const line of lines) {
    if (line.startsWith('event:')) event = line.slice(6).trim()
    if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
  }
  const raw = dataLines.join('\n')
  if (!raw) return { event, payload: null }
  try { return { event, payload: JSON.parse(raw) } } catch { return { event, payload: { message: raw } } }
}

// 初始化
onMounted(async () => {
  loadSidebarState()
  loadConversations()
  document.addEventListener('keydown', handleKeydown)

  if (conversations.value.length === 0) {
    createNewConversation(messages, expandedThinking)
  } else {
    loadLatestConversation(messages, expandedThinking)
  }

  loadBackends()
})

onUnmounted(() => {
  cleanup()
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div class="app-layout">
    <Sidebar
      :conversations="conversations"
      :current-conversation-id="currentConversationId"
      :sidebar-open="sidebarOpen"
      @toggle="toggleSidebar"
      @new-chat="createNewConversation(messages, expandedThinking)"
      @select-conversation="switchConversation($event, messages, expandedThinking)"
    />

    <div class="main-area">
      <div class="page">
        <div class="panel">
          <EmptyState v-if="messages.length === 0" />
          <div v-else class="chat-area">
            <ChatMessage
              v-for="(msg, idx) in messages"
              :key="idx"
              :msg="msg"
              :idx="idx"
              :is-last="idx === messages.length - 1"
              :working-hard="workingHard"
              :tool-calling="toolCalling"
              :tool-calling-name="toolCallingName"
              :current-thinking-phrase="currentThinkingPhrase"
              :is-thinking-expanded="isThinkingExpanded(idx)"
              :get-random-thinking-done-phrase="getRandomThinkingDonePhrase"
              :get-thinking-duration="getThinkingDuration"
              @toggle-thinking="toggleThinking"
              @copy-code="copyCode"
            />
          </div>
        </div>
      </div>

      <div class="bottom-bar">
        <div class="panel">
          <Composer
            :input="input"
            :loading="loading"
            :attached-file="attachedFile"
            :uploading="uploading"
            :is-dragging="isDragging"
            :selected-backend-id="selectedBackendId"
            :api-base="apiBase"
            @update:input="input = $event"
            @send="handleSend"
            @attach="triggerFileInput"
            @file-select="handleFileSelect"
            @remove-attached="removeAttachedFile"
            @dragover="handleDragOver"
            @dragleave="handleDragLeave"
            @drop="handleDrop"
          />
        </div>
      </div>
    </div>

    <button v-if="!sidebarOpen" class="sidebar-open-btn" @click="toggleSidebar" title="展开侧边栏">
      <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
        <rect x="1" y="2" width="5" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
        <rect x="8" y="2" width="9" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
      </svg>
    </button>
  </div>
</template>
