<script setup>
import { ref, reactive, computed, onMounted, onUnmounted, watch } from 'vue'
import Sidebar from './components/Sidebar.vue'
import ChatMessage from './components/ChatMessage.vue'
import Composer from './components/Composer.vue'
import EmptyState from './components/EmptyState.vue'
import DetailPanel from './components/DetailPanel.vue'
import { useMarkdown } from './composables/useMarkdown'
import { useAuth } from './composables/useAuth'
import { useConversations } from './composables/useConversations'
import { useChatStream } from './composables/useChatStream'

const apiBase = import.meta.env.VITE_API_BASE || 'http://127.0.0.1:18888'

// composables
const { renderMarkdown, copyCode } = useMarkdown()
const { isAuthenticated, needsAuth, authError, checking, authHeaders, login, logout, checkAuth } = useAuth(apiBase)

const {
  conversations, currentConversationId, sidebarOpen, sortedConversations,
  loadConversations, loadSidebarState, createNewConversation,
  switchConversation, saveCurrentMessages, deleteConversation,
  loadLatestConversation, toggleSidebar
} = useConversations(renderMarkdown, apiBase, authHeaders)

const {
  loading, toolCalling, toolCallingName, workingHard, currentThinkingPhrase,
  backends, selectedBackendId, now, conversationStreaming, tokenStats, sendMessage, regenerate, loadBackends, cleanup,
  getRandomThinkingDonePhrase, getThinkingDuration, syncConversationState
} = useChatStream(apiBase, renderMarkdown, authHeaders)

// 登录状态
const loginPassword = ref('')
const loginLoading = ref(false)

async function handleLogin() {
  if (!loginPassword.value.trim()) return
  loginLoading.value = true
  const ok = await login(loginPassword.value)
  loginLoading.value = false
  if (ok) {
    loginPassword.value = ''
    await initAppData()
  }
}

async function initAppData() {
  await loadConversations()
  if (conversations.value.length === 0) {
    createNewConversation(messages, expandedThinking)
  } else {
    loadLatestConversation(messages, expandedThinking)
  }
  loadBackends()
  if (currentConversationId.value) {
    await syncConversationState(currentConversationId.value)
  }
}

// 本地状态
const input = ref('')
const messages = ref([])
const textareaRef = ref(null)
const expandedThinking = reactive(new Map())

// 详情面板状态
const detailPanel = reactive({
  open: false,
  msgIdx: null
})

const detailThinkingDuration = computed(() => {
  if (detailPanel.msgIdx == null) return ''
  return getThinkingDuration(messages.value[detailPanel.msgIdx])
})

const detailMessage = computed(() => {
  if (detailPanel.msgIdx == null) return null
  return messages.value[detailPanel.msgIdx] || null
})

function openThinkingDetail(msgIdx) {
  detailPanel.msgIdx = msgIdx
  detailPanel.open = true
}

function openToolDetail({ msgIdx }) {
  detailPanel.msgIdx = msgIdx
  detailPanel.open = true
}

function closeDetailPanel() {
  detailPanel.open = false
}

// 标题手动修改追踪
const titleManuallySet = reactive(new Map())

// 文件上传
const uploading = ref(false)
const attachedFiles = ref([])
const isDragging = ref(false)
let conversationStateTimer = null

function shouldPollConversationState() {
  return !!currentConversationId.value &&
    !document.hidden &&
    (loading.value || conversationStreaming.value)
}

function handleVisibilityChange() {
  if (!document.hidden && currentConversationId.value) {
    syncConversationState(currentConversationId.value)
  }
}

// 快捷键
function handleKeydown(e) {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault()
    handleNewChat()
  }
}

// 会话操作包装 (避免模板中 ref 自动解包问题)
function handleNewChat() {
  closeDetailPanel()
  createNewConversation(messages, expandedThinking)
}

function handleSelectConversation(id) {
  closeDetailPanel()
  switchConversation(id, messages, expandedThinking)
}

function handleDeleteConversation(id) {
  deleteConversation(id, messages, expandedThinking)
}

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
  await syncConversationState(currentConversationId.value)
  await sendMessage({
    input, messages, attachedFiles, textareaRef, expandedThinking,
    saveCurrentMessages: () => saveCurrentMessages(messages),
    generateTitleAsync,
    currentConversationId
  })
}

// 重新生成回复
async function handleRegenerate(idx) {
  await regenerate({
    idx, messages, expandedThinking,
    saveCurrentMessages: () => saveCurrentMessages(messages),
    currentConversationId
  })
}

// 文件上传

async function handleFileSelect(event) {
  const files = event.target.files
  if (!files || files.length === 0) return
  for (const file of files) {
    await uploadFile(file)
  }
  event.target.value = ''
}

async function uploadFile(file) {
  uploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file)
    const res = await fetch(`${apiBase}/api/upload`, { method: 'POST', headers: authHeaders(), body: formData })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      alert(data.error || `上传失败: HTTP ${res.status}`)
      return
    }
    const data = await res.json()
    attachedFiles.value.push({ name: file.name, url: data.url })
  } catch (err) { alert(`上传失败: ${err.message}`) } finally { uploading.value = false }
}

