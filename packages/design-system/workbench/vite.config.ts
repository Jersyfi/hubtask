// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// The workbench is a development tool and nothing else (ADR-0037). It has its own config and its
// own scripts precisely so that it cannot leak: `pnpm build` in this package still generates the
// four token targets and only those, `pnpm -r build` does not depend on this file, and no byte of
// what it produces reaches apps/webapp's bundle or the binary that embeds it (ADR-0028).
export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  // The root is workbench/, so the plugin has to be pointed at the package's svelte.config.js
  // rather than left to look beside the root - otherwise the workbench compiles with defaults
  // while src/ compiles with the project's settings, and the two disagree about runes.
  plugins: [svelte({ configFile: fileURLToPath(new URL('../svelte.config.js', import.meta.url)) })],
  build: {
    // Inside dist/, which .gitignore already covers for every workspace package. A build output
    // in a new directory is a build output somebody eventually commits.
    outDir: fileURLToPath(new URL('../dist/workbench', import.meta.url)),
    emptyOutDir: true,
  },
  // Vite's default 5173 belongs to apps/webapp. A tool that takes the application's port is a
  // tool somebody eventually stops running.
  server: { port: 5174, open: false },
});
