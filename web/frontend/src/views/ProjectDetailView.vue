<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { Project, Task, Artifact } from '../api/types'

const route = useRoute()
const projectId = route.params.id as string

const project = ref<Project | null>(null)
const tasks = ref<Task[]>([])
const artifacts = ref<Artifact[]>([])
const loading = ref(true)
const actionLoading = ref<string | null>(null)

onMounted(async () => {
  const [p, t, a] = await Promise.all([api.getProject(projectId), api.listTasks(projectId), api.listArtifacts(projectId)])
  project.value = p
  tasks.value = t || []
  artifacts.value = a || []
  loading.value = false
})

async function runAction(action: string) {
  actionLoading.value = action
  await new Promise(r => setTimeout(r, 1000))
  actionLoading.value = null
}

const taskTypeIcon: Record<string, string> = {
  code: '</>', browser: '◎', doc: '▤', test: '✓', review: '◉', writing: '✎', plan: '◇',
}
</script>

<template>
  <div v-if="loading" class="text-gray-500">Loading...</div>
  <div v-else-if="project" class="space-y-6">
    <!-- Project Header -->
    <div class="rounded-lg border border-gray-800 bg-gray-900 p-5">
      <div class="flex items-start justify-between">
        <div>
          <h2 class="text-lg font-semibold text-gray-100">{{ project.name }}</h2>
          <p class="mt-1 text-sm text-gray-400">{{ project.goal }}</p>
        </div>
        <StatusBadge :status="project.status" />
      </div>

      <!-- Actions -->
      <div class="mt-4 flex flex-wrap gap-2">
        <button @click="runAction('plan')" :disabled="!!actionLoading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">Replan</button>
        <button @click="runAction('run')" :disabled="!!actionLoading" class="rounded bg-blue-700 px-3 py-1.5 text-xs text-white hover:bg-blue-600 disabled:opacity-50">Run Next Task</button>
        <button @click="runAction('resume')" :disabled="!!actionLoading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">Resume All</button>
        <button @click="runAction('pause')" :disabled="!!actionLoading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">Pause</button>
        <button @click="runAction('stop')" :disabled="!!actionLoading" class="rounded border border-red-800 px-3 py-1.5 text-xs text-red-400 hover:bg-red-900/30 disabled:opacity-50">Stop</button>
        <span v-if="actionLoading" class="flex items-center text-xs text-gray-500 animate-pulse">{{ actionLoading }}...</span>
      </div>
    </div>

    <!-- Task List -->
    <div>
      <h3 class="mb-3 text-sm font-medium text-gray-300">Tasks ({{ tasks.length }})</h3>
      <div class="space-y-2">
        <RouterLink
          v-for="task in tasks"
          :key="task.id"
          :to="`/tasks/${task.id}`"
          class="flex items-center gap-4 rounded-lg border border-gray-800 bg-gray-900 px-4 py-3 hover:border-gray-700 transition-colors"
        >
          <span class="text-xs text-gray-600 font-mono w-12 text-center">{{ taskTypeIcon[task.description.includes('code') ? 'code' : task.description.includes('故事') ? 'writing' : task.description.includes('测试') ? 'test' : task.description.includes('验收') ? 'review' : 'plan'] }}</span>
          <div class="min-w-0 flex-1">
            <div class="text-sm text-gray-200">{{ task.title }}</div>
            <div class="text-xs text-gray-500 truncate">{{ task.description }}</div>
          </div>
          <div class="flex items-center gap-2">
            <span v-if="task.depends_on?.length" class="text-xs text-gray-600">depends: {{ task.depends_on.length }}</span>
            <StatusBadge :status="task.status" />
          </div>
        </RouterLink>
      </div>
    </div>

    <!-- Artifacts -->
    <div v-if="artifacts.length">
      <h3 class="mb-3 text-sm font-medium text-gray-300">Artifacts ({{ artifacts.length }})</h3>
      <div class="space-y-1">
        <div v-for="a in artifacts" :key="a.id" class="flex items-center justify-between rounded-lg border border-gray-800 bg-gray-900 px-4 py-2">
          <div>
            <span class="text-sm text-gray-200">{{ a.path }}</span>
            <span class="ml-2 text-xs text-gray-600">{{ a.type }}</span>
          </div>
          <span class="text-xs text-gray-500">{{ a.description }}</span>
        </div>
      </div>
    </div>
  </div>
</template>
