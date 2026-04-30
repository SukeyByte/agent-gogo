<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, RouterLink } from 'vue-router'

const route = useRoute()
const sidebarOpen = ref(true)

const navItems = [
  { path: '/', icon: '◈', label: 'Dashboard' },
  { path: '/chat', icon: '◐', label: 'Chat' },
  { path: '/projects', icon: '▣', label: 'Projects' },
  { path: '/browser', icon: '◎', label: 'Browser' },
  { path: '/skills', icon: '◇', label: 'Skills' },
  { path: '/personas', icon: '◆', label: 'Personas' },
  { path: '/memory', icon: '☰', label: 'Memory' },
  { path: '/files', icon: '▤', label: 'Files' },
  { path: '/channels', icon: '⚇', label: 'Channels' },
  { path: '/config', icon: '⚙', label: 'Config' },
]

const pageTitle = computed(() => {
  const item = navItems.find(n => n.path === route.path)
  return item?.label || 'Agent GoGo'
})
</script>

<template>
  <div class="flex h-screen overflow-hidden bg-gray-950">
    <!-- Sidebar -->
    <aside
      :class="[sidebarOpen ? 'w-56' : 'w-14', 'flex flex-col border-r border-gray-800 bg-gray-900 transition-all duration-200']"
    >
      <!-- Logo -->
      <div class="flex h-14 items-center gap-2 border-b border-gray-800 px-3">
        <button @click="sidebarOpen = !sidebarOpen" class="text-lg text-indigo-400 hover:text-indigo-300">
          ⬡
        </button>
        <span v-if="sidebarOpen" class="text-sm font-bold text-gray-100">Agent GoGo</span>
      </div>

      <!-- Nav -->
      <nav class="flex-1 overflow-y-auto py-2">
        <RouterLink
          v-for="item in navItems"
          :key="item.path"
          :to="item.path"
          :class="[
            route.path === item.path || (item.path !== '/' && route.path.startsWith(item.path))
              ? 'bg-gray-800 text-indigo-400'
              : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800/50',
            'flex items-center gap-3 px-3 py-2 text-sm transition-colors',
          ]"
        >
          <span class="w-5 text-center text-base">{{ item.icon }}</span>
          <span v-if="sidebarOpen">{{ item.label }}</span>
        </RouterLink>
      </nav>

      <!-- Status -->
      <div v-if="sidebarOpen" class="border-t border-gray-800 p-3">
        <div class="flex items-center gap-2 text-xs text-gray-500">
          <span class="h-2 w-2 rounded-full bg-green-500"></span>
          <span>Runtime Active</span>
        </div>
      </div>
    </aside>

    <!-- Main Content -->
    <main class="flex flex-1 flex-col overflow-hidden">
      <!-- Header -->
      <header class="flex h-14 items-center justify-between border-b border-gray-800 bg-gray-900 px-6">
        <h1 class="text-base font-semibold text-gray-100">{{ pageTitle }}</h1>
        <div class="flex items-center gap-4 text-xs text-gray-500">
          <span>DeepSeek Chat</span>
          <span class="h-2 w-2 rounded-full bg-green-500"></span>
        </div>
      </header>

      <!-- Page Content -->
      <div class="flex-1 overflow-y-auto p-6">
        <RouterView />
      </div>
    </main>
  </div>
</template>
