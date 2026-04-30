<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import { api } from '../api'
import type { AppConfig } from '../api/types'

const config = reactive<AppConfig>({
  llm: { provider: '', model: '', base_url: '', api_key: '', timeout: 0 },
  embedding: { provider: '', model: '', base_url: '', api_key: '' },
  browser: { provider: '', mcp_url: '', headless: false, timeout: 0 },
  storage: { workspace_path: '', sqlite_path: '', artifact_path: '', log_path: '', skill_roots: [], persona_path: '' },
  memory: { max_working_items: 0, max_project_items: 0, default_scope: '', enable_auto_extract: false },
  runtime: { max_tasks_per_project: 0, max_retries: 0, token_budget: 0, enable_prompt_cache: false, enable_auto_repair: false, enable_debug_log: false },
  chain_router: { l0_max_tokens: 0, l1_max_tools: 0, l2_max_tasks: 0, auto_plan_threshold: '' },
  security: { require_confirm_high_risk: false, allow_shell: false, allow_auto_execute_high_risk: false },
})
const loading = ref(true)
const saving = ref(false)
const saved = ref(false)
const activeSection = ref('llm')

const sections = [
  { key: 'llm', label: 'LLM Provider' },
  { key: 'embedding', label: 'Embedding' },
  { key: 'browser', label: 'Browser' },
  { key: 'storage', label: 'Storage' },
  { key: 'memory', label: 'Memory' },
  { key: 'runtime', label: 'Runtime' },
  { key: 'chain_router', label: 'Chain Router' },
  { key: 'security', label: 'Security' },
]

onMounted(async () => {
  const c = await api.getConfig()
  Object.assign(config.llm, c.llm)
  Object.assign(config.embedding, c.embedding)
  Object.assign(config.browser, c.browser)
  Object.assign(config.storage, c.storage)
  Object.assign(config.memory, c.memory)
  Object.assign(config.runtime, c.runtime)
  Object.assign(config.chain_router, c.chain_router)
  Object.assign(config.security, c.security)
  loading.value = false
})

async function save() {
  saving.value = true
  try {
    await api.saveConfig(config)
    saved.value = true
    setTimeout(() => saved.value = false, 2000)
  } catch (err: any) {
    alert('Save failed: ' + err.message)
  }
  saving.value = false
}

function reset() {
  api.getConfig().then(c => {
    Object.assign(config.llm, c.llm)
    Object.assign(config.embedding, c.embedding)
    Object.assign(config.browser, c.browser)
    Object.assign(config.storage, c.storage)
    Object.assign(config.memory, c.memory)
    Object.assign(config.runtime, c.runtime)
    Object.assign(config.chain_router, c.chain_router)
    Object.assign(config.security, c.security)
  })
}
</script>

