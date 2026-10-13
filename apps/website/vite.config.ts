// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// The website is **not** embedded into the binary. It has no API contract with the server and no
// reason to be in it, so it builds to a directory and deploys as static files (ADR-0028): the
// workflow in .github/workflows/website.yml publishes dist/ to the hubtask.eu webspace.
//
// It is fully prerendered static files (ADR-0030): many mostly static pages, filesystem routing,
// search engines read real HTML, and nothing runs a server. No fallback page - a path that was not
// prerendered is a 404 the web server answers, not an app shell pretending.
export default defineConfig({
  plugins: [
    sveltekit({
      adapter: adapter({
        pages: 'dist',
        assets: 'dist',
        fallback: undefined,
        precompress: false,
        strict: true,
      }),
    }),
  ],
});
