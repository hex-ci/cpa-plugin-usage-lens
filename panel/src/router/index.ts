import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'overview', component: () => import('../pages/UsagePage.vue') },
  { path: '/events', name: 'events', component: () => import('../pages/EventsPage.vue') },
  { path: '/analysis', name: 'analysis', component: () => import('../pages/AnalysisPage.vue') },
  { path: '/settings', name: 'settings', component: () => import('../pages/SettingsPage.vue') },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})