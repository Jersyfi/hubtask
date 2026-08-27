// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import adapter from '@sveltejs/adapter-static';

// The website is fully prerendered static files (ADR-0030): many mostly static pages, filesystem
// routing, search engines read real HTML, and nothing runs a server. No fallback page - a path
// that was not prerendered is a 404 the web server answers, not an app shell pretending.
/** @type {import('@sveltejs/kit').Config} */
export default {
  kit: {
    adapter: adapter({
      pages: 'dist',
      assets: 'dist',
      fallback: undefined,
      precompress: false,
      strict: true,
    }),
  },
};
