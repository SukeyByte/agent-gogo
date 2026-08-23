<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { Session, SessionContext } from '../api/types'

const sessions = ref<Session[]>([])
const selected = ref<Session | null>(null)
const context = ref<SessionContext | null>(null)
const loading = ref(true)
const acting = ref(false)
const filter = ref<string>('')

const filtered = computed(() => {
  if (!filter.value) return sessions.value
  return sessions.value.filter(s => s.status === filter.value)
})

const counts = computed(() => {
  const out: Record<string, number> = { '': sessions.value.length }
  for (const session of sessions.value) out[session.status] = (out[session.status] || 0) + 1
  return out
})

onMounted(refresh)

async function refresh() {
  loading.value = true
  sessions.value = await api.listSessions()
  if (!selected.value && sessions.value.length > 0) await selectSession(sessions.value[0])
  loading.value = false
}

async function selectSession(session: Session) {
  selected.value = session
  context.value = null
  try {
    context.value = await api.getSessionContext(session.id)
  } catch {
    context.value = null
  }
}

async function run(action: 'pause' | 'resume' | 'expire' | 'delete') {
  if (!selected.value || acting.value) return
  acting.value = true
  try {
    if (action === 'pause') selected.value = await api.pauseSession(selected.value.id)
    if (action === 'resume') selected.value = await api.resumeSession(selected.value.id)
    if (action === 'expire') selected.value = await api.expireSession(selected.value.id)
    if (action === 'delete') {
      await api.deleteSession(selected.value.id)
      selected.value = null
      context.value = null
    }
    sessions.value = await api.listSessions()
  } finally {
    acting.value = false
  }
}

function timeAgo(dateStr: string): string {
  if (!dateStr) return ''
  const minutes = Math.floor((Date.now() - new Date(dateStr).getTime()) / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

function short(text: string, limit = 160): string {
  if (!text) return ''
  return text.length > limit ? `${text.slice(0, limit)}...` : text
}
</script>

<template>
  <div class="grid gap-4 lg:grid-cols-[360px_1fr]">
    <section class="space-y-3">
      <div class="flex flex-wrap gap-2">
        <button
          v-for="item in [
            { value: '', label: 'All' },
            { value: 'ACTIVE', label: 'Active' },
            { value: 'PAUSED', label: 'Paused' },
            { value: 'COMPLETED', label: 'Completed' },
            { value: 'EXPIRED', label: 'Expired' },
          ]"
          :key="item.value"
          @click="filter = item.value"
          :class="[filter === item.value ? 'bg-gray-700 text-gray-100' : 'text-gray-500 hover:text-gray-300', 'rounded px-3 py-1.5 text-xs transition-colors']"
        >
          {{ item.label }} {{ counts[item.value] || 0 }}
        </button>
      </div>

      <div v-if="loading" class="text-sm text-gray-500">Loading...</div>
      <div v-else class="divide-y divide-gray-800 rounded-lg border border-gray-800 bg-gray-900">
        <button
          v-for="session in filtered"
          :key="session.id"
          @click="selectSession(session)"
          :class="[selected?.id === session.id ? 'bg-indigo-900/20' : 'hover:bg-gray-800/50', 'block w-full px-4 py-3 text-left transition-colors']"
        >
          <div class="mb-1 flex items-center justify-between gap-2">
            <span class="truncate text-sm font-medium text-gray-200">{{ session.title || session.id }}</span>
            <StatusBadge :status="session.status" />
          </div>
          <div class="flex items-center justify-between text-xs text-gray-600">
            <span>{{ session.channel_type || 'web' }} · {{ session.channel_id || 'local' }}</span>
            <span>{{ timeAgo(session.last_active_at) }}</span>
          </div>
        </button>
        <div v-if="filtered.length === 0" class="px-4 py-8 text-center text-sm text-gray-600">No sessions</div>
      </div>
    </section>

    <section class="min-w-0 rounded-lg border border-gray-800 bg-gray-900">
      <div v-if="selected" class="space-y-5 p-5">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <h2 class="truncate text-base font-semibold text-gray-100">{{ selected.title || selected.id }}</h2>
            <p class="mt-1 text-xs text-gray-500">{{ selected.id }}</p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button @click="run('pause')" :disabled="acting || selected.status !== 'ACTIVE'" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 disabled:opacity-40">Pause</button>
            <button @click="run('resume')" :disabled="acting || selected.status === 'ACTIVE'" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 disabled:opacity-40">Resume</button>
            <button @click="run('expire')" :disabled="acting" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 disabled:opacity-40">Expire</button>
            <button @click="run('delete')" :disabled="acting" class="rounded border border-red-900/60 px-3 py-1.5 text-xs text-red-300 disabled:opacity-40">Delete</button>
          </div>
        </div>

        <div class="grid gap-3 md:grid-cols-3">
          <div class="rounded border border-gray-800 bg-gray-950 p-3">
            <div class="text-xs text-gray-600">Status</div>
            <div class="mt-1"><StatusBadge :status="selected.status" /></div>
          </div>
          <div class="rounded border border-gray-800 bg-gray-950 p-3">
            <div class="text-xs text-gray-600">Last Active</div>
            <div class="mt-1 text-sm text-gray-200">{{ timeAgo(selected.last_active_at) }}</div>
          </div>
          <RouterLink
            v-if="selected.project_id"
            :to="`/projects/${selected.project_id}`"
            class="rounded border border-gray-800 bg-gray-950 p-3 transition-colors hover:border-gray-700"
          >
            <div class="text-xs text-gray-600">Project</div>
            <div class="mt-1 truncate text-sm text-indigo-300">{{ selected.project_id }}</div>
          </RouterLink>
          <div v-else class="rounded border border-gray-800 bg-gray-950 p-3">
            <div class="text-xs text-gray-600">Project</div>
            <div class="mt-1 text-sm text-gray-500">Unbound</div>
          </div>
        </div>

        <div v-if="context" class="space-y-3">
          <div>
            <div class="mb-1 text-xs font-medium text-gray-500">Chain Decision</div>
            <pre class="max-h-40 overflow-auto rounded bg-gray-950 p-3 text-xs text-gray-400">{{ short(context.chain_decision, 2000) }}</pre>
          </div>
          <div>
            <div class="mb-1 text-xs font-medium text-gray-500">Runtime Context</div>
            <pre class="max-h-96 overflow-auto rounded bg-gray-950 p-3 text-xs text-gray-400">{{ short(context.context_text, 6000) }}</pre>
          </div>
        </div>
        <div v-else class="rounded border border-gray-800 bg-gray-950 p-5 text-sm text-gray-500">No saved runtime context</div>
      </div>
      <div v-else class="p-12 text-center text-sm text-gray-600">Select a session</div>
    </section>
  </div>
</template>
