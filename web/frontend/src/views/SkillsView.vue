<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api'
import type { SkillCard } from '../api/types'

interface GithubRepo {
  owner: string; repo: string; full_name: string; description: string
  stars: number; html_url: string; default_branch: string
}
interface GithubFile {
  owner: string; repo: string; path: string; branch: string
}

const skills = ref<SkillCard[]>([])
const selectedSkill = ref<SkillCard | null>(null)
const searchQuery = ref('')
const loading = ref(true)
const skillRoots = ref<string[]>([])

// GitHub search state
const searchMode = ref<'local' | 'github'>('local')
const githubRepos = ref<GithubRepo[]>([])
const githubFiles = ref<GithubFile[]>([])
const selectedRepo = ref<GithubRepo | null>(null)
const githubLoading = ref(false)
const filesLoading = ref(false)
const installing = ref<string | null>(null)

onMounted(async () => {
  const [loadedSkills, config] = await Promise.all([api.listSkills(), api.getConfig()])
  skills.value = loadedSkills
  skillRoots.value = config.storage.skill_roots
  loading.value = false
})

async function refreshSkills() {
  skills.value = await api.listSkills()
}

async function searchLocal() {
  if (!searchQuery.value) {
    skills.value = await api.listSkills()
  } else {
    skills.value = await api.searchSkills(searchQuery.value)
  }
}

async function searchGithub() {
  if (!searchQuery.value) return
  githubLoading.value = true
  githubRepos.value = []
  githubFiles.value = []
  selectedRepo.value = null
  try {
    githubRepos.value = await api.searchGithubRepos(searchQuery.value)
  } catch (e: any) {
    alert('GitHub search failed: ' + e.message)
  } finally {
    githubLoading.value = false
  }
}

async function browseRepo(repo: GithubRepo) {
  selectedRepo.value = repo
  filesLoading.value = true
  githubFiles.value = []
  try {
    githubFiles.value = await api.listGithubSkillFiles(repo.owner, repo.repo, repo.default_branch)
  } catch (e: any) {
    alert('Failed to list files: ' + e.message)
  } finally {
    filesLoading.value = false
  }
}

async function installSkill(file: GithubFile) {
  installing.value = file.path
  try {
    await api.installSkill({ owner: file.owner, repo: file.repo, path: file.path, branch: file.branch })
    await refreshSkills()
  } catch (e: any) {
    alert('Install failed: ' + e.message)
  } finally {
    installing.value = null
  }
}

async function deleteSkill(id: string) {
  try {
    await api.deleteSkill(id)
    if (selectedSkill.value?.id === id) selectedSkill.value = null
    await refreshSkills()
  } catch (e: any) {
    alert('Failed to delete skill: ' + e.message)
  }
}

function isInstalled(path: string): boolean {
  const parts = path.split('/')
  let dirName = parts.length >= 2 ? parts[parts.length - 2] : ''
  if (!dirName) dirName = parts[parts.length - 1].replace(/\.md$/i, '')
  return skills.value.some(s => s.id === dirName)
}

function selectSkill(skill: SkillCard) {
  selectedSkill.value = selectedSkill.value?.id === skill.id ? null : skill
}
</script>

