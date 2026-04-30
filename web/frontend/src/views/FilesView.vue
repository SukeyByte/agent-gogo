<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '../api'
import type { FileEntry } from '../api/types'

const files = ref<FileEntry[]>([])
const loading = ref(true)
const selectedFile = ref<FileEntry | null>(null)
const fileContent = ref('')

onMounted(async () => {
  files.value = await api.listFiles()
  loading.value = false
})

function selectFile(file: FileEntry) {
  if (file.type === 'dir') return
  selectedFile.value = file
  // Mock file content
  if (file.path.includes('.md')) {
    fileContent.value = `# ${file.name}\n\nThis is a mock file content for ${file.path}.\n\nSize: ${file.size} bytes`
  } else if (file.path.includes('.patch')) {
    fileContent.value = `--- a/internal/auth/session.go\n+++ b/internal/auth/session.go\n@@ -42,6 +42,8 @@\n func (s *Session) Validate() error {\n   if s.Expired() {\n     return ErrSessionExpired\n+    // TODO: refresh token\n+    return s.refreshToken()\n   }\n   return nil\n }`
  } else if (file.path.includes('.db')) {
    fileContent.value = `SQLite Database\nSize: ${(file.size / 1024 / 1024).toFixed(2)} MB\n\nTables: projects, tasks, task_dependencies, task_attempts, task_events, tool_calls, observations, test_results, review_results, artifacts`
  } else {
    fileContent.value = `Binary file: ${file.path}\nSize: ${file.size} bytes`
  }
}

function formatSize(bytes: number): string {
  if (bytes === 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1048576).toFixed(2)} MB`
}
</script>

<template>
  <div class="space-y-4">
    <!-- Upload Area -->
    <div class="rounded-lg border-2 border-dashed border-gray-700 bg-gray-900 p-6 text-center hover:border-gray-600 transition-colors cursor-pointer">
      <div class="text-gray-500">
        <span class="text-2xl">↑</span>
        <p class="text-sm mt-1">Drop files here or click to upload</p>
      </div>
    </div>

    <div v-if="loading" class="text-gray-500">Loading...</div>
    <div v-else class="grid gap-4 lg:grid-cols-2">
      <!-- File List -->
      <div class="rounded-lg border border-gray-800 bg-gray-900 divide-y divide-gray-800">
        <div
          v-for="file in files"
          :key="file.path"
          @click="selectFile(file)"
          :class="[
            'flex items-center gap-3 px-4 py-2 cursor-pointer transition-colors',
            selectedFile?.path === file.path ? 'bg-indigo-900/20' : 'hover:bg-gray-800/50'
          ]"
        >
          <span class="text-sm">{{ file.type === 'dir' ? '📁' : '📄' }}</span>
          <div class="min-w-0 flex-1">
            <div class="text-sm text-gray-200 truncate">{{ file.name }}</div>
            <div class="text-xs text-gray-600 truncate">{{ file.path }}</div>
          </div>
          <span class="text-xs text-gray-500">{{ formatSize(file.size) }}</span>
        </div>
      </div>

      <!-- File Preview -->
      <div class="rounded-lg border border-gray-800 bg-gray-900 p-4">
        <div v-if="selectedFile">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-medium text-gray-200">{{ selectedFile.path }}</h3>
            <span class="text-xs text-gray-500">{{ formatSize(selectedFile.size) }}</span>
          </div>
          <pre class="rounded bg-gray-800 p-4 text-xs text-gray-400 whitespace-pre-wrap overflow-auto max-h-[500px]">{{ fileContent }}</pre>
        </div>
        <div v-else class="flex items-center justify-center py-16 text-gray-600">
          Select a file to preview
        </div>
      </div>
    </div>
  </div>
</template>
