<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api'
import type { PersonaCard } from '../api/types'

const personas = ref<PersonaCard[]>([])
const selectedPersona = ref<PersonaCard | null>(null)
const filterType = ref<string>('all')
const loading = ref(true)

// CRUD state
const showCreate = ref(false)
const editingPersona = ref<string | null>(null)
const saving = ref(false)
const form = ref({ name: '', type: 'role', description: '', instructions: '' })

const personaTypes = ['all', 'main', 'channel', 'project', 'role']
const createTypes = ['main', 'channel', 'project', 'role']

const filteredPersonas = ref<PersonaCard[]>([])

function filterPersonas() {
  if (filterType.value === 'all') {
    filteredPersonas.value = personas.value
  } else {
    filteredPersonas.value = personas.value.filter(p => p.type === filterType.value)
  }
}

onMounted(() => {
  api.listPersonas().then(p => {
    personas.value = p
    filterPersonas()
    loading.value = false
  })
})

function setFilter(type: string) {
  filterType.value = type
  filterPersonas()
}

function selectPersona(persona: PersonaCard) {
  if (editingPersona.value) return
  selectedPersona.value = selectedPersona.value?.id === persona.id ? null : persona
}

async function refreshPersonas() {
  personas.value = await api.listPersonas()
  filterPersonas()
}

function startCreate() {
  showCreate.value = true
  editingPersona.value = null
  selectedPersona.value = null
  form.value = { name: '', type: 'role', description: '', instructions: '' }
}

function startEdit() {
  if (!selectedPersona.value) return
  showCreate.value = false
  editingPersona.value = selectedPersona.value.id
  form.value = {
    name: selectedPersona.value.name,
    type: selectedPersona.value.type,
    description: selectedPersona.value.description,
    instructions: selectedPersona.value.instructions,
  }
}

function cancelForm() {
  showCreate.value = false
  editingPersona.value = null
}

async function savePersona() {
  saving.value = true
  try {
    if (editingPersona.value) {
      const updated = await api.updatePersona(editingPersona.value, {
        name: form.value.name,
        type: form.value.type,
        description: form.value.description,
        instructions: form.value.instructions,
      })
      selectedPersona.value = updated
    } else {
      await api.createPersona({
        name: form.value.name,
        type: form.value.type,
        description: form.value.description,
        instructions: form.value.instructions,
      })
    }
    await refreshPersonas()
    showCreate.value = false
    editingPersona.value = null
  } catch (e: any) {
    alert('Failed to save persona: ' + e.message)
  } finally {
    saving.value = false
  }
}

async function deletePersona(id: string) {
  try {
    await api.deletePersona(id)
    if (selectedPersona.value?.id === id) selectedPersona.value = null
    await refreshPersonas()
  } catch (e: any) {
    alert('Failed to delete persona: ' + e.message)
  }
}

const typeColor: Record<string, string> = {
  main: 'bg-indigo-900/50 text-indigo-300',
  channel: 'bg-cyan-900/50 text-cyan-300',
  project: 'bg-purple-900/50 text-purple-300',
  role: 'bg-amber-900/50 text-amber-300',
  ephemeral: 'bg-gray-800 text-gray-400',
}
</script>

