<script setup>
import { ref, computed } from 'vue'

const fileInputRef = ref(null)

const props = defineProps({
  input: {
    type: String,
    default: ''
  },
  loading: {
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

const canSend = computed(() => (props.input.trim() !== '' || props.attachedFiles.length > 0) && !props.loading)
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
    <button class="attach-btn" @click="triggerFileInput" :disabled="loading || uploading" title="上传文件">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48"/>
      </svg>
    </button>
    <textarea
      :value="input"
      class="input"
      placeholder="尽管问..."
      rows="1"
      :disabled="loading"
      @input="emit('update:input', $event.target.value); $emit('input', $event)"
      @keydown.enter.exact.prevent="emit('send')"
    />
    <div class="send-wrapper">
      <div v-if="!input.trim() && !loading && attachedFiles.length === 0" class="send-tooltip">请输入你的问题</div>
      <button
        class="send"
        :class="{
          'send-active': (input.trim() || attachedFiles.length > 0) && !loading,
          'send-loading': loading
        }"
        :disabled="!input.trim() && attachedFiles.length === 0 && !loading"
        @click="emit('send')"
      >
        <!-- 空状态: 暗淡箭头 -->
        <svg v-if="!input.trim() && attachedFiles.length === 0 && !loading" width="18" height="18" viewBox="0 0 18 18" fill="none">
          <path d="M9 14V4M9 4L4.5 8.5M9 4L13.5 8.5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <!-- 有输入: 点亮箭头 -->
        <svg v-else-if="(input.trim() || attachedFiles.length > 0) && !loading" width="18" height="18" viewBox="0 0 18 18" fill="none">
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
    后端: {{ selectedBackendId || '未连接' }} | API: {{ apiBase }}
  </div>
</template>
