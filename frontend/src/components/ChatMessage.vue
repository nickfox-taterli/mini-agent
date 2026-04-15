<script setup>
import { ref, computed, watch } from 'vue'

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

const emit = defineEmits(['copy-code', 'regenerate', 'open-thinking-detail', 'open-tool-detail', 'submit-ask'])

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

const askValues = ref({})
const askOtherValues = ref({})

watch(
  () => props.msg.askUser,
  (askUser) => {
    if (!askUser) {
      askValues.value = {}
      askOtherValues.value = {}
      return
    }
    const nextValues = {}
    const nextOther = {}
    for (const field of askUser.fields || []) {
      const options = Array.isArray(field.options) ? field.options : []
      if (field.input_type === 'select') {
        nextValues[field.key] = options[0] || ''
      } else if (field.input_type === 'multiselect') {
        nextValues[field.key] = options[0] ? [options[0]] : []
      } else {
        nextValues[field.key] = ''
      }
      nextOther[field.key] = ''
    }
    askValues.value = nextValues
    askOtherValues.value = nextOther
  },
  { immediate: true }
)

function getFieldAnswer(field) {
  if (field.input_type === 'multiselect') {
    const picked = Array.isArray(askValues.value[field.key]) ? [...askValues.value[field.key]] : []
    const other = (askOtherValues.value[field.key] || '').trim()
    if (other) picked.push(other)
    return picked
  }
  const value = String(askValues.value[field.key] || '').trim()
  if (value === '__other__') return String(askOtherValues.value[field.key] || '').trim()
  return value
}

const canSubmitAsk = computed(() => {
  const askUser = props.msg.askUser
  if (!askUser || askUser.answered) return false
  for (const field of askUser.fields || []) {
    if (!field.required) continue
    const value = getFieldAnswer(field)
    if (Array.isArray(value)) {
      if (value.length === 0) return false
    } else if (!String(value || '').trim()) {
      return false
    }
  }
  return true
})

const supplementRows = computed(() => {
  if (props.msg.role !== 'user') return null
  const raw = String(props.msg.content || '').trim()
  if (!raw.startsWith('补充信息')) return null
  const lines = raw.split('\n').map(line => line.trim()).filter(Boolean)
  const rows = []

  for (const line of lines) {
    if (!line.startsWith('- ')) continue
    const item = line.slice(2).trim()
    const idx = item.indexOf(':')
    if (idx <= 0) continue
    const label = item.slice(0, idx).trim()
    const value = item.slice(idx + 1).trim()
    if (!label || !value) continue
    rows.push({ label, value })
  }
  return rows.length ? rows : null
})

const singleSupplementRow = computed(() => {
  if (!supplementRows.value || supplementRows.value.length !== 1) return null
  return supplementRows.value[0]
})

const compactUserBubble = computed(() => {
  if (props.msg.role !== 'user') return false
  if (supplementRows.value) return false
  const raw = String(props.msg.content || '')
  if (!raw || raw.includes('\n')) return false
  if (/[`*_#[\]|]/.test(raw)) return false
  return raw.trim().length <= 20
})

function toggleMultiOption(fieldKey, option, checked) {
  const list = Array.isArray(askValues.value[fieldKey]) ? [...askValues.value[fieldKey]] : []
  const has = list.includes(option)
  if (checked && !has) list.push(option)
  if (!checked && has) list.splice(list.indexOf(option), 1)
  askValues.value[fieldKey] = list
}

function submitAsk() {
  const askUser = props.msg.askUser
  if (!askUser || askUser.answered) return
  if (!canSubmitAsk.value) return
  const values = {}
  for (const field of askUser.fields || []) {
    values[field.key] = getFieldAnswer(field)
  }
  const answers = (askUser.fields || []).map(field => ({
    key: field.key,
    label: (field.label || field.key || '').trim() || field.key,
    value: values[field.key]
  }))
  emit('submit-ask', {
    msgIdx: props.idx,
    values,
    answers
  })
}
</script>

<template>
  <div class="msg" :class="`msg-${msg.role}`">
    <template v-if="msg.role === 'user'">
      <div class="user-message">
        <div v-if="supplementRows" class="user-bubble user-supplement-bubble">
          <div class="user-supplement-title">补充信息</div>
          <div v-if="singleSupplementRow" class="user-supplement-single">
            <span class="user-supplement-label">{{ singleSupplementRow.label }}</span>
            <span class="user-supplement-value">{{ singleSupplementRow.value }}</span>
          </div>
          <div v-else class="user-supplement-list">
            <div v-for="(row, rowIdx) in supplementRows" :key="`${row.label}-${rowIdx}`" class="user-supplement-row">
              <span class="user-supplement-label">{{ row.label }}</span>
              <span class="user-supplement-value">{{ row.value }}</span>
            </div>
          </div>
        </div>
        <div
          v-else
          class="user-bubble markdown-body"
          :class="{ 'user-bubble-compact': compactUserBubble }"
          v-html="msg.renderedContent"
        ></div>
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
        <div v-if="msg.askUser && !msg.askUser.answered" class="ask-user-card">
          <div class="ask-user-card-title">请补充信息</div>
          <div v-if="msg.askUser.title" class="ask-user-card-heading">{{ msg.askUser.title }}</div>
          <div v-if="msg.askUser.description" class="ask-user-card-desc">{{ msg.askUser.description }}</div>
          <div class="ask-user-form">
            <div v-for="field in msg.askUser.fields || []" :key="field.key" class="ask-user-field-block">
              <label class="ask-user-label">{{ field.label }}</label>
              <select
                v-if="field.input_type === 'select'"
                v-model="askValues[field.key]"
                class="ask-user-input"
              >
                <option v-for="option in field.options || []" :key="option" :value="option">{{ option }}</option>
                <option value="__other__">其他(手动输入)</option>
              </select>
              <div v-else-if="field.input_type === 'multiselect'" class="ask-user-multiselect">
                <label v-for="option in field.options || []" :key="option" class="ask-user-check-item">
                  <input
                    type="checkbox"
                    :checked="Array.isArray(askValues[field.key]) && askValues[field.key].includes(option)"
                    @change="toggleMultiOption(field.key, option, $event.target.checked)"
                  />
                  <span>{{ option }}</span>
                </label>
              </div>
              <input
                v-if="field.input_type === 'text'"
                v-model="askValues[field.key]"
                class="ask-user-input"
                type="text"
                :placeholder="field.placeholder || '请输入补充信息'"
              />
              <input
                v-if="field.input_type === 'select' && askValues[field.key] === '__other__'"
                v-model="askOtherValues[field.key]"
                class="ask-user-input ask-user-other-input"
                type="text"
                :placeholder="field.placeholder || '请输入其他选项'"
              />
              <div v-if="field.input_type === 'multiselect'" class="ask-user-other-wrap">
                <label class="ask-user-check-item">
                  <span>其他(手动输入)</span>
                </label>
                <input
                  v-model="askOtherValues[field.key]"
                  class="ask-user-input ask-user-other-input"
                  type="text"
                  :placeholder="field.placeholder || '请输入其他选项'"
                />
              </div>
            </div>
            <button class="ask-user-submit" :disabled="!canSubmitAsk || loading" @click="submitAsk">
              {{ msg.askUser.submit_label || '提交补充信息' }}
            </button>
          </div>
        </div>
        <div v-else-if="msg.askUser && msg.askUser.answered && msg.askUser.answer" class="ask-user-answered">
          已补充: {{ msg.askUser.answer }}
        </div>
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
