import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The dev server proxies /api server-side rather than pointing the client at a
// remote origin — that keeps CORS out of the picture and lets the local UI run
// against prod or staging by changing one env var.
const proxyTarget = process.env.VITE_PROXY_TARGET || 'http://localhost:8087'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5175,
    proxy: {
      '/api': { target: proxyTarget, changeOrigin: true },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
  },
})
