import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = process.env.VITE_API_URL || 'http://localhost:8080'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/agents': apiTarget,
      '/models': apiTarget,
      '/update-model': apiTarget,
      '/avatar-info': apiTarget,
      '/avatar': {
        target: apiTarget,
        changeOrigin: true,
        bypass(req) {
          if (req.url && req.url.startsWith('/avatars/')) {
            return req.url;
          }
        },
      },
      '/uploads': apiTarget,
      '/prompts': apiTarget,
      '/memory': apiTarget,
      '/tools': apiTarget,
      '/chat': apiTarget,
      '/health': apiTarget,
      '/providers': apiTarget,
      '/cloud-models': apiTarget,
      '/workspaces': apiTarget,
      '/agent': apiTarget,
      '/learning-stats': apiTarget,
      '/logs': apiTarget,
      '/rag': apiTarget,
      '/rag/add-folder': apiTarget,
      '/skills': apiTarget,         // Skill Engine (PR #21)
      '/graph': apiTarget,          // Graph Engine (PR #21)
      '/embeddings': apiTarget,     // Статус эмбеддингов (PR #21)
    },
  },
})
