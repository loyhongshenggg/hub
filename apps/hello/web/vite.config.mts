import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// Built inside the app's Docker image (see ../Dockerfile), so only use
// packages listed in this folder's package.json.
const api = `http://127.0.0.1:${process.env.HUB_PORT_API ?? 8080}`;

export default defineConfig({
  root: import.meta.dirname,
  plugins: [react(), tailwindcss()],
  server: {
    // In the hub's dev mode the core picks free ports: $PORT for this server
    // and $HUB_PORT_API for the Go API. Standalone: 5174 and 8080.
    port: Number(process.env.PORT) || 5174,
    strictPort: Boolean(process.env.PORT),
    host: '127.0.0.1',
    proxy: { '/api': api, '/healthz': api },
  },
  build: { outDir: 'dist', emptyOutDir: true, reportCompressedSize: false },
});
