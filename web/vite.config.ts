import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The built SPA is embedded into the Go binary, so assets must load from a
// relative base. During development /api requests are proxied to the Go server.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
