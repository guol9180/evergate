import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      '/admin/api': 'http://localhost:8787',
      '/v1': 'http://localhost:8787',
    },
  },
  build: { outDir: 'dist' },
})
