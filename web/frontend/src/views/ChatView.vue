<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { api, createEventSource } from '../api'
import type { ChatMessage } from '../api/types'

const messages = ref<ChatMessage[]>([])
const input = ref('')
const sessionId = ref('sess-1')
const sending = ref(false)
const messagesEl = ref<HTMLElement>()
let eventSource: EventSource | null = null

onMounted(async () => {
  messages.value = await api.listChatMessages(sessionId.value)
  await nextTick()
  scrollToBottom()

  // Connect SSE for real-time responses
  eventSource = createEventSource()
  eventSource.addEventListener('message', (e) => {
    try {
      const data = JSON.parse(e.data)
      messages.value.push({
        id: data.id || `msg-${Date.now()}`,
        session_id: sessionId.value,
        project_id: data.project_id || '',
        role: 'assistant',
        content: data.text || data.content || JSON.stringify(data),
        artifacts: [],
        metadata: data,
        created_at: new Date().toISOString(),
      })
      nextTick(scrollToBottom)
    } catch { /* ignore malformed SSE */ }
  })
  eventSource.addEventListener('done', (e) => {
    try {
      const data = JSON.parse(e.data)
      messages.value.push({
        id: data.id || `msg-${Date.now()}`,
        session_id: sessionId.value,
        project_id: data.project_id || '',
        role: 'system',
        content: data.text || data.message || 'Task completed',
        artifacts: [],
        metadata: data,
        created_at: new Date().toISOString(),
      })
      nextTick(scrollToBottom)
    } catch { /* ignore */ }
  })
  eventSource.addEventListener('confirmation', (e) => {
    try {
      const data = JSON.parse(e.data)
      messages.value.push({
        id: data.id || `msg-${Date.now()}`,
        session_id: sessionId.value,
        project_id: data.project_id || '',
        role: 'system',
        content: `Confirmation needed: ${data.text || data.message || ''}`,
        artifacts: [],
        metadata: { ...data, requires_confirmation: true },
        created_at: new Date().toISOString(),
      })
      nextTick(scrollToBottom)
    } catch { /* ignore */ }
  })
  eventSource.onerror = () => {
    // Auto-reconnect is handled by EventSource natively
  }
})

onUnmounted(() => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
})

function scrollToBottom() {
  if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
}

async function send() {
  if (!input.value.trim() || sending.value) return
  sending.value = true
  const content = input.value
  input.value = ''

  messages.value.push({
    id: `msg-${Date.now()}`, session_id: sessionId.value, project_id: '', role: 'user',
    content, artifacts: [], metadata: {}, created_at: new Date().toISOString(),
  })
  await nextTick()
  scrollToBottom()

  try {
    await api.sendChatMessage(sessionId.value, content)
  } catch (err: any) {
    messages.value.push({
      id: `msg-${Date.now()}`, session_id: sessionId.value, project_id: '', role: 'system',
      content: `Failed to send: ${err.message}`, artifacts: [], metadata: {}, created_at: new Date().toISOString(),
    })
    nextTick(scrollToBottom)
  }
  sending.value = false
}

async function confirmAction(msg: ChatMessage, approved: boolean) {
  const meta = msg.metadata || {}
  try {
    await api.sendConfirmation(
      meta.confirmation_id || meta.id || '',
      meta.project_id || '',
      meta.task_id || '',
      meta.attempt_id || '',
      meta.action_id || '',
      approved,
      approved ? 'Approved via web console' : 'Rejected via web console',
    )
    // Replace the confirmation message with result
    const idx = messages.value.indexOf(msg)
    if (idx >= 0) {
      messages.value[idx] = {
        ...msg,
        role: 'system',
        content: approved ? 'Approved' : 'Rejected',
        metadata: { ...meta, resolved: true },
      }
    }
  } catch (err: any) {
    messages.value.push({
      id: `msg-${Date.now()}`, session_id: sessionId.value, project_id: '', role: 'system',
      content: `Confirmation failed: ${err.message}`, artifacts: [], metadata: {}, created_at: new Date().toISOString(),
    })
  }
}

const roleColor: Record<string, string> = {
  user: 'text-indigo-300',
  assistant: 'text-gray-100',
  tool: 'text-yellow-300',
  system: 'text-gray-500',
}
const roleLabel: Record<string, string> = { user: 'You', assistant: 'Agent', tool: 'Tool', system: 'System' }
</script>

<template>
  <div class="flex h-[calc(100vh-8rem)] flex-col">
    <!-- Messages -->
    <div ref="messagesEl" class="flex-1 overflow-y-auto space-y-4 mb-4">
      <div
        v-for="msg in messages"
        :key="msg.id"
        :class="['rounded-lg border border-gray-800 bg-gray-900 p-4', msg.role === 'user' ? 'border-indigo-900/50' : '']"
      >
        <div class="flex items-center gap-2 mb-1">
          <span :class="['text-xs font-medium', roleColor[msg.role]]">{{ roleLabel[msg.role] }}</span>
          <span class="text-xs text-gray-600">{{ new Date(msg.created_at).toLocaleTimeString() }}</span>
          <span v-if="msg.metadata?.tool" class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-gray-400">{{ msg.metadata.tool }}</span>
          <span v-if="msg.metadata?.chain_level" class="rounded bg-indigo-900/50 px-1.5 py-0.5 text-xs text-indigo-300">{{ msg.metadata.chain_level }}</span>
        </div>
        <div :class="['text-sm whitespace-pre-wrap', roleColor[msg.role]]">{{ msg.content }}</div>
        <div v-if="msg.artifacts?.length" class="mt-2 flex gap-2">
          <span v-for="a in msg.artifacts" :key="a" class="rounded bg-gray-800 px-2 py-0.5 text-xs text-gray-400">📎 {{ a }}</span>
        </div>
        <!-- Confirmation buttons -->
        <div v-if="msg.metadata?.requires_confirmation && !msg.metadata?.resolved" class="mt-3 flex gap-2">
          <button @click="confirmAction(msg, true)" class="rounded bg-green-700 px-3 py-1.5 text-xs text-white hover:bg-green-600">Approve</button>
          <button @click="confirmAction(msg, false)" class="rounded bg-red-700 px-3 py-1.5 text-xs text-white hover:bg-red-600">Reject</button>
        </div>
        <div v-if="msg.metadata?.resolved" class="mt-1 text-xs text-gray-600">Resolved</div>
      </div>
      <div v-if="sending" class="text-sm text-gray-500 animate-pulse">Sending...</div>
    </div>

    <!-- Input -->
    <div class="flex gap-2">
      <input
        v-model="input"
        @keydown.enter="send"
        placeholder="Type a message..."
        class="flex-1 rounded-lg border border-gray-700 bg-gray-900 px-4 py-3 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
      />
      <button
        @click="send"
        :disabled="sending || !input.trim()"
        class="rounded-lg bg-indigo-600 px-6 py-3 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      >
        Send
      </button>
    </div>
  </div>
</template>
