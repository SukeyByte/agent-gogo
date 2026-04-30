<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { Task, TaskAttempt, TaskEvent, ToolCall, Observation, TestResult, ReviewResult } from '../api/types'

const route = useRoute()
const taskId = route.params.id as string

const task = ref<Task | null>(null)
const attempts = ref<TaskAttempt[]>([])
const events = ref<TaskEvent[]>([])
const toolCallsByAttempt = ref<Record<string, ToolCall[]>>({})
const observationsByAttempt = ref<Record<string, Observation[]>>({})
const expandedAttempt = ref<string | null>(null)
const loading = ref(true)
const actionLoading = ref<string | null>(null)

onMounted(async () => {
  const [t, a, e] = await Promise.all([api.getTask(taskId), api.listAttempts(taskId), api.listEvents(taskId)])
  task.value = t
  attempts.value = a || []
  events.value = e || []

  // Load tool calls and observations for each attempt
  const tcMap: Record<string, ToolCall[]> = {}
  const obsMap: Record<string, Observation[]> = {}
  for (const att of a) {
    const [tc, obs] = await Promise.all([api.listToolCalls(att.id), api.listObservations(att.id)])
    tcMap[att.id] = tc
    obsMap[att.id] = obs
  }
  toolCallsByAttempt.value = tcMap
  observationsByAttempt.value = obsMap
  if (a.length) expandedAttempt.value = a[0].id
  loading.value = false
})

async function taskAction(action: string) {
  actionLoading.value = action
  try {
    if (action === 'retry') {
      await api.retryTask(taskId)
      // Refresh task data
      task.value = await api.getTask(taskId)
    } else if (action === 'approve') {
      const att = attempts.value[0]
      await api.sendConfirmation(task.value?.project_id || '', taskId, att?.id || '', '', true, 'Approved via web console')
      task.value = await api.getTask(taskId)
    } else if (action === 'reject') {
      const att = attempts.value[0]
      await api.sendConfirmation(task.value?.project_id || '', taskId, att?.id || '', '', false, 'Rejected via web console')
      task.value = await api.getTask(taskId)
    } else {
      // skip/fix — send as channel message
      await api.sendChatMessage('', `/${action} task ${taskId}`)
    }
  } catch (err: any) {
    alert(`Action failed: ${err.message}`)
  }
  actionLoading.value = null
}

const statusOrder = ['DRAFT', 'READY', 'IN_PROGRESS', 'IMPLEMENTED', 'TESTING', 'REVIEWING', 'DONE']

function statusProgress(currentStatus: string): number {
  const idx = statusOrder.indexOf(currentStatus)
  return idx >= 0 ? idx / (statusOrder.length - 1) * 100 : 0
}
</script>

