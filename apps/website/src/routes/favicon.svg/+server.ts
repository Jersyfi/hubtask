// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The tab icon: the design system's generated file (the reduced mark, light and dark), the same
// one the web app ships, prerendered into dist/ at its stable name. Read at build time, so the
// site holds no copy of its own that could drift from the tokens.

import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';

export const prerender = true;

export function GET(): Response {
  const source = createRequire(import.meta.url).resolve('@hubtask/design-system/mark/favicon.svg');
  return new Response(readFileSync(source), { headers: { 'content-type': 'image/svg+xml' } });
}
