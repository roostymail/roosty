/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import solid from 'vite-plugin-solid'

export default defineConfig({
  plugins: [solid()],
  test: { environment: 'jsdom', include: ['src/**/*.test.ts'] },
  build: { outDir: 'dist', target: 'es2022', assetsInlineLimit: 0 },
  server: {
    port: 5173,
    proxy: { '/api': 'http://127.0.0.1:8090', '/img': 'http://127.0.0.1:8090', '/branding': 'http://127.0.0.1:8090' },
  },
})