<template>
  <div class="space-y-4">
    <!-- Search bar -->
    <div class="flex gap-2">
      <input
        v-model="searchQuery"
        @keydown.enter="searchMode === 'local' ? searchLocal() : searchGithub()"
        placeholder="Search skills..."
        class="flex-1 rounded-lg border border-gray-700 bg-gray-900 px-4 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
      />
      <button @click="searchMode === 'local' ? searchLocal() : searchGithub()" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500">Search</button>
    </div>

    <!-- Mode tabs -->
    <div class="flex gap-2">
      <button
        @click="searchMode = 'local'"
        :class="['rounded px-3 py-1.5 text-xs font-medium transition-colors', searchMode === 'local' ? 'bg-indigo-600 text-white' : 'border border-gray-700 text-gray-400 hover:bg-gray-800']"
      >Local</button>
      <button
        @click="searchMode = 'github'"
        :class="['rounded px-3 py-1.5 text-xs font-medium transition-colors', searchMode === 'github' ? 'bg-indigo-600 text-white' : 'border border-gray-700 text-gray-400 hover:bg-gray-800']"
      >GitHub</button>
    </div>

    <!-- GitHub search results -->
    <div v-if="searchMode === 'github'" class="space-y-3">
      <div v-if="githubLoading" class="text-gray-500">Searching GitHub...</div>
      <div v-else-if="githubRepos.length === 0 && searchQuery" class="text-center py-8 text-gray-600">No repos found. Try a different keyword.</div>
      <div v-else-if="githubRepos.length === 0" class="text-center py-8 text-gray-600">Enter a keyword and click Search</div>

      <!-- Repo list -->
      <div v-if="!selectedRepo" class="space-y-2">
        <div
          v-for="repo in githubRepos"
          :key="repo.full_name"
          @click="browseRepo(repo)"
          class="cursor-pointer rounded-lg border border-gray-800 bg-gray-900 p-4 hover:border-gray-700 transition-colors"
        >
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-gray-200">{{ repo.full_name }}</span>
            <span class="text-xs text-gray-600">{{ repo.stars }} stars</span>
          </div>
          <p class="mt-1 text-xs text-gray-500 line-clamp-2">{{ repo.description }}</p>
        </div>
      </div>

      <!-- File list for selected repo -->
      <div v-else class="space-y-2">
        <div class="flex items-center gap-2 mb-2">
          <button @click="selectedRepo = null; githubFiles = []" class="text-xs text-indigo-400 hover:text-indigo-300">&larr; Back to repos</button>
          <span class="text-sm font-medium text-gray-200">{{ selectedRepo.full_name }}</span>
        </div>
        <div v-if="filesLoading" class="text-gray-500">Loading files...</div>
        <div v-else-if="githubFiles.length === 0" class="text-center py-8 text-gray-600">No SKILL.md files found in this repo</div>
        <div
          v-for="file in githubFiles"
          :key="file.path"
          class="rounded-lg border border-gray-800 bg-gray-900 p-3 flex items-center justify-between"
        >
          <span class="text-xs font-mono text-gray-400">{{ file.path }}</span>
          <div class="flex items-center gap-2">
            <span v-if="isInstalled(file.path)" class="text-xs text-gray-600">Installed</span>
            <button
              v-else
              @click="installSkill(file)"
              :disabled="installing === file.path"
              class="rounded bg-indigo-600 px-3 py-1 text-xs text-white hover:bg-indigo-500 disabled:opacity-50"
            >Install</button>
          </div>
        </div>
      </div>
    </div>

    <!-- Local skill list -->
    <div v-if="searchMode === 'local'">
      <!-- Skill Roots -->
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-3 mb-4">
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
            <div class="flex items-center gap-2">
              <button @click="deleteSkill(selectedSkill.id)" class="rounded border border-red-900 px-3 py-1 text-xs text-red-400 hover:bg-red-900/20">Delete</button>
              <span class="text-xs text-gray-500 font-mono">{{ selectedSkill.path }}</span>
            </div>
          </div>
          <p class="text-sm text-gray-400 mb-4">{{ selectedSkill.description }}</p>
          <div class="mb-4">
            <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Frontmatter</h4>
            <div class="rounded bg-gray-800 p-3 text-xs font-mono text-gray-400">
              <div v-for="(v, k) in selectedSkill.frontmatter" :key="k">{{ k }}: {{ v }}</div>
            </div>
          </div>
          <div class="mb-4">
            <h4 class="text-xs font-medium text-gray-500 uppercase mb-2">Allowed Tools</h4>
            <div class="flex flex-wrap gap-1">
              <span v-for="tool in selectedSkill.allowed_tools" :key="tool" class="rounded bg-indigo-900/30 px-2 py-0.5 text-xs text-indigo-300">{{ tool }}</span>
            </div>
          </div>
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
  </div>
</template>
