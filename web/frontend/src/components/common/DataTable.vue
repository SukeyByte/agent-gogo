<script setup lang="ts">
defineProps<{
  headers: { key: string; label: string; class?: string }[]
  rows: Record<string, any>[]
}>()
</script>

<template>
  <div class="overflow-x-auto rounded-lg border border-gray-800">
    <table class="w-full text-sm">
      <thead class="bg-gray-900">
        <tr>
          <th
            v-for="h in headers"
            :key="h.key"
            :class="['px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider', h.class]"
          >
            {{ h.label }}
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-800">
        <tr v-if="rows.length === 0">
          <td :colspan="headers.length" class="px-4 py-8 text-center text-gray-500">No data</td>
        </tr>
        <tr
          v-for="(row, i) in rows"
          :key="i"
          class="hover:bg-gray-800/50 transition-colors"
        >
          <td v-for="h in headers" :key="h.key" :class="['px-4 py-3', h.class]">
            <slot :name="h.key" :value="row[h.key]" :row="row">
              {{ row[h.key] }}
            </slot>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
