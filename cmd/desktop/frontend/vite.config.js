import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Relative base so the built assets load from Wails' embedded asset server.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
