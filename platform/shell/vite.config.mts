/// <reference types='vitest' />
import { nxReadySignal } from '@nx/vite/plugins/nx-ready-signal.plugin';
import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// Dev: run `pnpm nx dev core` — the core serves the shell from this dev
// server at http://hub.localhost:7000 (HMR included), so /api and app
// iframes behave exactly as in production.
export default defineConfig(() => ({
  root: import.meta.dirname,
  cacheDir: '../../node_modules/.vite/platform/shell',
  server: {
    port: 5173,
    strictPort: true,
    host: 'localhost',
  },
  preview: {
    port: 5173,
    host: 'localhost',
  },
  plugins: [react(), tailwindcss(), nxReadySignal()],
  build: {
    // The core embeds this folder (core:embed-shell).
    outDir: '../../dist/platform/shell',
    emptyOutDir: true,
    reportCompressedSize: false,
  },
  define: {
    'import.meta.vitest': undefined,
  },
  test: {
    name: '@hub/shell',
    watch: false,
    globals: true,
    environment: 'jsdom',
    setupFiles: ['src/test-setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    reporters: ['default'],
  },
}));
