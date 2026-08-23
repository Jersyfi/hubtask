// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// The bundle this produces is embedded into the Go binary and served from `/` by
// presentation/webui (ADR-0028). Two settings follow from that and are not stylistic:
//
//   - assets are content-hashed, because the adapter serves them `immutable, max-age=31536000`
//     while `index.html` is served `no-cache`. A hash in the name is what makes that pair safe.
//   - nothing is inlined as a data: URI beyond the trivial, so that the content security policy
//     can stay free of `'unsafe-inline'`.
export default defineConfig({
  plugins: [svelte()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    sourcemap: true,
    // 0 would inline nothing at all and cost a request per icon; the default 4 kB is a sensible
    // ceiling for a data: URI, and CSP treats those as `img-src data:` rather than as script.
    assetsInlineLimit: 4096,
    rollupOptions: {
      output: {
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
      },
    },
  },
  server: {
    // In development the API is a separate process. In production both come from the same origin,
    // which is the whole point of embedding, so no proxy exists there.
    proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: true } },
  },
});
