import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  base: '/ui/',
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:18463',
      '/v1': 'http://localhost:18463',
      '/v0': 'http://localhost:18463',
    }
  },
  test: {
    environment: 'jsdom',
    globals: true,
  }
})
