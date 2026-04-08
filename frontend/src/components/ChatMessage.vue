<script setup>
const props = defineProps({
  msg: { type: Object, required: true },
  idx: { type: Number, required: true },
  isLast: { type: Boolean, default: false },
  workingHard: { type: Boolean, default: false },
  toolCalling: { type: Boolean, default: false },
  toolCallingName: { type: String, default: '' },
  currentThinkingPhrase: { type: String, default: '' },
  isThinkingExpanded: { type: Boolean, default: false },
  getRandomThinkingDonePhrase: { type: Function, required: true },
  getThinkingDuration: { type: Function, required: true }
})

const emit = defineEmits(['toggle-thinking', 'copy-code'])
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
          v-if="msg.reasoningDone || !msg.reasoning"
          class="markdown-body"
          v-html="msg.renderedContent"
          @click="emit('copy-code', $event)"
        ></div>
      </div>
    </template>
  </div>
</template>
