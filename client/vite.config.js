import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: { port: 5173 },
  // react-player lazy-loads HLS/DASH engines we never request for YouTube sources.
  build: { chunkSizeWarningLimit: 900 },
})
