import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

const API_TARGET = process.env.VITE_API_TARGET || 'http://127.0.0.1:8090'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@airport/ui': path.resolve(__dirname, '../../packages/ui/src'),
    },
  },
  server: {
    host: '127.0.0.1',
    port: 5178,
    strictPort: true,
    proxy: {
      '/api': {
        target: API_TARGET,
        changeOrigin: true,
      },
      '/sub': {
        target: API_TARGET,
        changeOrigin: true,
      },
      '/link': {
        target: API_TARGET,
        changeOrigin: true,
      },
    },
  },
})
