<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { Project } from '../api/types'

const projects = ref<Project[]>([])
const loading = ref(true)
const showCreate = ref(false)
const newName = ref('')
const newGoal = ref('')
const creating = ref(false)

onMounted(async () => {
  projects.value = await api.listProjects()
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
    // Refresh list after short delay to pick up newly created project
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
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <p class="text-sm text-gray-400">{{ projects.length }} projects</p>
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

    <div v-if="loading" class="text-gray-500">Loading...</div>
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
        <div class="flex items-center gap-3 ml-4">
          <StatusBadge :status="p.status" />
          <span class="text-xs text-gray-600">{{ timeAgo(p.updated_at) }}</span>
        </div>
      </RouterLink>
    </div>
  </div>
</template>
