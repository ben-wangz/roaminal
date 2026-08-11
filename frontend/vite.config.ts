import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    target: 'es2022',
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    sourcemap: 'hidden'
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:9846',
      '/ws': { target: 'ws://127.0.0.1:9846', ws: true }
    }
  }
});
