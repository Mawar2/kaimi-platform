import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// base: './' keeps every asset reference relative so the built bundle is
// relocatable — it is embedded via go:embed and served from an arbitrary path
// prefix by the Go Handler, with no knowledge of its mount point.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
