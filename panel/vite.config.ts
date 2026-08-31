import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// base './' + hash routing: the built SPA is served from an arbitrary
// /v0/resource/plugins/<id>/ mount, so asset URLs must be relative and the
// router must use hash history (no server-side fallback for resource routes).
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: 'assets',
  },
  server: {
    proxy: {
      '/v0': {
        target: 'http://127.0.0.1:18317',
        changeOrigin: true,
      },
    },
  },
})