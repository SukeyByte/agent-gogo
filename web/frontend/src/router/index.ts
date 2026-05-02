import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: () => import('../views/DashboardView.vue') },
    { path: '/chat', name: 'chat', component: () => import('../views/ChatView.vue') },
    { path: '/projects', name: 'projects', component: () => import('../views/ProjectsView.vue') },
    { path: '/projects/:id', name: 'project-detail', component: () => import('../views/ProjectDetailView.vue') },
    { path: '/tasks/:id', name: 'task-detail', component: () => import('../views/TaskDetailView.vue') },
    { path: '/browser', name: 'browser', component: () => import('../views/BrowserView.vue') },
    { path: '/skills', name: 'skills', component: () => import('../views/SkillsView.vue') },
    { path: '/personas', name: 'personas', component: () => import('../views/PersonasView.vue') },
    { path: '/memory', name: 'memory', component: () => import('../views/MemoryView.vue') },
    { path: '/files', name: 'files', component: () => import('../views/FilesView.vue') },
    { path: '/channels', name: 'channels', component: () => import('../views/ChannelsView.vue') },
    { path: '/config', name: 'config', component: () => import('../views/ConfigView.vue') },
  ],
})

export default router
