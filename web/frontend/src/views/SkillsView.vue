<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api'
import type { SkillCard } from '../api/types'

const skills = ref<SkillCard[]>([])
const selectedSkill = ref<SkillCard | null>(null)
const searchQuery = ref('')
const loading = ref(true)
const skillRoots = ref<string[]>([])

onMounted(async () => {
  const [loadedSkills, config] = await Promise.all([api.listSkills(), api.getConfig()])
  skills.value = loadedSkills
  skillRoots.value = config.storage.skill_roots
  loading.value = false
})

async function search() {
  if (!searchQuery.value) {
    skills.value = await api.listSkills()
  } else {
    skills.value = await api.searchSkills(searchQuery.value)
  }
}

function selectSkill(skill: SkillCard) {
  selectedSkill.value = selectedSkill.value?.id === skill.id ? null : skill
}
</script>

<template>
  <div class="space-y-4">
    <!-- Search -->
    <div class="flex gap-2">
      <input
        v-model="searchQuery"
        @keydown.enter="search"
        placeholder="Search skills by name, description, or tags..."
        class="flex-1 rounded-lg border border-gray-700 bg-gray-900 px-4 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
      />
      <button @click="search" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500">Search</button>
      <button @click="searchQuery = ''; search()" class="rounded-lg border border-gray-700 px-4 py-2 text-sm text-gray-400 hover:bg-gray-800">Clear</button>
    </div>

    <!-- Skill Roots -->
    <div class="rounded-lg border border-gray-800 bg-gray-900 p-3">
      <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Skill Roots</h4>
      <div class="flex flex-wrap gap-2">
        <span v-for="root in skillRoots" :key="root" class="rounded bg-gray-800 px-2 py-1 text-xs font-mono text-gray-400">{{ root }}</span>
        <span v-if="skillRoots.length === 0" class="text-xs text-gray-600">No skill roots configured</span>
      </div>
    </div>

    <div v-if="loading" class="text-gray-500">Loading...</div>
    <div v-else class="grid gap-4 lg:grid-cols-3">
      <!-- Skill List -->
      <div class="lg:col-span-1 space-y-2">
        <div
          v-for="skill in skills"
          :key="skill.id"
          @click="selectSkill(skill)"
          :class="[
            'cursor-pointer rounded-lg border p-3 transition-colors',
            selectedSkill?.id === skill.id ? 'border-indigo-600 bg-indigo-900/20' : 'border-gray-800 bg-gray-900 hover:border-gray-700'
          ]"
        >
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-gray-200">{{ skill.name }}</span>
            <span class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-gray-500 font-mono">{{ skill.version_hash.slice(0, 6) }}</span>
          </div>
          <p class="mt-1 text-xs text-gray-500 line-clamp-2">{{ skill.description }}</p>
          <div class="mt-2 flex flex-wrap gap-1">
            <span v-for="tool in skill.allowed_tools.slice(0, 3)" :key="tool" class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-gray-500">{{ tool }}</span>
            <span v-if="skill.allowed_tools.length > 3" class="text-xs text-gray-600">+{{ skill.allowed_tools.length - 3 }}</span>
          </div>
        </div>
      </div>

      <!-- Skill Detail -->
      <div v-if="selectedSkill" class="lg:col-span-2 rounded-lg border border-gray-800 bg-gray-900 p-5">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-base font-semibold text-gray-100">{{ selectedSkill.name }}</h3>
          <span class="text-xs text-gray-500 font-mono">{{ selectedSkill.path }}</span>
        </div>
        <p class="text-sm text-gray-400 mb-4">{{ selectedSkill.description }}</p>

        <!-- Frontmatter -->
        <div class="mb-4">
          <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Frontmatter</h4>
          <div class="rounded bg-gray-800 p-3 text-xs font-mono text-gray-400">
            <div v-for="(v, k) in selectedSkill.frontmatter" :key="k">{{ k }}: {{ v }}</div>
          </div>
        </div>

        <!-- Allowed Tools -->
        <div class="mb-4">
          <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Allowed Tools</h4>
          <div class="flex flex-wrap gap-1">
            <span v-for="tool in selectedSkill.allowed_tools" :key="tool" class="rounded bg-indigo-900/30 px-2 py-0.5 text-xs text-indigo-300">{{ tool }}</span>
          </div>
        </div>

        <!-- SKILL.md Body -->
        <div>
          <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">SKILL.md</h4>
          <pre class="rounded bg-gray-800 p-4 text-xs text-gray-400 whitespace-pre-wrap overflow-auto max-h-96">{{ selectedSkill.body }}</pre>
        </div>
      </div>

      <div v-else class="lg:col-span-2 flex items-center justify-center text-gray-600">
        Select a skill to view details
      </div>
    </div>
  </div>
</template>
