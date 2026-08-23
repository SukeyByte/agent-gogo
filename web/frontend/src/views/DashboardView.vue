<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { api } from '../api'
import StatusBadge from '../components/common/StatusBadge.vue'
import type { DashboardStats, ProviderStatus, Project, TokenUsageSnapshot } from '../api/types'

const stats = ref<DashboardStats>({ project_count: 0, task_count: 0, done_count: 0, running_count: 0, failed_count: 0 })
const providers = ref<ProviderStatus[]>([])
const recentProjects = ref<Project[]>([])
const tokenUsage = ref<TokenUsageSnapshot | null>(null)
const loading = ref(true)

onMounted(async () => {
  const [s, p, rp, tu] = await Promise.all([api.getDashboardStats(), api.getProviders(), api.getRecentProjects(), api.getTokenUsage()])
  stats.value = s
  providers.value = p
  recentProjects.value = rp
  tokenUsage.value = tu
  loading.value = false
})

function formatTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(n)
}
</script>

<template>
  <div v-if="loading" class="text-gray-500">Loading...</div>
  <div v-else class="space-y-6">
    <!-- Stats Cards -->
    <div class="grid grid-cols-2 gap-4 lg:grid-cols-5">
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <div class="text-xs text-gray-500">Projects</div>
        <div class="mt-1 text-2xl font-bold text-gray-100">{{ stats.project_count }}</div>
      </div>
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <div class="text-xs text-gray-500">Tasks</div>
        <div class="mt-1 text-2xl font-bold text-gray-100">{{ stats.task_count }}</div>
      </div>
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <div class="text-xs text-gray-500">Running</div>
        <div class="mt-1 text-2xl font-bold text-blue-400">{{ stats.running_count }}</div>
      </div>
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <div class="text-xs text-gray-500">Done</div>
        <div class="mt-1 text-2xl font-bold text-green-400">{{ stats.done_count }}</div>
      </div>
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <div class="text-xs text-gray-500">Failed</div>
        <div class="mt-1 text-2xl font-bold text-red-400">{{ stats.failed_count }}</div>
      </div>
    </div>

    <!-- Token Usage -->
    <div v-if="tokenUsage" class="rounded-lg border border-gray-800 bg-gray-900 p-4">
      <div class="mb-3 flex items-center justify-between">
        <h3 class="text-sm font-medium text-gray-300">Token Usage</h3>
        <span class="text-xs text-gray-500">{{ tokenUsage.totals.calls }} calls</span>
      </div>
      <div class="grid grid-cols-3 gap-4">
        <div>
          <div class="text-xs text-gray-500">Input</div>
          <div class="mt-1 text-xl font-bold text-gray-100">{{ formatTokens(tokenUsage.totals.input_tokens) }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500">Output</div>
          <div class="mt-1 text-xl font-bold text-gray-100">{{ formatTokens(tokenUsage.totals.output_tokens) }}</div>
        </div>
        <div>
          <div class="text-xs text-gray-500">Total</div>
          <div class="mt-1 text-xl font-bold text-indigo-400">{{ formatTokens(tokenUsage.totals.total_tokens) }}</div>
        </div>
      </div>
      <div v-if="Object.keys(tokenUsage.by_stage).length" class="mt-3 flex flex-wrap gap-2">
        <span
          v-for="(counts, stage) in tokenUsage.by_stage"
          :key="stage"
          class="rounded-full bg-gray-800 px-2.5 py-1 text-xs text-gray-400"
        >
          {{ stage }} · {{ formatTokens(counts.total_tokens) }}
        </span>
      </div>
    </div>

    <div class="grid gap-6 lg:grid-cols-2">
      <!-- Provider Status -->
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-300">Provider Status</h3>
        <div class="space-y-3">
          <div v-for="p in providers" :key="p.name" class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span :class="['h-2 w-2 rounded-full', p.status === 'connected' ? 'bg-green-500' : p.status === 'error' ? 'bg-red-500' : 'bg-yellow-500']"></span>
              <span class="text-sm text-gray-200">{{ p.name }}</span>
            </div>
            <span class="text-xs text-gray-500">{{ p.detail }}</span>
          </div>
        </div>
      </div>

      <!-- Recent Projects -->
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-300">Recent Projects</h3>
        <div class="space-y-2">
          <RouterLink
            v-for="p in recentProjects"
            :key="p.id"
            :to="`/projects/${p.id}`"
            class="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-gray-800 transition-colors"
          >
            <div>
              <div class="text-sm text-gray-200">{{ p.name }}</div>
              <div class="text-xs text-gray-500 truncate max-w-xs">{{ p.goal }}</div>
            </div>
            <StatusBadge :status="p.status" />
          </RouterLink>
        </div>
      </div>
    </div>

    <!-- Quick Actions -->
    <div class="flex gap-3">
      <RouterLink to="/chat" class="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 transition-colors">
        New Chat
      </RouterLink>
      <RouterLink to="/projects" class="rounded-lg border border-gray-700 px-4 py-2 text-sm font-medium text-gray-300 hover:bg-gray-800 transition-colors">
        View All Projects
      </RouterLink>
    </div>
  </div>
</template>
