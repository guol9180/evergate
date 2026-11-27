import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  // @/ 别名与 tsconfig paths、components.json 保持一致（shadcn 生成的组件用 @/lib/utils）
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: 5173,
    proxy: {
      '/admin/api': 'http://localhost:8787',
      '/v1': 'http://localhost:8787',
    },
  },
  build: { outDir: 'dist' },
})
