<script setup lang="ts">
defineProps<{ message?: string }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center py-16 text-gray-500">
    <div class="text-4xl mb-4">∅</div>
    <p class="text-sm">{{ message || 'Nothing here yet' }}</p>
  </div>
</template>
