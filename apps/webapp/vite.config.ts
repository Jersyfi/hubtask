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
    // Only images may be inlined, and only small ones. The policy of ADR-0028 says
    // `img-src 'self' data:` and nothing else says data: - a small font inlined by a byte
    // threshold becomes a blocked resource and an empty glyph, which is exactly what happened
    // to two sub-4kB IBM Plex subsets before W-08 watched the console (font-src has no data:).
    assetsInlineLimit: (filePath: string, content: Buffer) =>
      /\.(svg|png|jpe?g|gif|webp|avif)$/i.test(filePath) && content.length < 4096,
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
