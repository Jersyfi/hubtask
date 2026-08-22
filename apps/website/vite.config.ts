// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { defineConfig } from 'vite';

// The website is **not** embedded into the binary. It has no API contract with the server and no
// reason to be in it, so it builds to a directory and deploys as static files (ADR-0028). Where
// it deploys is not decided here.
export default defineConfig({
  base: '/',
  build: { outDir: 'dist', emptyOutDir: true, sourcemap: true },
});