<template>
  <div v-if="loading" class="text-gray-500">Loading...</div>
  <div v-else class="grid gap-6 lg:grid-cols-4">
    <!-- Section Nav -->
    <nav class="space-y-1">
      <button
        v-for="s in sections"
        :key="s.key"
        @click="activeSection = s.key"
        :class="[
          'block w-full text-left rounded px-3 py-2 text-sm transition-colors',
          activeSection === s.key ? 'bg-gray-800 text-indigo-300' : 'text-gray-400 hover:bg-gray-800/50'
        ]"
      >
        {{ s.label }}
      </button>
    </nav>

    <!-- Section Content -->
    <div class="lg:col-span-3 space-y-4">
      <!-- LLM -->
      <div v-if="activeSection === 'llm'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">LLM Provider</h3>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="block text-xs text-gray-500 mb-1">Provider</label>
            <input v-model="config.llm.provider" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Model</label>
            <input v-model="config.llm.model" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Base URL</label>
            <input v-model="config.llm.base_url" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">API Key</label>
            <input v-model="config.llm.api_key" type="password" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Timeout (s)</label>
            <input v-model.number="config.llm.timeout" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
        </div>
      </div>

      <!-- Embedding -->
      <div v-if="activeSection === 'embedding'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">Embedding Provider</h3>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="block text-xs text-gray-500 mb-1">Provider</label>
            <input v-model="config.embedding.provider" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Model</label>
            <input v-model="config.embedding.model" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Base URL</label>
            <input v-model="config.embedding.base_url" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">API Key</label>
            <input v-model="config.embedding.api_key" type="password" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
        </div>
      </div>

      <!-- Browser -->
      <div v-if="activeSection === 'browser'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">Browser</h3>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="block text-xs text-gray-500 mb-1">Provider</label>
            <input v-model="config.browser.provider" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">MCP URL</label>
            <input v-model="config.browser.mcp_url" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Timeout (s)</label>
            <input v-model.number="config.browser.timeout" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div class="flex items-center gap-2">
            <input v-model="config.browser.headless" type="checkbox" class="rounded" />
            <label class="text-sm text-gray-300">Headless Mode</label>
          </div>
        </div>
      </div>

      <!-- Storage -->
      <div v-if="activeSection === 'storage'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">Storage</h3>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="block text-xs text-gray-500 mb-1">Workspace Path</label>
            <input v-model="config.storage.workspace_path" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">SQLite Path</label>
            <input v-model="config.storage.sqlite_path" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Artifact Path</label>
            <input v-model="config.storage.artifact_path" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Log Path</label>
            <input v-model="config.storage.log_path" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Persona Path</label>
            <input v-model="config.storage.persona_path" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
          </div>
        </div>
        <div>
          <label class="block text-xs text-gray-500 mb-1">Skill Roots</label>
          <div class="space-y-1">
            <div v-for="(root, i) in config.storage.skill_roots" :key="i" class="flex gap-2">
              <input :value="root" @input="config.storage.skill_roots[i] = ($event.target as HTMLInputElement).value" class="flex-1 rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 font-mono focus:border-indigo-500 focus:outline-none" />
              <button @click="config.storage.skill_roots.splice(i, 1)" class="text-red-400 text-sm px-2">✕</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Memory -->
      <div v-if="activeSection === 'memory'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">Memory</h3>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="block text-xs text-gray-500 mb-1">Max Working Items</label>
            <input v-model.number="config.memory.max_working_items" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Max Project Items</label>
            <input v-model.number="config.memory.max_project_items" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Default Scope</label>
            <select v-model="config.memory.default_scope" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none">
              <option value="working">Working</option>
              <option value="project">Project</option>
              <option value="long_term">Long-Term</option>
            </select>
          </div>
          <div class="flex items-center gap-2">
            <input v-model="config.memory.enable_auto_extract" type="checkbox" class="rounded" />
            <label class="text-sm text-gray-300">Auto Extract</label>
          </div>
        </div>
      </div>

      <!-- Runtime -->
      <div v-if="activeSection === 'runtime'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">Runtime</h3>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="block text-xs text-gray-500 mb-1">Max Tasks Per Project</label>
            <input v-model.number="config.runtime.max_tasks_per_project" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Max Retries</label>
            <input v-model.number="config.runtime.max_retries" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Token Budget</label>
            <input v-model.number="config.runtime.token_budget" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div class="flex flex-col gap-2 justify-center">
            <label class="flex items-center gap-2">
              <input v-model="config.runtime.enable_prompt_cache" type="checkbox" class="rounded" />
              <span class="text-sm text-gray-300">Prompt Cache</span>
            </label>
            <label class="flex items-center gap-2">
              <input v-model="config.runtime.enable_auto_repair" type="checkbox" class="rounded" />
              <span class="text-sm text-gray-300">Auto Repair</span>
            </label>
            <label class="flex items-center gap-2">
              <input v-model="config.runtime.enable_debug_log" type="checkbox" class="rounded" />
              <span class="text-sm text-gray-300">Debug Log</span>
            </label>
          </div>
        </div>
      </div>

      <!-- Chain Router -->
      <div v-if="activeSection === 'chain_router'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">Chain Router</h3>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="block text-xs text-gray-500 mb-1">L0 Max Tokens</label>
            <input v-model.number="config.chain_router.l0_max_tokens" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">L1 Max Tools</label>
            <input v-model.number="config.chain_router.l1_max_tools" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">L2 Max Tasks</label>
            <input v-model.number="config.chain_router.l2_max_tasks" type="number" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none" />
          </div>
          <div>
            <label class="block text-xs text-gray-500 mb-1">Auto Plan Threshold</label>
            <select v-model="config.chain_router.auto_plan_threshold" class="w-full rounded border border-gray-700 bg-gray-800 px-3 py-2 text-sm text-gray-100 focus:border-indigo-500 focus:outline-none">
              <option value="low">Low</option>
              <option value="medium">Medium</option>
              <option value="high">High</option>
            </select>
          </div>
        </div>
      </div>

      <!-- Security -->
      <div v-if="activeSection === 'security'" class="rounded-lg border border-gray-800 bg-gray-900 p-5 space-y-4">
        <h3 class="text-sm font-medium text-gray-300">Security</h3>
        <div class="space-y-3">
          <label class="flex items-center gap-2">
            <input v-model="config.security.require_confirm_high_risk" type="checkbox" class="rounded" />
            <span class="text-sm text-gray-300">Require confirmation for high-risk operations</span>
          </label>
          <label class="flex items-center gap-2">
            <input v-model="config.security.allow_shell" type="checkbox" class="rounded" />
            <span class="text-sm text-gray-300">Allow shell commands</span>
          </label>
          <label class="flex items-center gap-2">
            <input v-model="config.security.allow_auto_execute_high_risk" type="checkbox" class="rounded" />
            <span class="text-sm text-gray-300">Allow auto-execute high-risk tasks (bypass confirmation)</span>
          </label>
        </div>
      </div>

      <!-- Actions -->
      <div class="flex gap-3">
        <button @click="save" :disabled="saving" class="rounded-lg bg-indigo-600 px-6 py-2 text-sm text-white hover:bg-indigo-500 disabled:opacity-50">
          {{ saving ? 'Saving...' : 'Save' }}
        </button>
        <button @click="reset" class="rounded-lg border border-gray-700 px-4 py-2 text-sm text-gray-400 hover:bg-gray-800">Reset</button>
        <span v-if="saved" class="flex items-center text-sm text-green-400">Saved!</span>
      </div>
    </div>
  </div>
</template>
