import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// Built inside the app's Docker image (see ../Dockerfile), so only use
// packages listed in this folder's package.json.
export default defineConfig({
  root: import.meta.dirname,
  plugins: [react(), tailwindcss()],
  server: {
    port: 5174,
    host: 'localhost',
    // Dev: run the Go API (`pnpm nx serve hello-api`) next to this.
    proxy: { '/api': 'http://localhost:8080' },
  },
  build: { outDir: 'dist', emptyOutDir: true, reportCompressedSize: false },
});
