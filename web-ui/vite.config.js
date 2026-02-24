import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 5180,
    open: true,
    strictPort: true,
    cors: true,
    hmr: {
      clientPort: 443
    }
  },
  build: {
    outDir: 'dist'
  }
});