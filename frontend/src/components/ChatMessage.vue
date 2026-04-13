<script setup>
import { ref, computed } from 'vue'

const props = defineProps({
  msg: { type: Object, required: true },
  idx: { type: Number, required: true },
  isLast: { type: Boolean, default: false },
  loading: { type: Boolean, default: false },
  workingHard: { type: Boolean, default: false },
  toolCalling: { type: Boolean, default: false },
  toolCallingName: { type: String, default: '' },
  currentThinkingPhrase: { type: String, default: '' },
  isThinkingExpanded: { type: Boolean, default: false },
  getRandomThinkingDonePhrase: { type: Function, required: true },
  getThinkingDuration: { type: Function, required: true }
})

const emit = defineEmits(['toggle-thinking', 'copy-code', 'regenerate'])

const isComplete = computed(() => !props.isLast || !props.loading)
const copyLabel = ref('复制Markdown')

function handleCopyMarkdown() {
  navigator.clipboard.writeText(props.msg.content || '').then(() => {
    copyLabel.value = '已复制'
    setTimeout(() => { copyLabel.value = '复制Markdown' }, 2000)
  })
}
</script>

<template>
  <div class="msg" :class="`msg-${msg.role}`">
    <template v-if="msg.role === 'user'">
      <div class="user-bubble markdown-body" v-html="msg.renderedContent"></div>
    </template>
    <template v-else>
      <div class="assistant-message">
        <!-- 排队重试提示 -->
        <div v-if="msg.retrying" class="retrying-indicator">
          <span class="retrying-spinner"></span>
          <span>LLM服务排队中...</span>
        </div>
        <!-- 正在努力干活提示 -->
        <div v-if="workingHard && !msg.retrying && isLast" class="working-hard-indicator">
          <span class="working-hard-spinner"></span>
          <span>正在非常努力干活...</span>
        </div>
        <!-- 工具调用提示 -->
        <div v-if="toolCalling" class="tool-calling-indicator">
          <span class="tool-calling-spinner"></span>
          <span>正在调用工具: {{ toolCallingName || '...' }}</span>
        </div>
        <!-- 思考块 -->
        <div v-if="msg.reasoning" class="thinking-block">
          <button class="thinking-toggle" @click="emit('toggle-thinking', idx)">
            <span class="thinking-chevron" :class="{ expanded: isThinkingExpanded }">&#9654;</span>
            <span class="thinking-label">
              {{ msg.reasoningDone ? getRandomThinkingDonePhrase() : currentThinkingPhrase }} ({{ getThinkingDuration(msg) }})
            </span>
          </button>
          <div v-show="isThinkingExpanded" class="thinking-content markdown-body" v-html="msg.renderedReasoning"></div>
        </div>
        <!-- 回答内容 -->
        <div
          v-if="msg.content || !msg.reasoning"
          class="markdown-body"
          v-html="msg.renderedContent"
          @click="emit('copy-code', $event)"
        ></div>
        <!-- 操作按钮 -->
        <div v-if="isComplete && msg.content" class="message-actions">
          <button class="action-btn" @click="emit('regenerate', idx)" title="重新回答">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
            <span>重新回答</span>
          </button>
          <button class="action-btn" @click="handleCopyMarkdown" title="复制Markdown">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            <span>{{ copyLabel }}</span>
          </button>
        </div>
      </div>
    </template>
  </div>
</template>
