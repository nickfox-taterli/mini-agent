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
  getRandomThinkingDonePhrase: { type: Function, required: true },
  getThinkingDuration: { type: Function, required: true }
})

const emit = defineEmits(['copy-code', 'regenerate', 'open-thinking-detail', 'open-tool-detail'])

const isComplete = computed(() => !props.isLast || !props.loading)
const copyLabel = ref('复制Markdown')

function handleCopyMarkdown() {
  navigator.clipboard.writeText(props.msg.content || '').then(() => {
    copyLabel.value = '已复制'
    setTimeout(() => { copyLabel.value = '复制Markdown' }, 2000)
  })
}

// 思考和工具调用标签互斥显示
// 有工具调用时隐藏思考标签
const showThinkingTag = computed(() => !!props.msg.reasoning)

const latestToolCall = computed(() => {
  const list = props.msg.toolCalls || []
  if (!list.length) return null
  const running = list.find(tc => tc.status === 'running')
  if (running) return { call: running, index: list.indexOf(running) }
  // 工具完成后不再展示绿色标签, 仅运行中展示
  return null
})

const showTagsRow = computed(() => {
  return showThinkingTag.value || !!latestToolCall.value || (props.toolCalling && props.isLast)
})
</script>

<template>
  <div class="msg" :class="`msg-${msg.role}`">
    <template v-if="msg.role === 'user'">
      <div class="user-message">
        <div class="user-bubble markdown-body" v-html="msg.renderedContent"></div>
        <div v-if="msg.content" class="message-actions user-message-actions">
          <button class="action-btn" @click="handleCopyMarkdown" title="复制Markdown">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
            <span>{{ copyLabel }}</span>
          </button>
        </div>
      </div>
    </template>
    <template v-else>
      <div class="assistant-message">
        <!-- 排队重试提示 -->
        <div v-if="msg.retrying" class="retrying-indicator">
          <span class="retrying-spinner"></span>
          <span>LLM服务排队中...</span>
        </div>
        <!-- 正在努力干活提示: 工具调用时不显示 -->
        <div v-if="workingHard && !msg.retrying && isLast && !toolCalling" class="working-hard-indicator">
          <span class="working-hard-spinner"></span>
          <span>正在非常努力干活...</span>
        </div>
        <!-- 紧凑标签行: 思考 + 工具调用 -->
        <div v-if="showTagsRow" class="tags-row">
          <!-- 思考标签: 与工具调用互斥,工具调用时隐藏 -->
          <button
            v-if="showThinkingTag"
            class="detail-tag thinking-tag"
            @click="emit('open-thinking-detail', idx)"
          >
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>
            </svg>
            <span>{{ msg.reasoningDone ? '思考了 ' + getThinkingDuration(msg) : currentThinkingPhrase }}</span>
          </button>
          <!-- 工具调用标签 -->
          <button
            v-if="latestToolCall"
            :key="latestToolCall.call.call_id || latestToolCall.call.display_name"
            class="detail-tag tool-tag"
            :class="{ 'tool-tag-running': latestToolCall.call.status === 'running' }"
            @click="emit('open-tool-detail', { msgIdx: idx, toolIdx: latestToolCall.index })"
          >
            <span v-if="latestToolCall.call.status === 'running'" class="tool-tag-spinner"></span>
            <svg v-else width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>
            </svg>
            <span>{{ latestToolCall.call.display_name }}</span>
          </button>
          <!-- 实时工具调用中(尚无持久化条目) -->
          <div v-if="toolCalling && isLast && !msg.toolCalls?.length" class="detail-tag tool-tag tool-tag-running">
            <span class="tool-tag-spinner"></span>
            <span>{{ toolCallingName || '...' }}</span>
          </div>
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
