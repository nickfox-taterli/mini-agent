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

export function useChatStream(apiBase, renderMarkdown) {
  // 状态
  const loading = ref(false)
  const toolCalling = ref(false)
  const toolCallingName = ref('')
  const workingHard = ref(false)
  const currentThinkingPhrase = ref('')
  const backends = ref([])
  const selectedBackendId = ref('')

  // 内部状态
  let toolCallStartTime = 0
  let workingHardTimer = null
  let lastOutputTime = 0
  let thinkingPhraseTimer = null
  let streamController = null
  const now = ref(Date.now())
  let timerInterval = null

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
      const res = await fetch(`${apiBase}/api/backends`)
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

  async function sendMessage({ input, messages, attachedFile, textareaRef, expandedThinking, saveCurrentMessages, generateTitleAsync, currentConversationId }) {
    if (loading.value) { stopGeneration(); return }
    if (!input.value.trim() && !attachedFile.value) return

    const userText = input.value.trim()
    input.value = ''
    await nextTick()
    if (textareaRef.value) textareaRef.value.style.height = 'auto'

    let finalText = userText
    if (attachedFile.value) {
      finalText += `\n\n[附件: ${attachedFile.value.name}](${attachedFile.value.url})`
      attachedFile.value = null
    }

    const userMsg = reactive({
      role: 'user', content: finalText,
      renderedContent: renderMarkdown(finalText)
    })
    messages.value.push(userMsg)
    saveCurrentMessages()
    generateTitleAsync(currentConversationId.value)

    const assistant = reactive({
      role: 'assistant', reasoning: '', content: '', reasoningDone: false,
      thinkingDuration: null, thinkingStartTime: Date.now(), retrying: null,
      renderedContent: '', renderedReasoning: ''
    })
    messages.value.push(assistant)
    await nextTick()
    window.scrollTo(0, document.body.scrollHeight)

    loading.value = true
    startTimer()
    startWorkingHardTimer()
    streamController = new AbortController()

    try {
      const res = await fetch(`${apiBase}/api/chat/stream`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ backend_id: selectedBackendId.value || undefined, messages: toChatMessages(messages) }),
        signal: streamController.signal
      })

      if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`)

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
            assistant.reasoning += payload.delta || ''
            assistant.renderedReasoning = renderMarkdown(assistant.reasoning)
            if (wasEmpty && assistant.reasoning) startThinkingPhraseCycle()
            await scrollToBottom()
          }

          if (event === 'tool_start') {
            resetWorkingHard(); toolCallingName.value = payload.tool_name || ''
            toolCalling.value = true; toolCallStartTime = Date.now(); await scrollToBottom()
          }

          if (event === 'tool_end') {
            resetWorkingHard()
            const elapsed = Date.now() - toolCallStartTime
            await new Promise(resolve => setTimeout(resolve, Math.max(0, 2000 - elapsed)))
            toolCalling.value = false; toolCallingName.value = ''
            startWorkingHardTimer(); await scrollToBottom()
          }

          if (event === 'content') {
            resetWorkingHard(); startWorkingHardTimer()
            assistant.reasoningDone = true
            assistant.content += payload.delta || ''
            assistant.renderedContent = renderMarkdown(assistant.content)
            assistant.retrying = null; await scrollToBottom()
          }

          if (event === 'retrying') {
            resetWorkingHard()
            assistant.retrying = { attempt: payload.attempt, maxAttempts: payload.max_attempts, delaySeconds: payload.delay_seconds }
            await scrollToBottom()
          }

          if (event === 'error') {
            resetWorkingHard()
            assistant.content += `\n[Error] ${payload.message || 'stream error'}`
            assistant.reasoningDone = true; doneReceived = true
            assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
            assistant.renderedContent = renderMarkdown(assistant.content)
            await scrollToBottom(); break
          }

          if (event === 'done') {
            resetWorkingHard(); assistant.reasoningDone = true
            assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
            assistant.renderedContent = renderMarkdown(assistant.content)
            doneReceived = true; break
          }
        }
        if (doneReceived) { await reader.cancel(); break }
      }
    } catch (err) {
      assistant.content += `\n[Error] ${err.message || 'request failed'}`
      assistant.thinkingDuration = (Date.now() - assistant.thinkingStartTime) / 1000
      assistant.renderedContent = renderMarkdown(assistant.content)
    } finally {
      stopTimer(); stopThinkingPhraseCycle(); resetWorkingHard()
      loading.value = false; streamController = null
      await scrollToBottom(); saveCurrentMessages()
    }
  }

  function cleanup() {
    stopTimer()
    stopThinkingPhraseCycle()
    if (workingHardTimer) { clearTimeout(workingHardTimer); workingHardTimer = null }
  }

  return {
    loading, toolCalling, toolCallingName, workingHard, currentThinkingPhrase,
    backends, selectedBackendId, now,
    sendMessage, stopGeneration, loadBackends, cleanup,
    getRandomThinkingDonePhrase, getThinkingDuration
  }
}
