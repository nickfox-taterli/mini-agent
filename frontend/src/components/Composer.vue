<script setup>
import { ref, computed, watch, nextTick, onMounted } from 'vue'

const fileInputRef = ref(null)
const textareaRef = ref(null)

const props = defineProps({
  input: {
    type: String,
    default: ''
  },
  loading: {
    type: Boolean,
    default: false
  },
  conversationStreaming: {
    type: Boolean,
    default: false
  },
  attachedFiles: {
    type: Array,
    default: () => []
  },
  uploading: {
    type: Boolean,
    default: false
  },
  isDragging: {
    type: Boolean,
    default: false
  },
  selectedBackendId: {
    type: String,
    default: ''
  },
  apiBase: {
    type: String,
    default: ''
  },
  tokenStats: {
    type: Object,
    default: () => ({ estimatedTokens: 0, tokensPerSecond: 0, coefficient: 1 })
  }
})

const emit = defineEmits([
  'update:input',
  'send',
  'stop',
  'file-select',
  'remove-attached',
  'dragover',
  'dragleave',
  'drop'
])

function triggerFileInput() {
  fileInputRef.value?.click()
}

const canSend = computed(() => {
  return (props.input.trim() !== '' || props.attachedFiles.length > 0) && !props.loading && !props.conversationStreaming
})

function autoResizeTextarea() {
  const el = textareaRef.value
  if (!el) return

  el.style.height = 'auto'
  const lineHeight = parseFloat(getComputedStyle(el).lineHeight) || 24
  const maxHeight = lineHeight * 10
  const nextHeight = Math.min(el.scrollHeight, maxHeight)

  el.style.height = `${nextHeight}px`
  el.style.overflowY = el.scrollHeight > maxHeight ? 'auto' : 'hidden'
}

function handleTextareaInput(event) {
  const value = event.target.value
  emit('update:input', value)
  emit('input', event)
  autoResizeTextarea()
}

watch(
  () => props.input,
  async () => {
    await nextTick()
    autoResizeTextarea()
  }
)

onMounted(() => {
  autoResizeTextarea()
})
</script>

<template>
  <div
    class="composer"
    :class="{ 'composer-dragover': isDragging }"
  >
    <input
      type="file"
      ref="fileInputRef"
      class="file-input-hidden"
      multiple
      @change="emit('file-select', $event)"
    />
    <button class="attach-btn" @click="triggerFileInput" :disabled="loading || uploading || conversationStreaming" title="上传文件">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21 11v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6"/>
        <polyline points="17 8 12 3 7 8"/>
        <line x1="12" y1="3" x2="12" y2="15"/>
      </svg>
    </button>
    <textarea
      ref="textareaRef"
      :value="input"
      class="input"
      :placeholder="conversationStreaming ? '另一页面正在生成中...' : '尽管问...'"
      rows="1"
      :disabled="loading || conversationStreaming"
      @input="handleTextareaInput"
      @keydown.enter.exact.prevent="emit('send')"
    />
    <div class="send-wrapper">
      <div v-if="conversationStreaming" class="send-tooltip">另一页面正在生成, 暂不可发送</div>
      <div v-else-if="!input.trim() && !loading && attachedFiles.length === 0" class="send-tooltip">请输入你的问题</div>
      <button
        class="send"
        :class="{
          'send-active': (input.trim() || attachedFiles.length > 0) && !loading && !conversationStreaming,
          'send-loading': loading
        }"
        :disabled="(!input.trim() && attachedFiles.length === 0 && !loading) || conversationStreaming"
        @click="emit('send')"
      >
        <!-- 空状态: 暗淡箭头 -->
        <svg v-if="(!input.trim() && attachedFiles.length === 0 && !loading) || conversationStreaming" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="22" y1="2" x2="11" y2="13"/>
          <polygon points="22 2 15 22 11 13 2 9 22 2"/>
        </svg>
        <svg v-else-if="canSend" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="22" y1="2" x2="11" y2="13"/>
          <polygon points="22 2 15 22 11 13 2 9 22 2"/>
        </svg>
        <!-- 加载中: 停止图标 -->
        <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
          <rect x="6" y="6" width="12" height="12" rx="2"/>
        </svg>
      </button>
    </div>
  </div>
  <!-- 附件预览 -->
  <div v-if="attachedFiles.length > 0 || uploading" class="attachment-bar">
    <div v-if="uploading" class="attachment-chip">
      <span class="attachment-spinner"></span>
      <span>上传中...</span>
    </div>
    <div v-for="(file, index) in attachedFiles" :key="index" class="attachment-chip">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M13 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V9z"/><polyline points="13 2 13 9 20 9"/>
      </svg>
      <span class="attachment-name">{{ file.name }}</span>
      <button class="attachment-remove" @click="emit('remove-attached', index)" title="移除附件">&times;</button>
    </div>
  </div>
  <div class="meta">
    后端: {{ selectedBackendId || '未连接' }} | API: {{ apiBase }} | Token: {{ tokenStats.estimatedTokens }} | Token/s: {{ tokenStats.tokensPerSecond.toFixed(2) }} | 系数: {{ tokenStats.coefficient }}
  </div>
</template>
