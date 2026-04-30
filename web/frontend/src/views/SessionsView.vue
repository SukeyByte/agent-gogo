<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { Session, SessionContext } from '../api/types'

const sessions = ref<Session[]>([])
const loading = ref(true)
const selectedSession = ref<Session | null>(null)
const sessionContext = ref<SessionContext | null>(null)
const contextLoading = ref(false)
const statusFilter = ref<string>('')

const filteredSessions = computed(() => {
  if (!statusFilter.value) return sessions.value
  return sessions.value.filter(s => s.status === statusFilter.value)
})

const statusCounts = computed(() => {
  const counts: Record<string, number> = {}
  for (const s of sessions.value) {
    counts[s.status] = (counts[s.status] || 0) + 1
  }
  return counts
})

onMounted(async () => {
  sessions.value = await api.listSessions()
  loading.value = false
})

async function selectSession(session: Session) {
  selectedSession.value = session
  sessionContext.value = null
  contextLoading.value = true
  sessionContext.value = await api.getSessionContext(session.id)
  contextLoading.value = false
}

function closeDetail() {
  selectedSession.value = null
  sessionContext.value = null
}

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
  switch (type) {
    case 'cli': return '▸'
    case 'web': return '◎'
    case 'telegram': return '✈'
    case 'whatsapp': return '⬡'
    default: return '○'
  }
}

function truncate(s: string, len: number): string {
  if (!s) return ''
  return s.length > len ? s.slice(0, len) + '...' : s
}
</script>

<template>
  <div class="space-y-4">
    <!-- Filter Bar -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <button
          v-for="s in ['', 'ACTIVE', 'PAUSED', 'COMPLETED', 'EXPIRED']"
          :key="s"
          @click="statusFilter = s"
          :class="[
            statusFilter === s ? 'bg-gray-700 text-gray-100' : 'text-gray-500 hover:text-gray-300 hover:bg-gray-800',
            'rounded px-3 py-1.5 text-xs transition-colors'
          ]"
        >
          {{ s || 'All' }}
          <span v-if="s && statusCounts[s]" class="ml-1 text-gray-500">{{ statusCounts[s] }}</span>
        </button>
      </div>
      <p class="text-xs text-gray-600">{{ filteredSessions.length }} sessions</p>
    </div>

    <div v-if="loading" class="text-gray-500 text-sm">Loading...</div>
    <div v-else-if="filteredSessions.length === 0" class="text-gray-600 text-sm">No sessions found.</div>

    <!-- Session List -->
    <div v-else class="space-y-2">
      <div
        v-for="s in filteredSessions"
        :key="s.id"
        @click="selectSession(s)"
        :class="[
          selectedSession?.id === s.id ? 'border-indigo-600 bg-gray-800/80' : 'border-gray-800 bg-gray-900 hover:border-gray-700',
          'flex items-center gap-4 rounded-lg border px-4 py-3 cursor-pointer transition-colors'
        ]"
      >
        <span class="text-base text-gray-500 w-6 text-center">{{ channelIcon(s.channel_type) }}</span>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="text-sm text-gray-200 font-medium">{{ s.title || 'Untitled session' }}</span>
            <span class="text-xs text-gray-600 font-mono">{{ s.id.slice(0, 8) }}</span>
          </div>
          <div class="flex items-center gap-3 mt-0.5">
            <span class="text-xs text-gray-500">{{ s.channel_type }}/{{ s.channel_id }}</span>
            <span v-if="s.user_id" class="text-xs text-gray-600">user: {{ s.user_id }}</span>
            <span v-if="s.project_id" class="text-xs text-gray-600">project: {{ truncate(s.project_id, 8) }}</span>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-xs text-gray-600">{{ timeAgo(s.last_active_at) }}</span>
          <StatusBadge :status="s.status" />
        </div>
      </div>
    </div>

    <!-- Session Detail Panel -->
    <div v-if="selectedSession" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
      <div class="flex items-start justify-between">
        <div>
          <h3 class="text-base font-semibold text-gray-100">{{ selectedSession.title || 'Untitled session' }}</h3>
          <p class="text-xs text-gray-500 font-mono mt-1">{{ selectedSession.id }}</p>
        </div>
        <button @click="closeDetail" class="text-gray-500 hover:text-gray-300 text-sm">✕</button>
      </div>

      <!-- Session Info Grid -->
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div class="rounded border border-gray-800 bg-gray-950 p-3">
          <div class="text-xs text-gray-500">Status</div>
          <div class="mt-1"><StatusBadge :status="selectedSession.status" /></div>
        </div>
        <div class="rounded border border-gray-800 bg-gray-950 p-3">
          <div class="text-xs text-gray-500">Channel</div>
          <div class="mt-1 text-sm text-gray-200">{{ selectedSession.channel_type }} / {{ selectedSession.channel_id }}</div>
        </div>
        <div class="rounded border border-gray-800 bg-gray-950 p-3">
          <div class="text-xs text-gray-500">User</div>
          <div class="mt-1 text-sm text-gray-200">{{ selectedSession.user_id || '—' }}</div>
        </div>
        <div class="rounded border border-gray-800 bg-gray-950 p-3">
          <div class="text-xs text-gray-500">Last Active</div>
          <div class="mt-1 text-sm text-gray-200">{{ timeAgo(selectedSession.last_active_at) }}</div>
        </div>
      </div>

      <!-- Project Link -->
      <div v-if="selectedSession.project_id" class="flex items-center gap-2">
        <span class="text-xs text-gray-500">Bound Project:</span>
        <RouterLink :to="`/projects/${selectedSession.project_id}`" class="text-xs text-indigo-400 hover:text-indigo-300 font-mono">
          {{ selectedSession.project_id }}
        </RouterLink>
      </div>

      <!-- Runtime Context -->
      <div>
        <h4 class="text-sm font-medium text-gray-300 mb-2">Runtime Context</h4>
        <div v-if="contextLoading" class="text-xs text-gray-500">Loading context...</div>
        <div v-else-if="!sessionContext" class="text-xs text-gray-600">No saved runtime context.</div>
        <div v-else class="space-y-2">
          <div v-if="sessionContext.chain_decision && sessionContext.chain_decision !== '{}'" class="rounded border border-gray-800 bg-gray-950 p-3">
            <div class="text-xs text-gray-500 mb-1">Chain Decision</div>
            <pre class="text-xs text-gray-300 whitespace-pre-wrap break-all">{{ truncate(sessionContext.chain_decision, 200) }}</pre>
          </div>
          <div v-if="sessionContext.intent_profile && sessionContext.intent_profile !== '{}'" class="rounded border border-gray-800 bg-gray-950 p-3">
            <div class="text-xs text-gray-500 mb-1">Intent Profile</div>
            <pre class="text-xs text-gray-300 whitespace-pre-wrap break-all">{{ truncate(sessionContext.intent_profile, 200) }}</pre>
          </div>
          <div v-if="sessionContext.memory_snapshot && sessionContext.memory_snapshot !== '[]'" class="rounded border border-gray-800 bg-gray-950 p-3">
            <div class="text-xs text-gray-500 mb-1">Memory Snapshot</div>
            <pre class="text-xs text-gray-300 whitespace-pre-wrap break-all">{{ truncate(sessionContext.memory_snapshot, 200) }}</pre>
          </div>
          <div class="text-xs text-gray-600">Context updated: {{ timeAgo(sessionContext.updated_at) }}</div>
        </div>
      </div>
    </div>
  </div>
</template>