<template>
  <div v-if="loading" class="text-gray-500">Loading...</div>
  <div v-else-if="task" class="space-y-6">
    <!-- Task Header -->
    <div class="rounded-lg border border-gray-800 bg-gray-900 p-5">
      <div class="flex items-start justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-100">{{ task.title }}</h2>
          <p class="mt-1 text-sm text-gray-400">{{ task.description }}</p>
        </div>
        <StatusBadge :status="task.status" />
      </div>

      <!-- Progress Bar -->
      <div class="mt-4">
        <div class="flex justify-between text-xs text-gray-500 mb-1">
          <span v-for="s in statusOrder.slice(0, -1)" :key="s">{{ s.slice(0, 3) }}</span>
        </div>
        <div class="h-1.5 rounded-full bg-gray-800 overflow-hidden">
          <div class="h-full rounded-full bg-indigo-500 transition-all" :style="{ width: statusProgress(task.status) + '%' }"></div>
        </div>
      </div>

      <!-- Acceptance Criteria -->
      <div v-if="task.acceptance_criteria?.length" class="mt-4">
        <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Acceptance Criteria</h4>
        <ul class="space-y-1">
          <li v-for="c in task.acceptance_criteria" :key="c" class="flex items-center gap-2 text-sm text-gray-300">
            <span class="text-gray-600">☐</span> {{ c }}
          </li>
        </ul>
      </div>

      <!-- Actions -->
      <div class="mt-4 flex flex-wrap gap-2">
        <button @click="taskAction('retry')" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800">Retry</button>
        <button @click="taskAction('skip')" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800">Skip</button>
        <button @click="taskAction('approve')" class="rounded bg-green-800 px-3 py-1.5 text-xs text-white hover:bg-green-700">Approve</button>
        <button @click="taskAction('reject')" class="rounded bg-red-800 px-3 py-1.5 text-xs text-white hover:bg-red-700">Reject</button>
        <button @click="taskAction('fix')" class="rounded border border-yellow-700 px-3 py-1.5 text-xs text-yellow-300 hover:bg-yellow-900/30">Create Fix Task</button>
        <span v-if="actionLoading" class="text-xs text-gray-500 animate-pulse">{{ actionLoading }}...</span>
      </div>
    </div>

    <!-- Attempts -->
    <div>
      <h3 class="mb-3 text-sm font-medium text-gray-300">Attempts ({{ attempts.length }})</h3>
      <div class="space-y-3">
        <div
          v-for="att in attempts"
          :key="att.id"
          class="rounded-lg border border-gray-800 bg-gray-900"
        >
          <button
            @click="expandedAttempt = expandedAttempt === att.id ? null : att.id"
            class="flex w-full items-center justify-between px-4 py-3"
          >
            <div class="flex items-center gap-3">
              <span class="text-sm text-gray-400">Attempt #{{ att.number }}</span>
              <StatusBadge :status="att.status" />
              <span v-if="att.error" class="text-xs text-red-400 truncate max-w-xs">{{ att.error }}</span>
            </div>
            <span class="text-gray-600 text-xs">{{ expandedAttempt === att.id ? '▼' : '▶' }}</span>
          </button>

          <div v-if="expandedAttempt === att.id" class="border-t border-gray-800 p-4 space-y-4">
            <!-- Tool Calls -->
            <div v-if="toolCallsByAttempt[att.id]?.length">
              <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Tool Calls</h4>
              <div class="space-y-2">
                <div v-for="tc in toolCallsByAttempt[att.id]" :key="tc.id" class="rounded border border-gray-800 bg-gray-800/50 p-3">
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-xs font-mono text-indigo-300">{{ tc.name }}</span>
                    <StatusBadge :status="tc.status" />
                  </div>
                  <details class="text-xs">
                    <summary class="cursor-pointer text-gray-400">Input</summary>
                    <pre class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap text-gray-500">{{ tc.input_json }}</pre>
                  </details>
                  <details v-if="tc.output_json" class="text-xs mt-1">
                    <summary class="cursor-pointer text-gray-400">Output</summary>
                    <pre class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap text-gray-500">{{ tc.output_json }}</pre>
                  </details>
                </div>
              </div>
            </div>

            <!-- Observations -->
            <div v-if="observationsByAttempt[att.id]?.length">
              <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Observations</h4>
              <div class="space-y-1">
                <div v-for="obs in observationsByAttempt[att.id]" :key="obs.id" class="flex items-center gap-2 text-xs">
                  <span class="text-gray-600 font-mono">{{ obs.type }}</span>
                  <span class="text-gray-400">{{ obs.summary }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Events Log -->
    <div>
      <h3 class="mb-3 text-sm font-medium text-gray-300">Events ({{ events.length }})</h3>
      <div class="rounded-lg border border-gray-800 bg-gray-900 divide-y divide-gray-800">
        <div v-for="ev in events" :key="ev.id" class="flex items-center gap-3 px-4 py-2">
          <span class="text-xs text-gray-600 font-mono w-36">{{ new Date(ev.created_at).toLocaleTimeString() }}</span>
          <span class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-gray-400">{{ ev.type }}</span>
          <span v-if="ev.from_state && ev.to_state" class="text-xs text-gray-500">{{ ev.from_state }} → {{ ev.to_state }}</span>
          <span class="text-xs text-gray-400 flex-1 truncate">{{ ev.message }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
