// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// The website is **not** embedded into the binary. It has no API contract with the server and no
// reason to be in it, so it builds to a directory and deploys as static files (ADR-0028): the
// workflow in .github/workflows/website.yml publishes dist/ to the hubtask.eu webspace.
export default defineConfig({
  plugins: [sveltekit()],
});
