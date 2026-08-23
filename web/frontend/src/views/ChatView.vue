<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { RouterLink } from 'vue-router'
import { api, createEventSource } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { ChatMessage, Session, SessionContext } from '../api/types'

// --- Session state ---
const sessions = ref<Session[]>([])
const selectedSession = ref<Session | null>(null)
const sessionContext = ref<SessionContext | null>(null)
const statusFilter = ref<string>('')
const sessionLoading = ref(true)
const acting = ref(false)
const showActions = ref(false)

// --- Chat state ---
const messages = ref<ChatMessage[]>([])
const input = ref('')
const sending = ref(false)
const messagesEl = ref<HTMLElement>()
let eventSource: EventSource | null = null

// --- Computed ---
const filteredSessions = computed(() => {
  if (!statusFilter.value) return sessions.value
  return sessions.value.filter(s => s.status === statusFilter.value)
})

const statusCounts = computed(() => {
  const counts: Record<string, number> = { '': sessions.value.length }
  for (const s of sessions.value) {
    counts[s.status] = (counts[s.status] || 0) + 1
  }
  return counts
})

// --- Helpers ---
function timeAgo(dateStr: string): string {
  if (!dateStr) return ''
  const ms = Date.now() - new Date(dateStr).getTime()
  const m = Math.floor(ms / 60000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

function channelIcon(type: string): string {
  const map: Record<string, string> = { cli: '▸', web: '◎', telegram: '✈', whatsapp: '⬡' }
  return map[type] || '○'
}

function truncate(s: string, len: number): string {
  if (!s) return ''
  return s.length > len ? s.slice(0, len) + '...' : s
}

// --- Session actions ---
async function refreshSessions() {
  sessions.value = await api.listSessions()
}

async function selectSession(session: Session) {
  selectedSession.value = session
  sessionContext.value = null
  messages.value = []
  try { sessionContext.value = await api.getSessionContext(session.id) } catch {}
}

async function runAction(action: 'pause' | 'resume' | 'expire' | 'delete') {
  if (!selectedSession.value || acting.value) return
  acting.value = true
  showActions.value = false
  try {
    if (action === 'pause') selectedSession.value = await api.pauseSession(selectedSession.value.id)
    else if (action === 'resume') selectedSession.value = await api.resumeSession(selectedSession.value.id)
    else if (action === 'expire') selectedSession.value = await api.expireSession(selectedSession.value.id)
    else if (action === 'delete') {
      await api.deleteSession(selectedSession.value.id)
      selectedSession.value = null
      sessionContext.value = null
    }
    await refreshSessions()
  } catch (e: any) {
    alert('Action failed: ' + e.message)
  } finally {
    acting.value = false
  }
}

function startNewChat() {
  selectedSession.value = null
  sessionContext.value = null
  messages.value = []
  input.value = ''
  nextTick(() => document.querySelector<HTMLInputElement>('#chat-input')?.focus())
}

// --- Chat ---
function pushSSEMessage(e: MessageEvent, role: ChatMessage['role'], fallback: string) {
  try {
    const data = JSON.parse(e.data)
    const payload = data.payload || {}
    messages.value.push({
      id: data.id || `msg-${Date.now()}`,
      session_id: selectedSession.value?.id || '',
      project_id: data.project_id || payload.project_id || '',
      role,
      content: data.text || data.message || payload.text || fallback,
      artifacts: [],
      metadata: { ...payload, ...data },
      created_at: new Date().toISOString(),
    })
    nextTick(scrollToBottom)
  } catch { /* ignore malformed SSE */ }
}

function scrollToBottom() {
  if (messagesEl.value) messagesEl.value.scrollTop = messagesEl.value.scrollHeight
}

async function send() {
  if (!input.value.trim() || sending.value) return
  sending.value = true
  const content = input.value
  input.value = ''

  messages.value.push({
    id: `msg-${Date.now()}`, session_id: selectedSession.value?.id || '', project_id: '', role: 'user',
    content, artifacts: [], metadata: {}, created_at: new Date().toISOString(),
  })
  await nextTick()
  scrollToBottom()

  try {
    await api.sendChatMessage(selectedSession.value?.id || '', content)
    if (!selectedSession.value) {
      await refreshSessions()
      const active = sessions.value.find(s => s.status === 'ACTIVE')
      if (active) selectSession(active)
    }
  } catch (err: any) {
    messages.value.push({
      id: `msg-${Date.now()}`, session_id: '', project_id: '', role: 'system',
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
      meta.confirmation_id || meta.id || '', meta.project_id || '',
      meta.task_id || '', meta.attempt_id || '',
      meta.action_id || '', approved,
      approved ? 'Approved via web console' : 'Rejected via web console',
    )
    const idx = messages.value.indexOf(msg)
    if (idx >= 0) {
      messages.value[idx] = { ...msg, role: 'system', content: approved ? 'Approved' : 'Rejected', metadata: { ...meta, resolved: true } }
    }
  } catch (err: any) {
    messages.value.push({
      id: `msg-${Date.now()}`, session_id: '', project_id: '', role: 'system',
      content: `Confirmation failed: ${err.message}`, artifacts: [], metadata: {}, created_at: new Date().toISOString(),
    })
  }
}

const roleColor: Record<string, string> = {
  user: 'text-indigo-300', assistant: 'text-gray-100', tool: 'text-yellow-300', system: 'text-gray-500',
}
const roleLabel: Record<string, string> = { user: 'You', assistant: 'Agent', tool: 'Tool', system: 'System' }

// --- Lifecycle ---
onMounted(async () => {
  await refreshSessions()
  sessionLoading.value = false
  const active = sessions.value.find(s => s.status === 'ACTIVE')
  if (active) selectSession(active)

  eventSource = createEventSource()
  eventSource.addEventListener('message', (e) => pushSSEMessage(e, 'assistant', 'Message received'))
  eventSource.addEventListener('progress', (e) => pushSSEMessage(e, 'assistant', 'Progress update'))
  eventSource.addEventListener('done', (e) => pushSSEMessage(e, 'system', 'Task completed'))
  eventSource.addEventListener('blocked', (e) => pushSSEMessage(e, 'system', 'Blocked'))
  eventSource.addEventListener('confirmation', (e) => {
    try {
      const data = JSON.parse(e.data)
      const payload = data.payload || {}
      messages.value.push({
        id: data.id || `msg-${Date.now()}`,
        session_id: selectedSession.value?.id || '',
        project_id: data.project_id || payload.project_id || '',
        role: 'system',
        content: `Confirmation needed: ${data.text || data.message || payload.text || ''}`,
        artifacts: [],
        metadata: { ...payload, ...data, requires_confirmation: true },
        created_at: new Date().toISOString(),
      })
      nextTick(scrollToBottom)
    } catch {}
  })
  eventSource.onerror = () => {}
})

onUnmounted(() => {
  if (eventSource) { eventSource.close(); eventSource = null }
})
</script>

<template>
  <div class="flex gap-4 h-[calc(100vh-8rem)]">
    <!-- Left: Session list -->
    <div class="w-80 flex-shrink-0 flex flex-col gap-3 overflow-hidden">
      <button @click="startNewChat" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500 transition-colors">+ New Chat</button>

      <!-- Status filter -->
      <div class="flex flex-wrap gap-1">
        <button
          v-for="(label, key) in ({ '': 'All', 'ACTIVE': 'Active', 'PAUSED': 'Paused', 'COMPLETED': 'Done', 'EXPIRED': 'Expired' } as Record<string, string>)"
          :key="key"
          @click="statusFilter = key"
          :class="['rounded px-2 py-1 text-xs transition-colors', statusFilter === key ? 'bg-indigo-600 text-white' : 'bg-gray-800 text-gray-400 hover:bg-gray-700']"
        >{{ label }} ({{ statusCounts[key] || 0 }})</button>
      </div>

      <!-- Session list -->
      <div v-if="sessionLoading" class="text-gray-500 text-sm">Loading...</div>
      <div v-else class="flex-1 overflow-y-auto space-y-2 pr-1">
        <div v-if="filteredSessions.length === 0" class="text-center py-8 text-gray-600 text-sm">No sessions</div>
        <div
          v-for="s in filteredSessions"
          :key="s.id"
          @click="selectSession(s)"
          :class="['cursor-pointer rounded-lg border p-3 transition-colors', selectedSession?.id === s.id ? 'border-indigo-600 bg-indigo-900/20' : 'border-gray-800 bg-gray-900 hover:border-gray-700']"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-sm font-medium text-gray-200 truncate">{{ s.title || 'Untitled session' }}</span>
            <StatusBadge :status="s.status" />
          </div>
          <div class="mt-1 flex items-center gap-2 text-xs text-gray-500">
            <span>{{ channelIcon(s.channel_type) }} {{ s.channel_type }}</span>
            <span>{{ timeAgo(s.last_active_at) }}</span>
          </div>
          <div class="mt-1 text-xs text-gray-600 font-mono truncate">{{ truncate(s.id, 24) }}</div>
        </div>
      </div>
    </div>

    <!-- Right: Chat window -->
    <div class="flex-1 flex flex-col rounded-lg border border-gray-800 bg-gray-900 overflow-hidden">
      <!-- No session selected -->
      <template v-if="!selectedSession">
        <div ref="messagesEl" class="flex-1 overflow-y-auto p-4 space-y-3">
          <div v-if="messages.length === 0" class="flex h-full items-center justify-center">
            <div class="text-center">
              <p class="text-gray-500 text-sm">Select a session or start a new chat</p>
              <p class="text-gray-600 text-xs mt-1">Messages will appear here</p>
            </div>
          </div>
          <div v-else v-for="msg in messages" :key="msg.id" :class="['rounded-lg border border-gray-800 bg-gray-800/50 p-3', msg.role === 'user' ? 'border-indigo-900/30' : '']">
            <div class="flex items-center gap-2 mb-1">
              <span :class="['text-xs font-medium', roleColor[msg.role]]">{{ roleLabel[msg.role] }}</span>
              <span class="text-xs text-gray-600">{{ new Date(msg.created_at).toLocaleTimeString() }}</span>
            </div>
            <div :class="['text-sm whitespace-pre-wrap', roleColor[msg.role]]">{{ msg.content }}</div>
          </div>
          <div v-if="sending" class="text-sm text-gray-500 animate-pulse">Sending...</div>
        </div>
        <div class="border-t border-gray-800 p-3 flex gap-2">
          <input
            v-model="input"
            @keydown.enter="send"
            placeholder="Type a message to start a new session..."
            id="chat-input"
            class="flex-1 rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
          />
          <button @click="send" :disabled="!input.trim() || sending" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500 disabled:opacity-50">Send</button>
        </div>
      </template>

      <!-- Session selected: full chat -->
      <template v-else>
        <!-- Toolbar -->
        <div class="flex items-center justify-between border-b border-gray-800 px-4 py-2">
          <div class="flex items-center gap-2 min-w-0">
            <span class="text-sm font-medium text-gray-200 truncate">{{ selectedSession.title || 'Untitled' }}</span>
            <StatusBadge :status="selectedSession.status" />
            <span class="text-xs text-gray-600 font-mono truncate">{{ truncate(selectedSession.id, 12) }}</span>
          </div>
          <div class="flex items-center gap-2">
            <RouterLink v-if="selectedSession.project_id" :to="`/projects/${selectedSession.project_id}`" class="text-xs text-indigo-400 hover:text-indigo-300">Project</RouterLink>
            <div class="relative">
              <button @click="showActions = !showActions" class="rounded px-2 py-1 text-xs text-gray-400 hover:bg-gray-800 transition-colors">Actions</button>
              <div v-if="showActions" class="absolute right-0 top-8 z-10 rounded-lg border border-gray-700 bg-gray-800 py-1 shadow-lg min-w-[120px]">
                <button v-if="selectedSession.status === 'ACTIVE'" @click="runAction('pause')" class="block w-full px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-700 text-left">Pause</button>
                <button v-if="selectedSession.status === 'PAUSED'" @click="runAction('resume')" class="block w-full px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-700 text-left">Resume</button>
                <button @click="runAction('expire')" class="block w-full px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-700 text-left">Expire</button>
                <button @click="runAction('delete')" class="block w-full px-3 py-1.5 text-xs text-red-400 hover:bg-gray-700 text-left">Delete</button>
              </div>
            </div>
          </div>
        </div>

        <!-- Messages -->
        <div ref="messagesEl" class="flex-1 overflow-y-auto p-4 space-y-3">
          <div v-if="messages.length === 0" class="text-center py-12 text-gray-600 text-sm">No messages yet. Type below to start chatting.</div>
          <div
            v-for="msg in messages"
            :key="msg.id"
            :class="['rounded-lg border border-gray-800 bg-gray-800/50 p-3', msg.role === 'user' ? 'border-indigo-900/30' : '']"
          >
            <div class="flex items-center gap-2 mb-1">
              <span :class="['text-xs font-medium', roleColor[msg.role]]">{{ roleLabel[msg.role] }}</span>
              <span class="text-xs text-gray-600">{{ new Date(msg.created_at).toLocaleTimeString() }}</span>
              <span v-if="msg.metadata?.tool" class="rounded bg-gray-700 px-1.5 py-0.5 text-xs text-gray-400">{{ msg.metadata.tool }}</span>
              <span v-if="msg.metadata?.chain_level" class="rounded bg-indigo-900/50 px-1.5 py-0.5 text-xs text-indigo-300">{{ msg.metadata.chain_level }}</span>
            </div>
            <div :class="['text-sm whitespace-pre-wrap', roleColor[msg.role]]">{{ msg.content }}</div>
            <div v-if="msg.artifacts?.length" class="mt-2 flex gap-2">
              <span v-for="a in msg.artifacts" :key="a" class="rounded bg-gray-700 px-2 py-0.5 text-xs text-gray-400">{{ a }}</span>
            </div>
            <div v-if="msg.metadata?.requires_confirmation && !msg.metadata?.resolved" class="mt-3 flex gap-2">
              <button @click="confirmAction(msg, true)" class="rounded bg-green-700 px-3 py-1.5 text-xs text-white hover:bg-green-600">Approve</button>
              <button @click="confirmAction(msg, false)" class="rounded bg-red-700 px-3 py-1.5 text-xs text-white hover:bg-red-600">Reject</button>
            </div>
            <div v-if="msg.metadata?.resolved" class="mt-1 text-xs text-gray-600">Resolved</div>
          </div>
          <div v-if="sending" class="text-sm text-gray-500 animate-pulse">Sending...</div>
        </div>

        <!-- Input -->
        <div class="border-t border-gray-800 p-3 flex gap-2">
          <input
            v-model="input"
            @keydown.enter="send"
            :placeholder="selectedSession.status === 'PAUSED' ? 'Session is paused...' : 'Type a message...'"
            :disabled="selectedSession.status === 'PAUSED'"
            id="chat-input"
            class="flex-1 rounded-lg border border-gray-700 bg-gray-800 px-4 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none disabled:opacity-50"
          />
          <button
            @click="send"
            :disabled="sending || !input.trim() || selectedSession.status === 'PAUSED'"
            class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500 disabled:opacity-50 transition-colors"
          >Send</button>
        </div>
      </template>
    </div>
  </div>
</template>
