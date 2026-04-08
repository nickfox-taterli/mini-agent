import { ref, computed } from 'vue'

const STORAGE_KEY_CONVERSATIONS = 'agent-chat-conversations'
const STORAGE_KEY_SIDEBAR = 'agent-chat-sidebar-open'

export function useConversations(renderMarkdown) {
  const conversations = ref([])
  const currentConversationId = ref(null)
  const sidebarOpen = ref(true)

  function loadConversations() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY_CONVERSATIONS)
      if (raw) conversations.value = JSON.parse(raw)
    } catch { conversations.value = [] }
  }

  function saveConversations() {
    try { localStorage.setItem(STORAGE_KEY_CONVERSATIONS, JSON.stringify(conversations.value)) } catch {}
  }

  function loadSidebarState() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY_SIDEBAR)
      if (raw !== null) sidebarOpen.value = JSON.parse(raw)
    } catch { sidebarOpen.value = true }
  }

  function saveSidebarState() {
    try { localStorage.setItem(STORAGE_KEY_SIDEBAR, JSON.stringify(sidebarOpen.value)) } catch {}
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

  function createNewConversation(messages, expandedThinking) {
    const current = conversations.value.find(c => c.id === currentConversationId.value)
    if (current && current.messages.length === 0) return
    saveCurrentMessages(messages)
    const conv = { id: generateId(), title: '新对话', messages: [], createdAt: Date.now() }
    conversations.value.unshift(conv)
    currentConversationId.value = conv.id
    messages.value = []
    expandedThinking.clear()
    saveConversations()
  }

  function switchConversation(id, messages, expandedThinking) {
    if (id === currentConversationId.value) return
    saveCurrentMessages(messages)
    currentConversationId.value = id
    const conv = conversations.value.find(c => c.id === id)
    if (conv) {
      messages.value = conv.messages.map(m => {
        const base = {
          role: m.role, content: m.content,
          reasoning: m.reasoning || '', reasoningDone: m.reasoningDone || false,
          thinkingDuration: m.thinkingDuration || null
        }
        if (m.role === 'assistant') {
          base.renderedContent = renderMarkdown(m.content)
          base.renderedReasoning = renderMarkdown(m.reasoning || '')
          base.thinkingStartTime = null; base.retrying = null
        }
        return { ...base }
      })
      expandedThinking.clear()
    }
  }

  function saveCurrentMessages(messages) {
    if (!currentConversationId.value) return
    const conv = conversations.value.find(c => c.id === currentConversationId.value)
    if (!conv) return
    conv.messages = messages.value.map(m => ({
      role: m.role, content: m.content, reasoning: m.reasoning || '',
      reasoningDone: m.reasoningDone || false, thinkingDuration: m.thinkingDuration || null
    }))
    if (conv.title === '新对话' || !conv.title) conv.title = extractTitle(conv.messages)
    saveConversations()
  }

  function loadLatestConversation(messages, expandedThinking) {
    const latest = [...conversations.value].sort((a, b) => b.createdAt - a.createdAt)[0]
    if (!latest) return
    currentConversationId.value = latest.id
    messages.value = latest.messages.map(m => {
      const base = {
        role: m.role, content: m.content,
        reasoning: m.reasoning || '', reasoningDone: m.reasoningDone || false,
        thinkingDuration: m.thinkingDuration || null
      }
      if (m.role === 'assistant') {
        base.renderedContent = renderMarkdown(m.content)
        base.renderedReasoning = renderMarkdown(m.reasoning || '')
        base.thinkingStartTime = null; base.retrying = null
      }
      return { ...base }
    })
    expandedThinking.clear()
  }

  function toggleSidebar() {
    sidebarOpen.value = !sidebarOpen.value
    saveSidebarState()
  }

  const sortedConversations = computed(() => [...conversations.value].sort((a, b) => b.createdAt - a.createdAt))

  return {
    conversations, currentConversationId, sidebarOpen, sortedConversations,
    loadConversations, saveConversations, loadSidebarState, saveSidebarState,
    createNewConversation, switchConversation, saveCurrentMessages,
    loadLatestConversation, toggleSidebar, extractTitle, generateId
  }
}
