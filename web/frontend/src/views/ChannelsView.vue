<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api'
import type { ChannelInfo } from '../api/types'

const channels = ref<ChannelInfo[]>([])
const loading = ref(true)

onMounted(async () => {
  channels.value = await api.listChannels()
  loading.value = false
})

const typeIcon: Record<string, string> = { web: '◎', cli: '>', api: '{}', telegram: '✉', discord: '◆' }
</script>

<template>
  <div class="space-y-4">
    <p class="text-sm text-gray-400">{{ channels.length }} channels configured</p>

    <div v-if="loading" class="text-gray-500">Loading...</div>
    <div v-else class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="ch in channels"
        :key="ch.id"
        class="rounded-lg border border-gray-800 bg-gray-900 p-5"
      >
        <div class="flex items-center justify-between mb-3">
          <div class="flex items-center gap-2">
            <span class="text-lg">{{ typeIcon[ch.type] || '◈' }}</span>
            <div>
              <div class="text-sm font-medium text-gray-200">{{ ch.name }}</div>
              <div class="text-xs text-gray-500">{{ ch.type }}</div>
            </div>
          </div>
          <span :class="['h-2 w-2 rounded-full', ch.enabled ? 'bg-green-500' : 'bg-gray-600']"></span>
        </div>

        <!-- Capabilities -->
        <div class="space-y-2 text-xs">
          <div>
            <span class="text-gray-500">Messages:</span>
            <div class="mt-1 flex flex-wrap gap-1">
              <span v-for="t in ch.capabilities.supported_message_types" :key="t" class="rounded bg-gray-800 px-1.5 py-0.5 text-gray-400">{{ t }}</span>
            </div>
          </div>
          <div v-if="ch.capabilities.supported_interactions.length">
            <span class="text-gray-500">Interactions:</span>
            <div class="mt-1 flex flex-wrap gap-1">
              <span v-for="t in ch.capabilities.supported_interactions" :key="t" class="rounded bg-gray-800 px-1.5 py-0.5 text-gray-400">{{ t }}</span>
            </div>
          </div>
          <div class="flex gap-3 mt-2 text-gray-500">
            <span :class="ch.capabilities.supports_confirmation ? 'text-green-400' : 'text-gray-600'">Confirmation</span>
            <span :class="ch.capabilities.supports_streaming ? 'text-green-400' : 'text-gray-600'">Streaming</span>
            <span :class="ch.capabilities.supports_file_request ? 'text-green-400' : 'text-gray-600'">Files</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
