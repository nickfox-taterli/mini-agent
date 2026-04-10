<script setup>
import { computed } from 'vue'

const props = defineProps({
  conversations: {
    type: Array,
    default: () => []
  },
  currentConversationId: {
    type: String,
    default: null
  },
  sidebarOpen: {
    type: Boolean,
    default: true
  }
})

const emit = defineEmits(['toggle', 'new-chat', 'select-conversation', 'delete-conversation'])

function handleDelete(e, id) {
  e.stopPropagation()
  emit('delete-conversation', id)
}

const sortedConversations = computed(() => {
  return [...props.conversations].sort((a, b) => b.createdAt - a.createdAt)
})
</script>

<template>
  <aside class="sidebar" :class="{ 'sidebar-collapsed': !sidebarOpen }">
    <div class="sidebar-header">
      <img src="/logo.png" alt="Logo" class="sidebar-logo" />
      <button class="sidebar-toggle" @click="emit('toggle')" title="收起侧边栏">
        <svg width="18" height="18" viewBox="0 0 18 18" fill="none">
          <rect x="1" y="2" width="5" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
          <rect x="8" y="2" width="9" height="14" rx="1" stroke="currentColor" stroke-width="1.5"/>
        </svg>
      </button>
    </div>

    <button class="new-chat-btn" @click="emit('new-chat')">
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
        <div
          v-for="conv in sortedConversations"
          :key="conv.id"
          class="history-item"
          :class="{ active: conv.id === currentConversationId }"
          @click="emit('select-conversation', conv.id)"
        >
          <span class="history-title">{{ conv.title }}</span>
          <button class="history-delete" @click="handleDelete($event, conv.id)" title="删除会话">
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
              <path d="M2.5 2.5L9.5 9.5M9.5 2.5L2.5 9.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
            </svg>
          </button>
        </div>
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
</template>
