<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api'
import type { PersonaCard } from '../api/types'

const personas = ref<PersonaCard[]>([])
const selectedPersona = ref<PersonaCard | null>(null)
const filterType = ref<string>('all')
const loading = ref(true)

const personaTypes = ['all', 'main', 'channel', 'project', 'role']

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
  selectedPersona.value = selectedPersona.value?.id === persona.id ? null : persona
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
    <!-- Type Filter -->
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

      <!-- Persona Detail -->
      <div v-if="selectedPersona" class="lg:col-span-2 rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <div>
          <h3 class="text-base font-semibold text-gray-100">{{ selectedPersona.name }}</h3>
          <p class="mt-1 text-sm text-gray-400">{{ selectedPersona.description }}</p>
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
