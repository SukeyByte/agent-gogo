<script setup lang="ts">
defineProps<{ status: string }>()

const colorMap: Record<string, string> = {
  DONE: 'bg-green-900/50 text-green-300',
  COMPLETED: 'bg-green-900/50 text-green-300',
  PASSED: 'bg-green-900/50 text-green-300',
  APPROVED: 'bg-green-900/50 text-green-300',
  SUCCEEDED: 'bg-green-900/50 text-green-300',
  ACTIVE: 'bg-blue-900/50 text-blue-300',
  IN_PROGRESS: 'bg-blue-900/50 text-blue-300',
  RUNNING: 'bg-blue-900/50 text-blue-300',
  TESTING: 'bg-yellow-900/50 text-yellow-300',
  REVIEWING: 'bg-yellow-900/50 text-yellow-300',
  IMPLEMENTED: 'bg-cyan-900/50 text-cyan-300',
  READY: 'bg-gray-800 text-gray-300',
  DRAFT: 'bg-gray-800 text-gray-400',
  BLOCKED: 'bg-orange-900/50 text-orange-300',
  NEED_USER_INPUT: 'bg-orange-900/50 text-orange-300',
  REVIEW_FAILED: 'bg-red-900/50 text-red-300',
  FAILED: 'bg-red-900/50 text-red-300',
  CANCELLED: 'bg-gray-800 text-gray-500',
  ARCHIVED: 'bg-gray-800 text-gray-500',
  PENDING: 'bg-gray-800 text-gray-400',
}
</script>

<template>
  <span :class="['inline-flex items-center rounded px-2 py-0.5 text-xs font-medium', colorMap[status] || 'bg-gray-800 text-gray-400']">
    {{ status }}
  </span>
</template>
