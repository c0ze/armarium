import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// `npm run dev` proxies the API to a local `armarium serve` on :8580.
const backend = 'http://127.0.0.1:8580';

export default defineConfig({
  plugins: [svelte()],
  build: { outDir: 'dist', emptyOutDir: true, assetsDir: 'assets' },
  server: {
    proxy: { '/api': backend, '/read': backend, '/opds': backend },
  },
  test: { environment: 'node' },
});
