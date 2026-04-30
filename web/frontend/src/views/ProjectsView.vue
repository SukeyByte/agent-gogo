<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { Project, Task } from '../api/types'

const projects = ref<Project[]>([])
const taskCounts = ref<Record<string, { total: number; done: number; running: number; failed: number }>>({})
const loading = ref(true)
const showCreate = ref(false)
const newName = ref('')
const newGoal = ref('')
const creating = ref(false)
const viewMode = ref<'board' | 'list'>('board')

const activeProjects = computed(() => projects.value.filter(p => p.status === 'ACTIVE'))
const completedProjects = computed(() => projects.value.filter(p => p.status === 'COMPLETED'))
const archivedProjects = computed(() => projects.value.filter(p => p.status === 'ARCHIVED'))

onMounted(async () => {
  projects.value = await api.listProjects()
  for (const p of projects.value) {
    const tasks = await api.listTasks(p.id)
    taskCounts.value[p.id] = {
      total: tasks.length,
      done: tasks.filter(t => t.status === 'DONE').length,
      running: tasks.filter(t => ['IN_PROGRESS', 'TESTING', 'REVIEWING', 'IMPLEMENTED'].includes(t.status)).length,
      failed: tasks.filter(t => ['FAILED', 'REVIEW_FAILED', 'BLOCKED'].includes(t.status)).length,
    }
  }
  loading.value = false
})

async function createProject() {
  if (!newName.value || !newGoal.value) return
  creating.value = true
  try {
    await api.createProject(newName.value, newGoal.value)
    newName.value = ''
    newGoal.value = ''
    showCreate.value = false
    setTimeout(async () => {
      projects.value = await api.listProjects()
    }, 500)
  } catch (err: any) {
    alert('Failed to create project: ' + err.message)
  }
  creating.value = false
}

