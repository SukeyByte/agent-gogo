<script setup lang="ts">
import { ref } from 'vue'

const url = ref('https://example.com')
const domSummary = ref('# Example Domain\n\nThis domain is for use in illustrative examples in documents.\n\nYou may use this domain in literature without prior coordination or asking for permission.\n\n[More information...](https://www.iana.org/domains/example)')
const screenshotRef = ref('')
const actionLog = ref<{ time: string; action: string; detail: string }[]>([])
const loading = ref(false)

function logAction(action: string, detail: string) {
  actionLog.value.unshift({ time: new Date().toLocaleTimeString(), action, detail })
}

async function doAction(action: string) {
  loading.value = true
  await new Promise(r => setTimeout(r, 500))
  logAction(action, url.value)
  loading.value = false
}
</script>

<template>
  <div class="space-y-4">
    <!-- URL Bar + Controls -->
    <div class="flex gap-2">
      <input
        v-model="url"
        class="flex-1 rounded-lg border border-gray-700 bg-gray-900 px-4 py-2 text-sm text-gray-100 placeholder-gray-600 focus:border-indigo-500 focus:outline-none font-mono"
        placeholder="Enter URL..."
      />
      <button @click="doAction('open')" :disabled="loading" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm text-white hover:bg-indigo-500 disabled:opacity-50">Open</button>
    </div>

    <div class="flex gap-2">
      <button @click="doAction('click')" :disabled="loading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">Click</button>
      <button @click="doAction('type')" :disabled="loading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">Type</button>
      <button @click="doAction('extract')" :disabled="loading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">Extract</button>
      <button @click="doAction('screenshot')" :disabled="loading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">Screenshot</button>
      <button @click="doAction('dom_summary')" :disabled="loading" class="rounded border border-gray-700 px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-800 disabled:opacity-50">DOM Summary</button>
    </div>

    <div class="grid gap-4 lg:grid-cols-2">
      <!-- Screenshot Area -->
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4 min-h-[400px] flex flex-col">
        <h3 class="mb-3 text-sm font-medium text-gray-300">Screenshot</h3>
        <div class="flex-1 flex items-center justify-center border border-dashed border-gray-700 rounded-lg bg-gray-800/50">
          <div v-if="screenshotRef" class="text-sm text-gray-400">Screenshot: {{ screenshotRef }}</div>
          <div v-else class="text-center text-gray-600">
            <div class="text-3xl mb-2">◎</div>
            <p class="text-xs">Open a URL to capture screenshot</p>
          </div>
        </div>
      </div>

      <!-- DOM Summary -->
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4 min-h-[400px] flex flex-col">
        <h3 class="mb-3 text-sm font-medium text-gray-300">DOM Summary</h3>
        <pre class="flex-1 overflow-auto text-xs text-gray-400 whitespace-pre-wrap">{{ domSummary }}</pre>
      </div>
    </div>

    <!-- Action Log -->
    <div v-if="actionLog.length" class="rounded-lg border border-gray-800 bg-gray-900 divide-y divide-gray-800">
      <div v-for="(log, i) in actionLog" :key="i" class="flex items-center gap-3 px-4 py-2">
        <span class="text-xs text-gray-600 font-mono">{{ log.time }}</span>
        <span class="rounded bg-gray-800 px-1.5 py-0.5 text-xs text-indigo-300">{{ log.action }}</span>
        <span class="text-xs text-gray-400">{{ log.detail }}</span>
      </div>
    </div>
  </div>
</template>