function removeAttachedFile(index) { attachedFiles.value.splice(index, 1) }
function handleDragOver(e) { e.preventDefault(); isDragging.value = true }
function handleDragLeave(e) { e.preventDefault(); isDragging.value = false }
async function handleDrop(e) { e.preventDefault(); isDragging.value = false; const files = e.dataTransfer?.files; if (files) { for (const file of files) { await uploadFile(file) } } }

// 异步生成标题 (带重试机制)
async function generateTitleAsync(convId, maxRetries = 3, retryDelay = 1000) {
  const conv = conversations.value.find(c => c.id === convId)
  if (!conv) return
  // 如果用户手动修改过标题,不再自动覆盖
  if (titleManuallySet.get(convId)) return
  if (conv.title !== '新对话') return
  const userMsgs = messages.value.filter(m => m.role === 'user')
  if (userMsgs.length === 0) return

  for (let attempt = 0; attempt < maxRetries; attempt++) {
    try {
      const titleMessages = [
        { role: 'system', content: '你是一个标题生成器. 请根据用户的消息内容, 用中文生成一个简短的对话标题 (不超过20个字). 只输出标题文本, 不要加引号, 不要加句号, 不要加任何额外说明.' },
        { role: 'user', content: userMsgs.slice(0, 2).map(m => m.content).join('\n') }
      ]

      const res = await fetch(`${apiBase}/api/chat/stream`, {
        method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({ backend_id: selectedBackendId.value || undefined, messages: titleMessages })
      })
      if (!res.ok || !res.body) break

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
        if (target && !titleManuallySet.get(convId)) {
          target.title = title
        }
      }
      return // 成功则退出
    } catch {
      // 静默失败,等待重试
      if (attempt < maxRetries - 1) {
        await new Promise(r => setTimeout(r, retryDelay))
      }
    }
  }
}

// 标记标题已被用户手动修改
function markTitleManuallySet(convId) {
  titleManuallySet.set(convId, true)
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
  await checkAuth()
  if (needsAuth.value && !isAuthenticated.value) return

  document.addEventListener('keydown', handleKeydown)
  document.addEventListener('visibilitychange', handleVisibilityChange)

  await initAppData()

  conversationStateTimer = setInterval(() => {
    if (!shouldPollConversationState()) return
    syncConversationState(currentConversationId.value)
  }, 1500)
})

onUnmounted(() => {
  cleanup()
  document.removeEventListener('keydown', handleKeydown)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  if (conversationStateTimer) {
    clearInterval(conversationStateTimer)
    conversationStateTimer = null
  }
})

watch(currentConversationId, (id) => {
  syncConversationState(id)
})
</script>

<template>
  <!-- 认证检查中 -->
  <div v-if="checking" class="auth-checking">
    <div class="auth-checking-spinner"></div>
    <p>Connecting...</p>
  </div>

  <!-- 登录界面 -->
  <div v-else-if="needsAuth && !isAuthenticated" class="login-screen">
    <div class="login-card">
      <h2>Login</h2>
      <form @submit.prevent="handleLogin">
        <input
          v-model="loginPassword"
          type="password"
          placeholder="Enter password"
          class="login-input"
          :disabled="loginLoading"
          autofocus
        />
        <p v-if="authError" class="login-error">{{ authError }}</p>
        <button type="submit" class="login-btn" :disabled="loginLoading || !loginPassword.trim()">
          {{ loginLoading ? 'Logging in...' : 'Login' }}
        </button>
      </form>
    </div>
  </div>

  <!-- 正常应用 -->
  <div v-else class="app-layout">
    <Sidebar
      :conversations="conversations"
      :current-conversation-id="currentConversationId"
      :sidebar-open="sidebarOpen"
      @toggle="toggleSidebar"
      @new-chat="handleNewChat"
      @select-conversation="handleSelectConversation"
      @delete-conversation="handleDeleteConversation"
    />

    <div
      class="main-area"
      :class="{ 'main-dragover': isDragging }"
      @dragover="handleDragOver"
      @dragleave="handleDragLeave"
      @drop="handleDrop"
    >
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
              :loading="loading"
              :working-hard="workingHard"
              :tool-calling="toolCalling"
              :tool-calling-name="toolCallingName"
              :current-thinking-phrase="currentThinkingPhrase"
              :get-random-thinking-done-phrase="getRandomThinkingDonePhrase"
              :get-thinking-duration="getThinkingDuration"
              @copy-code="copyCode"
              @regenerate="handleRegenerate"
              @open-thinking-detail="openThinkingDetail"
              @open-tool-detail="openToolDetail"
            />
          </div>
        </div>
      </div>

      <div class="bottom-bar">
        <div class="panel">
          <Composer
            :input="input"
            :loading="loading"
            :attached-files="attachedFiles"
            :uploading="uploading"
            :is-dragging="isDragging"
            :selected-backend-id="selectedBackendId"
            :api-base="apiBase"
            :conversation-streaming="conversationStreaming"
            :token-stats="tokenStats"
            @update:input="input = $event"
            @send="handleSend"
            @file-select="handleFileSelect"
            @remove-attached="removeAttachedFile"
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

    <DetailPanel
      :open="detailPanel.open"
      :message="detailMessage"
      :thinking-duration="detailThinkingDuration"
      @close="closeDetailPanel"
    />
  </div>
</template>
