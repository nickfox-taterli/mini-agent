<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'

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

const showSettings = ref(false)

function handleDelete(e, id) {
  e.stopPropagation()
  emit('delete-conversation', id)
}

function toggleSettings() {
  showSettings.value = !showSettings.value
}

function handleClickOutside(e) {
  if (showSettings.value && !e.target.closest('.settings-popup') && !e.target.closest('.user-profile')) {
    showSettings.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})

const sortedConversations = computed(() => {
  return [...props.conversations].sort((a, b) => b.createdAt - a.createdAt)
})
</script>

<template>
  <aside class="sidebar" :class="{ 'sidebar-collapsed': !sidebarOpen }">
    <div class="sidebar-header">
      <img src="/logo.png" alt="Logo" class="sidebar-logo" />
      <button class="sidebar-toggle" @click="emit('toggle')" title="收起侧边栏">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="3" y1="6" x2="21" y2="6"/>
          <line x1="3" y1="12" x2="21" y2="12"/>
          <line x1="3" y1="18" x2="21" y2="18"/>
        </svg>
      </button>
    </div>

    <button class="new-chat-btn" @click="emit('new-chat')">
      <svg class="plus-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 5v14M5 12h14" stroke="#4FC3F7"/>
      </svg>
      <span class="new-chat-text">新对话</span>
      <span class="shortcut-hint">Ctrl K</span>
    </button>

    <div class="sidebar-history">
      <div class="history-header">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10" stroke="#4FC3F7"/>
          <polyline points="12 6 12 12 16 14" stroke="#4FC3F7"/>
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
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>
      </div>
    </div>

    <div class="sidebar-footer">
      <div class="user-profile" @click="toggleSettings">
        <img src="/avatar.png" alt="Avatar" class="user-avatar" />
        <div class="user-info">
          <span class="username">TaterLi</span>
          <span class="plan-badge">Plus</span>
        </div>
      </div>
      <div v-if="showSettings" class="settings-popup">
        <div class="settings-header">
          <img src="/avatar.png" alt="Avatar" class="settings-avatar" />
          <div class="settings-user-info">
            <span class="settings-username">TaterLi</span>
            <span class="settings-plan">Plus 会员</span>
          </div>
        </div>
        <div class="settings-divider"></div>
        <div class="settings-item">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
          <span>设置</span>
        </div>
        <div class="settings-item">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
          <span>帮助与反馈</span>
        </div>
        <div class="settings-divider"></div>
        <div class="settings-item">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
          <span>退出登录</span>
        </div>
      </div>
    </div>
  </aside>
</template>
