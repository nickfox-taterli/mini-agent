<script setup>
import { computed } from 'vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  message: { type: Object, default: null },
  thinkingDuration: { type: String, default: '' }
})

const emit = defineEmits(['close'])

function formatJSON(obj) {
  if (typeof obj === 'string') {
    try {
      const parsed = JSON.parse(obj)
      return JSON.stringify(parsed, null, 2)
    } catch {
      return obj
    }
  }
  return JSON.stringify(obj, null, 2)
}

const processSteps = computed(() => {
  const msg = props.message
  if (!msg || msg.role !== 'assistant') return []

  if (Array.isArray(msg.processTimeline) && msg.processTimeline.length > 0) {
    return msg.processTimeline.map((step) => {
      if (step.type === 'reasoning') {
        return {
          type: 'reasoning',
          title: '思考',
          content: step.rendered || ''
        }
      }
      if (step.type === 'content') {
        return {
          type: 'answer',
          title: '输出',
          content: step.rendered || ''
        }
      }
      return {
        type: 'tool',
        title: step.display_name || step.tool_name || '工具调用',
        toolCall: step
      }
    })
  }

  const steps = []

  if (msg.reasoning) {
    steps.push({
      type: 'reasoning',
      title: '思考',
      content: msg.renderedReasoning || ''
    })
  }

  const tools = (msg.toolCalls || []).map((tc, index) => ({ ...tc, _index: index }))
  tools.sort((a, b) => {
    const ta = typeof a.startTime === 'number' ? a.startTime : Number.MAX_SAFE_INTEGER
    const tb = typeof b.startTime === 'number' ? b.startTime : Number.MAX_SAFE_INTEGER
    if (ta !== tb) return ta - tb
    return a._index - b._index
  })

  for (const tc of tools) {
    steps.push({
      type: 'tool',
      title: tc.display_name || tc.tool_name || '工具调用',
      toolCall: tc
    })
  }

  if (msg.content) {
    steps.push({
      type: 'answer',
      title: '回答',
      content: msg.renderedContent || ''
    })
  }

  return steps
})

function statusLabel(toolCall) {
  const s = toolCall?.status
  if (s === 'running') return '执行中...'
  if (s === 'error') return '执行失败'
  return '已完成'
}
</script>

<template>
  <Transition name="slide-panel">
    <div v-if="open" class="detail-panel">
      <div class="detail-panel-header">
        <span class="detail-panel-title">完整过程</span>
        <button class="detail-panel-close" @click="emit('close')" title="关闭">×</button>
      </div>
      <div class="detail-panel-body">
        <div v-if="!processSteps.length" class="detail-panel-empty">暂无可展示过程</div>

        <div v-for="(step, idx) in processSteps" :key="idx" class="timeline-step">
          <div class="timeline-dot" :class="'timeline-dot-' + step.type"></div>
          <div class="timeline-content">
            <div class="timeline-title">{{ step.title }}</div>

            <template v-if="step.type === 'reasoning'">
              <div class="detail-panel-meta">用时: {{ thinkingDuration }}</div>
              <div class="markdown-body" v-html="step.content"></div>
            </template>

            <template v-else-if="step.type === 'tool'">
              <div class="detail-panel-meta">
                <span :class="['tool-status-badge', 'tool-status-' + step.toolCall.status]">
                  {{ statusLabel(step.toolCall) }}
                </span>
              </div>
              <div class="detail-section">
                <div class="detail-section-title">参数</div>
                <pre class="detail-json"><code>{{ formatJSON(step.toolCall.arguments) }}</code></pre>
              </div>
              <div v-if="step.toolCall.result" class="detail-section">
                <div class="detail-section-title">结果</div>
                <pre class="detail-json"><code>{{ formatJSON(step.toolCall.result) }}</code></pre>
              </div>
            </template>

            <template v-else>
              <div class="markdown-body" v-html="step.content"></div>
            </template>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.detail-panel {
  position: fixed;
  top: 0;
  right: 0;
  width: 480px;
  max-width: 90vw;
  height: 100vh;
  background: var(--bg-secondary, #252526);
  border-left: 1px solid var(--border, #3E3E3E);
  z-index: 40;
  display: flex;
  flex-direction: column;
  box-shadow: -4px 0 20px rgba(0, 0, 0, 0.3);
}

.detail-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--border, #3E3E3E);
  flex-shrink: 0;
}

.detail-panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary, #E0E0E0);
}

.detail-panel-close {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: var(--text-muted, #6A6A6A);
  font-size: 20px;
  cursor: pointer;
  border-radius: 6px;
  transition: background 0.15s, color 0.15s;
}

.detail-panel-close:hover {
  background: var(--bg-tertiary, #2D2D2D);
  color: var(--text-primary, #E0E0E0);
}

.detail-panel-body {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
}

.detail-panel-empty {
  font-size: 13px;
  color: var(--text-muted, #6A6A6A);
}

.timeline-step {
  position: relative;
  display: flex;
  gap: 12px;
  padding-bottom: 18px;
}

.timeline-step:not(:last-child)::after {
  content: '';
  position: absolute;
  left: 5px;
  top: 16px;
  bottom: -4px;
  width: 1px;
  background: var(--border, #3E3E3E);
}

.timeline-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-top: 6px;
  flex-shrink: 0;
}

.timeline-dot-reasoning {
  background: var(--accent, #4FC3F7);
}

.timeline-dot-tool {
  background: #66BB6A;
}

.timeline-dot-answer {
  background: #f7b84f;
}

.timeline-content {
  flex: 1;
  min-width: 0;
}

.timeline-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary, #E0E0E0);
  margin-bottom: 8px;
}

.detail-panel-meta {
  font-size: 12px;
  color: var(--text-muted, #6A6A6A);
  margin-bottom: 12px;
}

.tool-status-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 10px;
  font-size: 12px;
  font-weight: 500;
}

.tool-status-running {
  background: rgba(102, 187, 106, 0.15);
  color: #66BB6A;
}

.tool-status-completed {
  background: rgba(79, 195, 247, 0.15);
  color: #4FC3F7;
}

.tool-status-error {
  background: rgba(255, 82, 82, 0.15);
  color: #ff5252;
}

.detail-section {
  margin-bottom: 12px;
}

.detail-section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary, #9E9E9E);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 8px;
}

.detail-json {
  background: var(--bg-code, #1A1A1A);
  border: 1px solid var(--border, #3E3E3E);
  border-radius: 8px;
  padding: 12px 16px;
  overflow-x: auto;
  font-family: var(--font-mono, 'SF Mono', 'Fira Code', monospace);
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--text-primary, #E0E0E0);
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}

.detail-json code {
  font-family: inherit;
}

.slide-panel-enter-active,
.slide-panel-leave-active {
  transition: transform 0.25s ease, opacity 0.25s ease;
}

.slide-panel-enter-from,
.slide-panel-leave-to {
  transform: translateX(100%);
  opacity: 0;
}

@media (max-width: 768px) {
  .detail-panel {
    width: 100vw;
    max-width: 100vw;
  }

  .detail-panel-header {
    padding: 12px 16px;
  }

  .detail-panel-body {
    padding: 12px 16px;
  }

  .detail-panel-close {
    width: 44px;
    height: 44px;
  }

  .detail-json {
    font-size: 12px;
    padding: 10px 12px;
  }

  .detail-section-title {
    font-size: 11px;
  }
}
</style>
