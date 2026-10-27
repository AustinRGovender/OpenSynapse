import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// The control plane checks Host/Origin to block cross-site requests, so the
// dev proxy keeps the browser's Host header (changeOrigin: false) and also
// forwards WebSocket upgrades for live run updates.
const controlPlane = 'http://127.0.0.1:8090'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: controlPlane, changeOrigin: false, ws: true },
      '/health': { target: controlPlane, changeOrigin: false },
      '/ready': { target: controlPlane, changeOrigin: false },
      '/version': { target: controlPlane, changeOrigin: false },
    },
  },
  build: {
    rollupOptions: {
      output: {
        // Keep heavy libraries out of the main chunk so first load stays fast.
        manualChunks(id: string) {
          if (id.includes('node_modules/recharts') || id.includes('node_modules/d3-')) return 'charts'
          if (id.includes('node_modules/@xyflow')) return 'flow'
          if (id.includes('node_modules/react-dom') || id.includes('node_modules/react/')) return 'react'
          return undefined
        },
      },
    },
  },
})