function timeAgo(dateStr: string): string {
  const h = Math.floor((Date.now() - new Date(dateStr).getTime()) / 3600000)
  if (h < 1) return 'just now'
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

function progressPercent(id: string): number {
  const c = taskCounts.value[id]
  if (!c || c.total === 0) return 0
  return Math.round((c.done / c.total) * 100)
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <button
          @click="viewMode = 'board'"
          :class="[viewMode === 'board' ? 'bg-gray-700 text-gray-100' : 'text-gray-500 hover:text-gray-300', 'rounded px-3 py-1.5 text-xs transition-colors']"
        >Board</button>
        <button
          @click="viewMode = 'list'"
          :class="[viewMode === 'list' ? 'bg-gray-700 text-gray-100' : 'text-gray-500 hover:text-gray-300', 'rounded px-3 py-1.5 text-xs transition-colors']"
        >List</button>
        <span class="text-xs text-gray-600">{{ projects.length }} projects</span>
      </div>
      <button @click="showCreate = !showCreate" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 transition-colors">
        + New Project
      </button>
    </div>

    <!-- Create Form -->
    <div v-if="showCreate" class="rounded-lg border border-gray-800 bg-gray-900 p-4 space-y-3">
      <input v-model="newName" placeholder="Project name" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none" />
      <textarea v-model="newGoal" placeholder="Goal description..." rows="3" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none resize-none" />
      <div class="flex gap-2 justify-end">
        <button @click="showCreate = false" class="rounded-lg border border-gray-700 px-4 py-2 text-sm text-gray-400 hover:bg-gray-800">Cancel</button>
        <button @click="createProject" :disabled="creating" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500 disabled:opacity-50">Create</button>
      </div>
    </div>

    <div v-if="loading" class="text-gray-500 text-sm">Loading...</div>

    <!-- Board View (Jira-style columns) -->
    <div v-else-if="viewMode === 'board'" class="grid gap-4 lg:grid-cols-3">
      <!-- Active Column -->
      <div>
        <div class="flex items-center gap-2 mb-3">
          <span class="h-2 w-2 rounded-full bg-blue-500"></span>
          <h3 class="text-sm font-medium text-gray-300">Active</h3>
          <span class="text-xs text-gray-600">{{ activeProjects.length }}</span>
        </div>
        <div class="space-y-2">
          <RouterLink
            v-for="p in activeProjects"
            :key="p.id"
            :to="`/projects/${p.id}`"
            class="block rounded-lg border border-gray-800 bg-gray-900 p-4 hover:border-gray-700 transition-colors"
          >
            <div class="flex items-start justify-between mb-2">
              <div class="text-sm font-medium text-gray-100">{{ p.name }}</div>
              <StatusBadge :status="p.status" />
            </div>
            <p class="text-xs text-gray-500 mb-3 line-clamp-2">{{ p.goal }}</p>
            <!-- Progress bar -->
            <div v-if="taskCounts[p.id]" class="mb-2">
              <div class="flex items-center justify-between text-xs text-gray-600 mb-1">
                <span>{{ taskCounts[p.id].done }}/{{ taskCounts[p.id].total }} done</span>
                <span>{{ progressPercent(p.id) }}%</span>
              </div>
              <div class="h-1.5 rounded-full bg-gray-800 overflow-hidden">
                <div class="h-full rounded-full bg-indigo-600 transition-all" :style="{ width: progressPercent(p.id) + '%' }"></div>
              </div>
            </div>
            <!-- Task counts -->
            <div v-if="taskCounts[p.id]" class="flex items-center gap-3 text-xs">
              <span v-if="taskCounts[p.id].running" class="text-blue-400">{{ taskCounts[p.id].running }} running</span>
              <span v-if="taskCounts[p.id].failed" class="text-red-400">{{ taskCounts[p.id].failed }} failed</span>
            </div>
            <div class="text-xs text-gray-600 mt-2">{{ timeAgo(p.updated_at) }}</div>
          </RouterLink>
          <div v-if="activeProjects.length === 0" class="text-xs text-gray-600 text-center py-4">No active projects</div>
        </div>
      </div>

      <!-- Completed Column -->
      <div>
        <div class="flex items-center gap-2 mb-3">
          <span class="h-2 w-2 rounded-full bg-green-500"></span>
          <h3 class="text-sm font-medium text-gray-300">Completed</h3>
          <span class="text-xs text-gray-600">{{ completedProjects.length }}</span>
        </div>
        <div class="space-y-2">
          <RouterLink
            v-for="p in completedProjects"
            :key="p.id"
            :to="`/projects/${p.id}`"
            class="block rounded-lg border border-gray-800 bg-gray-900 p-4 hover:border-gray-700 transition-colors"
          >
            <div class="flex items-start justify-between mb-2">
              <div class="text-sm font-medium text-gray-100">{{ p.name }}</div>
              <StatusBadge :status="p.status" />
            </div>
            <p class="text-xs text-gray-500 line-clamp-2">{{ p.goal }}</p>
            <div v-if="taskCounts[p.id]" class="text-xs text-green-400 mt-2">{{ taskCounts[p.id].done }}/{{ taskCounts[p.id].total }} done</div>
            <div class="text-xs text-gray-600 mt-1">{{ timeAgo(p.updated_at) }}</div>
          </RouterLink>
          <div v-if="completedProjects.length === 0" class="text-xs text-gray-600 text-center py-4">No completed projects</div>
        </div>
      </div>

      <!-- Archived Column -->
      <div>
        <div class="flex items-center gap-2 mb-3">
          <span class="h-2 w-2 rounded-full bg-gray-600"></span>
          <h3 class="text-sm font-medium text-gray-300">Archived</h3>
          <span class="text-xs text-gray-600">{{ archivedProjects.length }}</span>
        </div>
        <div class="space-y-2">
          <RouterLink
            v-for="p in archivedProjects"
            :key="p.id"
            :to="`/projects/${p.id}`"
            class="block rounded-lg border border-gray-800 bg-gray-900 p-4 hover:border-gray-700 transition-colors opacity-60 hover:opacity-100 transition-opacity"
          >
            <div class="flex items-start justify-between mb-2">
              <div class="text-sm font-medium text-gray-100">{{ p.name }}</div>
              <StatusBadge :status="p.status" />
            </div>
            <p class="text-xs text-gray-500 line-clamp-2">{{ p.goal }}</p>
            <div class="text-xs text-gray-600 mt-2">{{ timeAgo(p.updated_at) }}</div>
          </RouterLink>
          <div v-if="archivedProjects.length === 0" class="text-xs text-gray-600 text-center py-4">No archived projects</div>
        </div>
      </div>
    </div>

    <!-- List View -->
    <div v-else class="space-y-2">
      <RouterLink
        v-for="p in projects"
        :key="p.id"
        :to="`/projects/${p.id}`"
        class="flex items-center justify-between rounded-lg border border-gray-800 bg-gray-900 px-5 py-4 hover:border-gray-700 transition-colors"
      >
        <div class="min-w-0 flex-1">
          <div class="text-sm font-medium text-gray-100">{{ p.name }}</div>
          <div class="text-xs text-gray-500 truncate">{{ p.goal }}</div>
        </div>
        <div class="flex items-center gap-4 ml-4">
          <div v-if="taskCounts[p.id]" class="text-xs text-gray-500">
            {{ taskCounts[p.id].done }}/{{ taskCounts[p.id].total }}
          </div>
          <StatusBadge :status="p.status" />
          <span class="text-xs text-gray-600">{{ timeAgo(p.updated_at) }}</span>
        </div>
      </RouterLink>
    </div>
  </div>
</template>
