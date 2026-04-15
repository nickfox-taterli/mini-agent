import { ref, reactive, nextTick } from 'vue'

// 思考循环短语
const THINKING_PHRASES = [
  '正在展开认知回路...', '正在同步当前任务世界线...', '推理核心升温中...',
  '正在装填策略模块...', '正在编译新的思考分支...', '正在校准目标与约束...',
  '正在重组知识碎片...', '正在加载高维理解补丁...', '正在收束可能性分支...',
  '正在执行脑内预演...', '正在唤醒备用推理核心...', '正在构建本轮作战方案...',
  '正在对齐上下文信号...', '正在生成下一阶段决策...', '正在完成逻辑链闭环...'
]
const THINKING_DONE_PHRASES = ['大功告成']

const DEFAULT_TOKEN_ESTIMATE_COEFFICIENT = 1

function parseTokenEstimateCoefficient(value) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed <= 0) return DEFAULT_TOKEN_ESTIMATE_COEFFICIENT
  return parsed
}

function estimateTokenCountFromText(text, coefficient) {
  if (!text) return 0
  const utf8Bytes = new TextEncoder().encode(text).length
  const baseTokenEstimate = utf8Bytes / 4
  return Math.max(0, Math.round(baseTokenEstimate * coefficient))
}

export function useChatStream(apiBase, renderMarkdown, authHeaders) {
  // 状态
  const loading = ref(false)
  const conversationStreaming = ref(false)
  const toolCalling = ref(false)
  const toolCallingName = ref('')
  const workingHard = ref(false)
  const currentThinkingPhrase = ref('')
  const backends = ref([])
  const selectedBackendId = ref('')
  const tokenEstimateCoefficient = parseTokenEstimateCoefficient(import.meta.env.VITE_TOKEN_ESTIMATE_COEFFICIENT)
  const tokenStats = reactive({
    estimatedTokens: 0,
    tokensPerSecond: 0,
    coefficient: tokenEstimateCoefficient
  })

  // 内部状态
  let toolCallStartTime = 0
  let workingHardTimer = null
  let lastOutputTime = 0
  let thinkingPhraseTimer = null
  let streamController = null
  let tokenStreamStartTime = 0
  const now = ref(Date.now())
  let timerInterval = null

  function resetTokenStats() {
    tokenStats.estimatedTokens = 0
    tokenStats.tokensPerSecond = 0
    tokenStreamStartTime = 0
  }

  function refreshTokenStats(assistant) {
    const fullText = `${assistant.reasoning || ''}${assistant.content || ''}`
    tokenStats.estimatedTokens = estimateTokenCountFromText(fullText, tokenEstimateCoefficient)
    if (!tokenStreamStartTime || tokenStats.estimatedTokens <= 0) {
      tokenStats.tokensPerSecond = 0
      assistant.tokenTotal = tokenStats.estimatedTokens
      assistant.tokenPerSecond = tokenStats.tokensPerSecond
      return
    }
    const elapsedSeconds = (Date.now() - tokenStreamStartTime) / 1000
    if (elapsedSeconds <= 0) {
      tokenStats.tokensPerSecond = 0
      assistant.tokenTotal = tokenStats.estimatedTokens
      assistant.tokenPerSecond = tokenStats.tokensPerSecond
      return
    }
    tokenStats.tokensPerSecond = Number((tokenStats.estimatedTokens / elapsedSeconds).toFixed(2))
    assistant.tokenTotal = tokenStats.estimatedTokens
    assistant.tokenPerSecond = tokenStats.tokensPerSecond
  }

  // SSE 解析
  function parseSSEBlock(block) {
    const lines = block.split('\n')
    let event = 'message'
    const dataLines = []
    for (const line of lines) {
      if (line.startsWith('event:')) event = line.slice(6).trim()
      if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
    }
    const raw = dataLines.join('\n')
    if (!raw) return { event, payload: null }
    try { return { event, payload: JSON.parse(raw) } } catch { return { event, payload: { message: raw } } }
  }

  function takeNextSSEBlock(buffer) {
    const normalized = buffer.replace(/\r\n/g, '\n')
    const idx = normalized.indexOf('\n\n')
    if (idx < 0) return { block: null, rest: normalized }
    return { block: normalized.slice(0, idx), rest: normalized.slice(idx + 2) }
  }

  // 计时器
  function startTimer() {
    now.value = Date.now()
    timerInterval = setInterval(() => { now.value = Date.now() }, 100)
  }

  function stopTimer() {
    if (timerInterval) { clearInterval(timerInterval); timerInterval = null }
  }

  // 思考短语轮播
  function startThinkingPhraseCycle() {
    currentThinkingPhrase.value = THINKING_PHRASES[Math.floor(Math.random() * THINKING_PHRASES.length)]
    if (thinkingPhraseTimer) clearInterval(thinkingPhraseTimer)
    thinkingPhraseTimer = setInterval(() => {
      currentThinkingPhrase.value = THINKING_PHRASES[Math.floor(Math.random() * THINKING_PHRASES.length)]
    }, 1000 + Math.random() * 4000)
  }

  function stopThinkingPhraseCycle() {
    if (thinkingPhraseTimer) { clearInterval(thinkingPhraseTimer); thinkingPhraseTimer = null }
  }

  function getRandomThinkingDonePhrase() {
    return THINKING_DONE_PHRASES[Math.floor(Math.random() * THINKING_DONE_PHRASES.length)]
  }

  // 努力干活提示
  function startWorkingHardTimer() {
    lastOutputTime = Date.now()
    if (workingHardTimer) clearTimeout(workingHardTimer)
    workingHardTimer = setTimeout(() => {
      if (!workingHard.value && !toolCalling.value && loading.value) workingHard.value = true
    }, 100)
  }

  function resetWorkingHard() {
    workingHard.value = false
    if (workingHardTimer) { clearTimeout(workingHardTimer); workingHardTimer = null }
    lastOutputTime = Date.now()
  }

  // 停止生成
  function stopGeneration() {
    if (streamController) { streamController.abort(); streamController = null }
  }

  function formatDuration(ms) {
    const totalSeconds = ms / 1000
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    if (minutes > 0) return `${minutes} 分钟 ${seconds.toFixed(2)} 秒`
    return `${seconds.toFixed(2)} 秒`
  }

  function getThinkingDuration(msg) {
    if (msg.thinkingDuration != null) return formatDuration(msg.thinkingDuration * 1000)
    if (msg.thinkingStartTime && loading.value) return formatDuration(now.value - msg.thinkingStartTime)
    return '...'
  }

  function isNearBottom() {
    const threshold = 120
    return window.innerHeight + window.scrollY >= document.body.scrollHeight - threshold
  }

  async function scrollToBottom() {
    await nextTick()
    if (isNearBottom()) window.scrollTo(0, document.body.scrollHeight)
  }

  // 加载后端列表
  async function loadBackends() {
    try {
      const res = await fetch(`${apiBase}/api/backends`, { headers: authHeaders() })
      if (!res.ok) return
      const data = await res.json()
      backends.value = data.backends || []
      const enabled = backends.value.find((b) => b.enabled)
      if (enabled) selectedBackendId.value = enabled.id
    } catch {}
  }

  // 发送消息
  function toChatMessages(messages) {
    return messages.value
      .filter((m) => m.role === 'user' || (m.role === 'assistant' && m.content.trim() !== ''))
      .map((m) => ({ role: m.role, content: m.content }))
  }

  // 流式处理核心逻辑
  async function _streamAssistant(assistant, messages, saveCurrentMessages, conversationId) {
    function appendTextStep(type, delta) {
      if (!delta) return
      const timeline = assistant.processTimeline || (assistant.processTimeline = [])
      const last = timeline[timeline.length - 1]
      if (last && last.type === type && !last.closed) {
        last.text += delta
        last.rendered = renderMarkdown(last.text)
        last.endTime = Date.now()
        return
      }
      timeline.push(reactive({
        type,
        text: delta,
        rendered: renderMarkdown(delta),
        startTime: Date.now(),
        endTime: Date.now(),
        closed: false
      }))
    }

    function appendToolStep(payload) {
      const timeline = assistant.processTimeline || (assistant.processTimeline = [])
      const existing = timeline.find((step) => step.type === 'tool' && step.call_id === payload.call_id)
      if (existing) {
        existing.tool_name = payload.tool_name
        existing.display_name = payload.display_name || payload.tool_name
        existing.arguments = payload.arguments || existing.arguments || ''
        existing.status = 'running'
        existing.result = null
        existing.endTime = null
        existing.closed = false
        return existing
      }
      const toolStep = reactive({
        type: 'tool',
        call_id: payload.call_id,
        tool_name: payload.tool_name,
        display_name: payload.display_name || payload.tool_name,
        arguments: payload.arguments || '',
        result: null,
        status: 'running',
        startTime: Date.now(),
        endTime: null,
        closed: false
      })
      timeline.push(toolStep)
      return toolStep
    }

    function closeCurrentTextStep(type) {
      const timeline = assistant.processTimeline || []
      const last = timeline[timeline.length - 1]
      if (!last || last.type !== type || last.closed) return
      last.endTime = Date.now()
      last.closed = true
    }

    loading.value = true
    conversationStreaming.value = true
    tokenStreamStartTime = Date.now()
    resetTokenStats()
    startTimer()
    startWorkingHardTimer()
    streamController = new AbortController()

    try {
      const res = await fetch(`${apiBase}/api/chat/stream`, {
        method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json' }),
        body: JSON.stringify({
          backend_id: selectedBackendId.value || undefined,
          conversation_id: conversationId || undefined,
          messages: toChatMessages(messages)
        }),
        signal: streamController.signal
      })

      if (!res.ok || !res.body) {
        if (res.status === 409) {
          conversationStreaming.value = true
          throw new Error('当前会话正在由其他页面生成, 请稍候')
        }
        throw new Error(`HTTP ${res.status}`)
      }

      const reader = res.body.getReader()
      const decoder = new TextDecoder('utf-8')
      let buffer = '', doneReceived = false

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        while (true) {
          const { block, rest } = takeNextSSEBlock(buffer)
          if (block === null) { buffer = rest; break }
          buffer = rest
          const { event, payload } = parseSSEBlock(block)
          if (!payload) continue

          if (event === 'reasoning') {
            resetWorkingHard(); startWorkingHardTimer()
            const wasEmpty = !assistant.reasoning
            const delta = payload.delta || ''
            assistant.reasoning += delta
            assistant.renderedReasoning = renderMarkdown(assistant.reasoning)
            appendTextStep('reasoning', delta)
            refreshTokenStats(assistant)
            if (wasEmpty && assistant.reasoning) startThinkingPhraseCycle()
            await scrollToBottom()
          }

          if (event === 'tool_start') {
            closeCurrentTextStep('reasoning')
            closeCurrentTextStep('content')
            resetWorkingHard()
            toolCallingName.value = payload.display_name || payload.tool_name || ''
            toolCalling.value = true; toolCallStartTime = Date.now()
            // 持久化工具调用到消息模型, 避免同 call_id 重复入栈.
            const existing = assistant.toolCalls.find(tc => tc.call_id === payload.call_id)
            if (existing) {
              existing.tool_name = payload.tool_name
              existing.display_name = payload.display_name || payload.tool_name
              existing.arguments = payload.arguments || existing.arguments || ''
              existing.status = 'running'
              existing.result = null
              existing.startTime = existing.startTime || Date.now()
              existing.endTime = null
            } else {
              assistant.toolCalls.push(reactive({
                call_id: payload.call_id,
                tool_name: payload.tool_name,
                display_name: payload.display_name || payload.tool_name,
                arguments: payload.arguments || '',
                result: null,
                status: 'running',
                startTime: Date.now(),
                endTime: null
              }))
            }
            appendToolStep(payload)
            await scrollToBottom()
          }

          if (event === 'tool_end') {
            resetWorkingHard()
            // 更新对应的工具调用条目
            const entry = assistant.toolCalls.find(tc => tc.call_id === payload.call_id)
            if (entry) {
              entry.result = payload.result
              entry.status = payload.result?.error ? 'error' : 'completed'
              entry.endTime = Date.now()
            }
            const toolStep = (assistant.processTimeline || []).find(
              (step) => step.type === 'tool' && step.call_id === payload.call_id
            )
            if (toolStep) {
              toolStep.result = payload.result
              toolStep.status = payload.result?.error ? 'error' : 'completed'
              toolStep.endTime = Date.now()
              toolStep.closed = true
            }
            const elapsed = Date.now() - toolCallStartTime
            await new Promise(resolve => setTimeout(resolve, Math.max(0, 2000 - elapsed)))
            toolCalling.value = false; toolCallingName.value = ''
            startWorkingHardTimer(); await scrollToBottom()
          }

          if (event === 'content') {
            resetWorkingHard(); startWorkingHardTimer()
            const delta = payload.delta || ''
            assistant.content += delta
            assistant.renderedContent = renderMarkdown(assistant.content)
            appendTextStep('content', delta)
            refreshTokenStats(assistant)
            assistant.retrying = null; await scrollToBottom()
          }

          if (event === 'retrying') {
            resetWorkingHard()
            assistant.retrying = { attempt: payload.attempt, maxAttempts: payload.max_attempts, delaySeconds: payload.delay_seconds }
            await scrollToBottom()
          }

          if (event === 'error') {
            closeCurrentTextStep('reasoning')
            closeCurrentTextStep('content')
            resetWorkingHard()
            assistant.content += `\n[Error] ${payload.message || 'stream error'}`
            assistant.reasoningDone = true; doneReceived = true
            assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
            assistant.renderedContent = renderMarkdown(assistant.content)
            refreshTokenStats(assistant)
            await scrollToBottom(); break
          }

          if (event === 'done') {
            closeCurrentTextStep('reasoning')
            closeCurrentTextStep('content')
            resetWorkingHard(); assistant.reasoningDone = true
            assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
            assistant.renderedContent = renderMarkdown(assistant.content)
            refreshTokenStats(assistant)
            doneReceived = true; break
          }
        }
        if (doneReceived) { await reader.cancel(); break }
      }
    } catch (err) {
      assistant.content += `\n[Error] ${err.message || 'request failed'}`
      assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
      assistant.renderedContent = renderMarkdown(assistant.content)
      refreshTokenStats(assistant)
    } finally {
      stopTimer(); stopThinkingPhraseCycle(); resetWorkingHard()
      loading.value = false; streamController = null
      await syncConversationState(conversationId)
      await scrollToBottom(); saveCurrentMessages()
    }
  }

  async function sendMessage({ input, messages, attachedFiles, textareaRef, expandedThinking, saveCurrentMessages, generateTitleAsync, currentConversationId }) {
    if (loading.value) { stopGeneration(); return }
    if (conversationStreaming.value) return
    if (!input.value.trim() && attachedFiles.value.length === 0) return

    const userText = input.value.trim()
    input.value = ''
    await nextTick()
    if (textareaRef.value) textareaRef.value.style.height = 'auto'

    let finalText = userText
    if (attachedFiles.value.length > 0) {
      for (const file of attachedFiles.value) {
        finalText += `\n\n[附件: ${file.name}](${file.url})`
      }
      attachedFiles.value = []
    }

    const userMsg = reactive({
      role: 'user', content: finalText,
      renderedContent: renderMarkdown(finalText),
      tokenTotal: null, tokenPerSecond: null
    })
    messages.value.push(userMsg)
    saveCurrentMessages()
    generateTitleAsync(currentConversationId.value)

    const assistant = reactive({
      role: 'assistant', reasoning: '', content: '', reasoningDone: false,
      thinkingDuration: null, thinkingStartTime: Date.now(), retrying: null,
      renderedContent: '', renderedReasoning: '', toolCalls: [], processTimeline: [],
      tokenTotal: null, tokenPerSecond: null
    })
    messages.value.push(assistant)
    await nextTick()
    window.scrollTo(0, document.body.scrollHeight)

    await _streamAssistant(assistant, messages, saveCurrentMessages, currentConversationId.value)
  }

  // 重新生成指定位置的助手回复
  async function regenerate({ idx, messages, saveCurrentMessages, expandedThinking, currentConversationId }) {
    if (loading.value) return
    if (conversationStreaming.value) return
    // 删除旧的助手消息
    messages.value.splice(idx, 1)
    // 创建新的助手消息占位
    const assistant = reactive({
      role: 'assistant', reasoning: '', content: '', reasoningDone: false,
      thinkingDuration: null, thinkingStartTime: Date.now(), retrying: null,
      renderedContent: '', renderedReasoning: '', toolCalls: [], processTimeline: [],
      tokenTotal: null, tokenPerSecond: null
    })
    messages.value.push(assistant)
    await nextTick()
    window.scrollTo(0, document.body.scrollHeight)

    // 重置该消息的思考展开状态
    expandedThinking.set(idx, false)

    await _streamAssistant(assistant, messages, saveCurrentMessages, currentConversationId.value)
  }

  async function syncConversationState(conversationId) {
    const id = (conversationId || '').trim()
    if (!id) {
      conversationStreaming.value = false
      return
    }
    try {
      const res = await fetch(`${apiBase}/api/conversations/${encodeURIComponent(id)}/state`, { headers: authHeaders() })
      if (!res.ok) return
      const data = await res.json()
      conversationStreaming.value = !!data.is_streaming
    } catch {}
  }

  function cleanup() {
    stopTimer()
    stopThinkingPhraseCycle()
    if (workingHardTimer) { clearTimeout(workingHardTimer); workingHardTimer = null }
  }

  return {
    loading, toolCalling, toolCallingName, workingHard, currentThinkingPhrase,
    backends, selectedBackendId, now, conversationStreaming, tokenStats,
    sendMessage, regenerate, stopGeneration, loadBackends, cleanup,
    getRandomThinkingDonePhrase, getThinkingDuration, syncConversationState
  }
}
