import { ref, computed } from 'vue'

const STORAGE_KEY_SIDEBAR = 'agent-chat-sidebar-open'

export function useConversations(renderMarkdown, apiBase) {
  const conversations = ref([])
  const currentConversationId = ref(null)
  const sidebarOpen = ref(true)

  // 追踪仅存在于本地的会话 (空对话不进数据库)
  const localOnlyIds = new Set()

  // --- 侧边栏状态 (仍用 LocalStorage, 纯 UI 状态) ---

  function loadSidebarState() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY_SIDEBAR)
      if (raw !== null) sidebarOpen.value = JSON.parse(raw)
    } catch { sidebarOpen.value = true }
  }

  function saveSidebarState() {
    try { localStorage.setItem(STORAGE_KEY_SIDEBAR, JSON.stringify(sidebarOpen.value)) } catch {}
  }

  function toggleSidebar() {
    sidebarOpen.value = !sidebarOpen.value
    saveSidebarState()
  }

  // --- 工具函数 ---

  function generateId() {
    return `conv-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
  }

  function extractTitle(msgs) {
    const first = msgs.find(m => m.role === 'user')
    if (!first) return '新对话'
    const text = first.content.trim().replace(/\n/g, ' ')
    return text.length > 30 ? text.slice(0, 30) + '...' : text
  }

  // 将后端消息格式转换为前端运行时格式 (添加渲染字段)
  function toRuntimeMessages(msgs) {
    return msgs.map(m => {
      const base = {
        role: m.role, content: m.content,
        renderedContent: renderMarkdown(m.content),
        reasoning: m.reasoning || '', reasoningDone: m.reasoningDone || false,
        thinkingDuration: m.thinkingDuration || null
      }
      if (m.role === 'assistant') {
        base.renderedReasoning = renderMarkdown(m.reasoning || '')
        base.thinkingStartTime = null
        base.retrying = null
        base.toolCalls = m.toolCalls || []
        base.processTimeline = m.processTimeline || []
      }
      return { ...base }
    })
  }

  // 将前端运行时消息转为后端存储格式 (去除渲染字段)
  function toStorageMessages(messages) {
    return messages.value.map(m => ({
      role: m.role, content: m.content, reasoning: m.reasoning || '',
      reasoningDone: m.reasoningDone || false, thinkingDuration: m.thinkingDuration || null,
      toolCalls: m.toolCalls || [],
      processTimeline: m.processTimeline || []
    }))
  }

  // --- 后端 API 调用 ---

  async function loadConversations() {
    try {
      const res = await fetch(`${apiBase}/api/conversations`)
      if (!res.ok) return
      const data = await res.json()
      conversations.value = data.conversations || []
    } catch { conversations.value = [] }
  }

  async function createNewConversation(messages, expandedThinking) {
    // 如果当前会话已经是空的, 直接复用, 不创建新的
    const current = conversations.value.find(c => c.id === currentConversationId.value)
    if (current && messages.value.length === 0) return

    // 如果存在其他空的本地会话, 复用它
    const existingLocal = conversations.value.find(
      c => c.id !== currentConversationId.value && localOnlyIds.has(c.id)
    )
    if (existingLocal) {
      await saveCurrentMessages(messages)
      currentConversationId.value = existingLocal.id
      messages.value = []
      expandedThinking.clear()
      return
    }

    // 先保存当前会话的消息到后端
    await saveCurrentMessages(messages)

    // 仅在本地创建, 不写入数据库
    const id = generateId()
    const conv = { id, title: '新对话', messages: [], createdAt: Date.now() }
    localOnlyIds.add(id)
    conversations.value.unshift({ ...conv, messages: [] })

    currentConversationId.value = id
    messages.value = []
    expandedThinking.clear()
  }

  async function switchConversation(id, messages, expandedThinking) {
    if (id === currentConversationId.value) return
    // 保存当前会话
    await saveCurrentMessages(messages)
    currentConversationId.value = id

    // 从后端加载目标会话的消息
    try {
      const res = await fetch(`${apiBase}/api/conversations/${id}`)
      if (res.ok) {
        const data = await res.json()
        const conv = data.conversation
        if (conv) {
          messages.value = toRuntimeMessages(conv.messages || [])
        }
      }
    } catch {}

    expandedThinking.clear()
  }

  async function saveCurrentMessages(messages) {
    if (!currentConversationId.value) return
    const conv = conversations.value.find(c => c.id === currentConversationId.value)
    if (!conv) return

    const storedMsgs = toStorageMessages(messages)
    const isLocalOnly = localOnlyIds.has(currentConversationId.value)

    // 本地会话且无消息 -> 丢弃, 不持久化
    if (isLocalOnly && storedMsgs.length === 0) {
      conversations.value = conversations.value.filter(c => c.id !== currentConversationId.value)
      localOnlyIds.delete(currentConversationId.value)
      return
    }

    const title = (conv.title === '新对话' || !conv.title) ? extractTitle(storedMsgs) : conv.title

    try {
      // 本地会话有消息了 -> 先 POST 创建, 再标记为已持久化
      if (isLocalOnly) {
        await fetch(`${apiBase}/api/conversations`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: conv.id, title, createdAt: conv.createdAt })
        })
        localOnlyIds.delete(currentConversationId.value)
      }

      await fetch(`${apiBase}/api/conversations/${currentConversationId.value}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title, messages: storedMsgs })
      })
      // 同步更新本地 title
      conv.title = title
    } catch {}
  }

  async function loadLatestConversation(messages, expandedThinking) {
    if (conversations.value.length === 0) return
    const sorted = [...conversations.value].sort((a, b) => b.createdAt - a.createdAt)
    const latest = sorted[0]
    if (!latest) return

    currentConversationId.value = latest.id

    // 从后端加载消息
    try {
      const res = await fetch(`${apiBase}/api/conversations/${latest.id}`)
      if (res.ok) {
        const data = await res.json()
        const conv = data.conversation
        if (conv) {
          messages.value = toRuntimeMessages(conv.messages || [])
          // 同步会话列表中的标题
          latest.title = conv.title
        }
      }
    } catch {}

    expandedThinking.clear()
  }

  async function deleteConversation(id, messages, expandedThinking) {
    // 如果是当前会话, 先切换走
    if (id === currentConversationId.value) {
      const remaining = conversations.value.filter(c => c.id !== id)
      if (remaining.length > 0) {
        // 切换到最新的其他会话
        const sorted = [...remaining].sort((a, b) => b.createdAt - a.createdAt)
        currentConversationId.value = sorted[0].id
        try {
          const res = await fetch(`${apiBase}/api/conversations/${sorted[0].id}`)
          if (res.ok) {
            const data = await res.json()
            if (data.conversation) {
              messages.value = toRuntimeMessages(data.conversation.messages || [])
            }
          }
        } catch {}
      } else {
        // 没有其他会话了, 创建新的空会话
        const newId = generateId()
        const conv = { id: newId, title: '新对话', messages: [], createdAt: Date.now() }
        localOnlyIds.add(newId)
        conversations.value.unshift(conv)
        currentConversationId.value = newId
        messages.value = []
      }
      expandedThinking.clear()
    }

    // 从列表中移除
    conversations.value = conversations.value.filter(c => c.id !== id)
    localOnlyIds.delete(id)

    // 非本地会话需要调用后端删除
    if (!localOnlyIds.has(id)) {
      try {
        await fetch(`${apiBase}/api/conversations/${id}`, { method: 'DELETE' })
      } catch {}
    }
  }

  const sortedConversations = computed(() => [...conversations.value].sort((a, b) => b.createdAt - a.createdAt))

  return {
    conversations, currentConversationId, sidebarOpen, sortedConversations,
    loadConversations, loadSidebarState, saveSidebarState,
    createNewConversation, switchConversation, saveCurrentMessages,
    deleteConversation, loadLatestConversation, toggleSidebar,
    extractTitle, generateId
  }
}
