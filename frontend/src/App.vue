<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'

const apiBase = import.meta.env.VITE_API_BASE || 'http://127.0.0.1:18888'

const input = ref('')
const messages = ref([])
const loading = ref(false)
const backends = ref([])
const selectedBackendId = ref('')
const chatScroll = ref(null)

const canSend = computed(() => input.value.trim() !== '' && !loading.value)

onMounted(async () => {
  try {
    const res = await fetch(`${apiBase}/api/backends`)
    if (!res.ok) return
    const data = await res.json()
    backends.value = data.backends || []
    const enabled = backends.value.find((b) => b.enabled)
    if (enabled) {
      selectedBackendId.value = enabled.id
    }
  } catch {
    // Ignore initial backend loading errors to keep UI minimal.
  }
})

function toChatMessages() {
  return messages.value
    .filter((m) => m.role === 'user' || (m.role === 'assistant' && m.content.trim() !== ''))
    .map((m) => ({ role: m.role, content: m.content }))
}

function parseSSEBlock(block) {
  const lines = block.split('\n')
  let event = 'message'
  const dataLines = []

  for (const line of lines) {
    if (line.startsWith('event:')) {
      event = line.slice(6).trim()
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trim())
    }
  }

  const raw = dataLines.join('\n')
  if (!raw) {
    return { event, payload: null }
  }

  try {
    return { event, payload: JSON.parse(raw) }
  } catch {
    return { event, payload: { message: raw } }
  }
}

async function scrollToBottom() {
  await nextTick()
  if (chatScroll.value) {
    chatScroll.value.scrollTop = chatScroll.value.scrollHeight
  }
}

async function sendMessage() {
  if (!canSend.value) return

  const userText = input.value.trim()
  input.value = ''

  messages.value.push({ role: 'user', content: userText })
  const assistant = { role: 'assistant', reasoning: '', content: '' }
  messages.value.push(assistant)
  await scrollToBottom()

  loading.value = true

  try {
    const res = await fetch(`${apiBase}/api/chat/stream`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        backend_id: selectedBackendId.value || undefined,
        messages: toChatMessages()
      })
    })

    if (!res.ok || !res.body) {
      throw new Error(`HTTP ${res.status}`)
    }

    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''
    let doneReceived = false

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      let separatorIndex = buffer.indexOf('\n\n')

      while (separatorIndex >= 0) {
        const block = buffer.slice(0, separatorIndex)
        buffer = buffer.slice(separatorIndex + 2)
        separatorIndex = buffer.indexOf('\n\n')

        const { event, payload } = parseSSEBlock(block)
        if (!payload) continue

        if (event === 'reasoning') {
          assistant.reasoning += payload.delta || ''
          await scrollToBottom()
        }

        if (event === 'content') {
          assistant.content += payload.delta || ''
          await scrollToBottom()
        }

        if (event === 'error') {
          const msg = payload.message || 'stream error'
          assistant.content += `\n[Error] ${msg}`
          doneReceived = true
          await scrollToBottom()
          break
        }

        if (event === 'done') {
          doneReceived = true
          break
        }
      }

      if (doneReceived) {
        await reader.cancel()
        break
      }
    }
  } catch (err) {
    assistant.content += `\n[Error] ${err.message || 'request failed'}`
  } finally {
    loading.value = false
    await scrollToBottom()
  }
}
</script>

<template>
  <div class="page">
    <div class="panel">
      <div ref="chatScroll" class="chat-area">
        <div v-for="(msg, idx) in messages" :key="idx" class="msg" :class="`msg-${msg.role}`">
          <template v-if="msg.role === 'user'">
            <div class="bubble user-bubble">{{ msg.content }}</div>
          </template>
          <template v-else>
            <div class="bubble assistant-bubble">
              <div v-if="msg.reasoning" class="reasoning">
                <div class="label">思考</div>
                <div class="text">{{ msg.reasoning }}</div>
              </div>
              <div class="answer">
                <div class="label">回答</div>
                <div class="text">{{ msg.content }}</div>
              </div>
            </div>
          </template>
        </div>
      </div>

      <div class="composer">
        <textarea
          v-model="input"
          class="input"
          placeholder="尽管问..."
          rows="2"
          :disabled="loading"
          @keydown.enter.exact.prevent="sendMessage"
        />
        <button class="send" :disabled="!canSend" @click="sendMessage">
          {{ loading ? '...' : '↑' }}
        </button>
      </div>

      <div class="meta">
        后端: {{ selectedBackendId || '未连接' }} | API: {{ apiBase }}
      </div>
    </div>
  </div>
</template>
