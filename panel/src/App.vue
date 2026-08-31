<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'

const tabs = [
  { to: '/', label: '概览' },
  { to: '/analysis', label: '分析' },
  { to: '/events', label: '请求事件' },
  { to: '/settings', label: '设置' },
]
</script>

<template>
  <div class="flex min-h-full flex-col bg-bg-secondary">
    <header
      class="sticky top-0 z-10 flex items-center gap-5 border-b border-border bg-bg-secondary/95 px-5 py-4 backdrop-blur"
    >
      <div class="flex items-center gap-2.5">
        <span
          class="grid h-8 w-8 place-items-center rounded-md bg-primary text-[13px] font-extrabold text-white"
          >L</span
        >
        <span class="text-[15px] font-extrabold tracking-wide">Usage Lens</span>
      </div>
      <nav class="flex gap-1">
        <RouterLink
          v-for="t in tabs"
          :key="t.to"
          :to="t.to"
          class="rounded-md px-3 py-1.5 text-[13px] font-semibold text-text-secondary hover:bg-bg-tertiary hover:text-text-primary"
          active-class="!bg-bg-tertiary !text-text-primary"
        >
          {{ t.label }}
        </RouterLink>
      </nav>
      <div class="ml-auto text-xs text-text-tertiary">用量分析 · 零独立服务</div>
    </header>
    <main class="mx-auto w-full max-w-[1400px] flex-1 px-5 py-6">
      <RouterView />
    </main>
  </div>
</template>