<template>
  <div class="space-y-4">
    <!-- Type Filter + Create -->
    <div class="flex items-center gap-3">
      <div class="flex gap-2">
        <button
          v-for="type in personaTypes"
          :key="type"
          @click="setFilter(type)"
          :class="[
            'rounded px-3 py-1.5 text-xs font-medium transition-colors capitalize',
            filterType === type ? 'bg-indigo-600 text-white' : 'border border-gray-700 text-gray-400 hover:bg-gray-800'
          ]"
        >
          {{ type }}
        </button>
      </div>
      <button @click="startCreate" class="rounded-lg border border-gray-700 px-4 py-1.5 text-sm text-gray-300 hover:bg-gray-800">+ New Persona</button>
    </div>

    <!-- Create Form -->
    <div v-if="showCreate" class="rounded-lg border border-gray-800 bg-gray-900 p-4 space-y-3">
      <h3 class="text-sm font-semibold text-gray-200">Create New Persona</h3>
      <input v-model="form.name" placeholder="Persona name" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none" />
      <select v-model="form.type" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100">
        <option v-for="t in createTypes" :key="t" :value="t">{{ t }}</option>
      </select>
      <input v-model="form.description" placeholder="Description" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none" />
      <textarea v-model="form.instructions" placeholder="Persona instructions (markdown)" rows="5" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none resize-none" />
      <div class="flex gap-2 justify-end">
        <button @click="cancelForm" class="rounded-lg border border-gray-700 px-4 py-2 text-sm text-gray-400">Cancel</button>
        <button @click="savePersona" :disabled="saving" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500 disabled:opacity-50">Create</button>
      </div>
    </div>

    <div v-if="loading" class="text-gray-500">Loading...</div>
    <div v-else class="grid gap-4 lg:grid-cols-3">
      <!-- Persona List -->
      <div class="lg:col-span-1 space-y-2">
        <div
          v-for="persona in filteredPersonas"
          :key="persona.id"
          @click="selectPersona(persona)"
          :class="[
            'cursor-pointer rounded-lg border p-3 transition-colors',
            selectedPersona?.id === persona.id ? 'border-indigo-600 bg-indigo-900/20' : 'border-gray-800 bg-gray-900 hover:border-gray-700'
          ]"
        >
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-gray-200">{{ persona.name }}</span>
            <span :class="['rounded px-1.5 py-0.5 text-xs', typeColor[persona.type]]">{{ persona.type }}</span>
          </div>
          <p class="mt-1 text-xs text-gray-500 line-clamp-2">{{ persona.description }}</p>
        </div>
      </div>

      <!-- Persona Detail / Edit -->
      <div v-if="editingPersona && selectedPersona" class="lg:col-span-2 rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-3">
        <h3 class="text-sm font-semibold text-gray-200">Edit Persona</h3>
        <input v-model="form.name" placeholder="Persona name" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none" />
        <select v-model="form.type" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100">
          <option v-for="t in createTypes" :key="t" :value="t">{{ t }}</option>
        </select>
        <input v-model="form.description" placeholder="Description" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none" />
        <textarea v-model="form.instructions" placeholder="Persona instructions (markdown)" rows="8" class="w-full rounded-lg border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none resize-none" />
        <div class="flex gap-2 justify-end">
          <button @click="cancelForm" class="rounded-lg border border-gray-700 px-4 py-2 text-sm text-gray-400">Cancel</button>
          <button @click="savePersona" :disabled="saving" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500 disabled:opacity-50">Save</button>
        </div>
      </div>
      <div v-else-if="selectedPersona" class="lg:col-span-2 rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-base font-semibold text-gray-100">{{ selectedPersona.name }}</h3>
            <p class="mt-1 text-sm text-gray-400">{{ selectedPersona.description }}</p>
          </div>
          <div class="flex gap-2">
            <button @click="startEdit" class="rounded border border-gray-700 px-3 py-1 text-xs text-gray-400 hover:bg-gray-800">Edit</button>
            <button @click="deletePersona(selectedPersona.id)" class="rounded border border-red-900 px-3 py-1 text-xs text-red-400 hover:bg-red-900/20">Delete</button>
          </div>
        </div>

        <!-- Style Rules -->
        <div>
          <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Style Rules</h4>
          <ul class="space-y-1">
            <li v-for="(rule, i) in selectedPersona.style_rules" :key="i" class="flex items-start gap-2 text-sm text-gray-300">
              <span class="text-indigo-400 mt-0.5">•</span> {{ rule }}
            </li>
          </ul>
        </div>

        <!-- Boundaries -->
        <div v-if="selectedPersona.boundaries.length">
          <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Boundaries</h4>
          <ul class="space-y-1">
            <li v-for="(b, i) in selectedPersona.boundaries" :key="i" class="flex items-start gap-2 text-sm text-amber-300">
              <span class="mt-0.5">⚠</span> {{ b }}
            </li>
          </ul>
        </div>

        <!-- Instructions -->
        <div>
          <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Instructions</h4>
          <pre class="rounded bg-gray-800 p-3 text-xs text-gray-400 whitespace-pre-wrap">{{ selectedPersona.instructions }}</pre>
        </div>
      </div>

      <div v-else class="lg:col-span-2 flex items-center justify-center text-gray-600">
        Select a persona to view details
      </div>
    </div>
  </div>
</template>
