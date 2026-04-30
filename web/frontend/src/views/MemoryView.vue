<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api'
import type { MemoryItem } from '../api/types'

const memories = ref<MemoryItem[]>([])
const filtered = ref<MemoryItem[]>([])
const searchQuery = ref('')
const scopeFilter = ref<string>('all')
const loading = ref(true)
const showAddForm = ref(false)
const newMemory = ref({ scope: 'working', type: 'fuzzy', summary: '', body: '', tags: '' })

const scopes = ['all', 'working', 'project', 'long_term']
const scopeLabel: Record<string, string> = { working: 'Working', project: 'Project', long_term: 'Long-term' }
const scopeColor: Record<string, string> = { working: 'bg-blue-900/50 text-blue-300', project: 'bg-purple-900/50 text-purple-300', long_term: 'bg-green-900/50 text-green-300' }

onMounted(async () => {
  memories.value = await api.listMemories()
  applyFilters()
  loading.value = false
})

function applyFilters() {
  let result = memories.value
  if (scopeFilter.value !== 'all') {
    result = result.filter(m => m.scope === scopeFilter.value)
  }
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    result = result.filter(m => m.summary.toLowerCase().includes(q) || m.body.toLowerCase().includes(q) || m.tags.some(t => t.toLowerCase().includes(q)))
  }
  filtered.value = result
}

function setScope(scope: string) {
  scopeFilter.value = scope
  applyFilters()
}

async function search() {
  if (!searchQuery.value) {
    applyFilters()
  } else {
    filtered.value = await api.searchMemories(searchQuery.value)
  }
}

async function addMemory() {
  const item = await api.addMemory({
    scope: newMemory.value.scope as MemoryItem['scope'],
    type: newMemory.value.type as MemoryItem['type'],
    summary: newMemory.value.summary,
    body: newMemory.value.body,
    tags: newMemory.value.tags.split(',').map(t => t.trim()).filter(Boolean),
  })
  memories.value.unshift(item)
  applyFilters()
  showAddForm.value = false
  newMemory.value = { scope: 'working', type: 'fuzzy', summary: '', body: '', tags: '' }
}

async function deleteMemory(id: string) {
  await api.deleteMemory(id)
  memories.value = memories.value.filter(m => m.id !== id)
  applyFilters()
}
</script>

<template>
  <div class="space-y-4">
    <!-- Filters -->
    <div class="flex items-center gap-3">
      <input
        v-model="searchQuery"
        @keydown.enter="search"
        placeholder="Search memories..."
        class="flex-1 rounded-lg border border-gray-700 bg-gray-900 px-4 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
      />
      <button @click="search" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500">Search</button>
      <button @click="showAddForm = !showAddForm" class="rounded-lg border border-gray-700 px-4 py-2 text-sm text-gray-300 hover:bg-gray-800">+ Add</button>
    </div>

    <!-- Scope Filter -->
    <div class="flex gap-2">
      <button
        v-for="scope in scopes"
        :key="scope"
        @click="setScope(scope)"
        :class="[
          'rounded px-3 py-1.5 text-xs font-medium transition-colors capitalize',
          scopeFilter === scope ? 'bg-indigo-600 text-white' : 'border border-gray-700 text-gray-400 hover:bg-gray-800'
        ]"
      >
        {{ scope === 'all' ? 'All' : scopeLabel[scope] || scope }}
      </button>
    </div>

    <!-- Add Form -->
    <div v-if="showAddForm" class="rounded-lg border border-gray-800 bg-gray-900 p-4 space-y-3">
      <div class="flex gap-3">
        <select v-model="newMemory.scope" class="rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100">
          <option value="working">Working</option>
          <option value="project">Project</option>
          <option value="long_term">Long-term</option>
        </select>
        <select v-model="newMemory.type" class="rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100">
          <option value="exact">Exact</option>
          <option value="fuzzy">Fuzzy</option>
        </select>
      </div>
      <input v-model="newMemory.summary" placeholder="Summary" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none" />
      <textarea v-model="newMemory.body" placeholder="Body content..." rows="3" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none resize-none" />
      <input v-model="newMemory.tags" placeholder="Tags (comma separated)" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none" />
      <div class="flex gap-2 justify-end">
        <button @click="showAddForm = false" class="rounded-lg border border-gray-700 px-4 py-2 text-sm text-gray-400">Cancel</button>
        <button @click="addMemory" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500">Save</button>
      </div>
    </div>

    <!-- Memory List -->
    <div v-if="loading" class="text-gray-500">Loading...</div>
    <div v-else class="space-y-2">
      <div v-if="filtered.length === 0" class="text-center py-8 text-gray-600">No memories found</div>
      <div
        v-for="mem in filtered"
        :key="mem.id"
        class="rounded-lg border border-gray-800 bg-gray-900 p-4"
      >
        <div class="flex items-start justify-between">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 mb-1">
              <span :class="['rounded px-1.5 py-0.5 text-xs', scopeColor[mem.scope]]">{{ scopeLabel[mem.scope] }}</span>
              <span class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-gray-500">{{ mem.type }}</span>
              <span class="text-xs text-gray-600">confidence: {{ mem.confidence.toFixed(2) }}</span>
            </div>
            <p class="text-sm text-gray-200">{{ mem.summary }}</p>
            <p class="mt-1 text-xs text-gray-500">{{ mem.body }}</p>
            <div class="mt-2 flex flex-wrap gap-1">
              <span v-for="tag in mem.tags" :key="tag" class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-gray-500">#{{ tag }}</span>
            </div>
          </div>
          <button @click="deleteMemory(mem.id)" class="ml-2 text-gray-600 hover:text-red-400 text-xs">✕</button>
        </div>
      </div>
    </div>
  </div>
</template>
