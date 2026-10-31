// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Run R3 of test/load/PLAN.md: the built bundle walked against a running server holding the
// capacity dataset, every console error, uncaught error, failed request and 5xx counted. The
// capacity run starts it per measured workspace (HUBTASK_LOAD_WALK); by hand:
//
//   node e2e/scalewalk.mjs <api origin> <bearer> <out.json> <path>...
//
// The bundle is served from dist/ as the binary serves it; /api/v1 is forwarded to the server by
// the browser context, so the page's origin and the server's need no CORS between them.

import { chromium } from 'playwright';
import { writeFileSync } from 'node:fs';
import { serve } from './serve.mjs';
import { fold, record } from './scalewalk-errors.mjs';

const [api, bearer, out, ...paths] = process.argv.slice(2);
if (!api || !bearer || !out || paths.length === 0) {
  console.error('usage: node e2e/scalewalk.mjs <api origin> <bearer> <out.json> <path>...');
  process.exit(2);
}
// How long one screen may take to settle; a screen that does not is itself a row.
const SETTLE_MS = 30_000;

const served = await serve(process.cwd() + '/dist');
const browser = await chromium.launch();
const events = [];
try {
  const context = await browser.newContext();
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    // The stream is a long-lived connection the walk does not need; its absence is not an error.
    if (url.pathname.endsWith('/api/v1/stream')) return route.abort();
    const response = await route.fetch({ url: api + url.pathname + url.search, timeout: SETTLE_MS });
    return route.fulfill({ response });
  });
  await context.addInitScript((token) => sessionStorage.setItem('hubtask.bearer', token), bearer);
  const page = await context.newPage();
  record(page, events);
  for (const path of paths) {
    try {
      await page.goto(served.origin + path, { waitUntil: 'networkidle', timeout: SETTLE_MS });
    } catch (error) {
      events.push({ level: 'ERROR', message: `did not settle: ${path.replace(/[0-9a-f-]{36}/g, '?')} ${error.name}` });
    }
  }
} finally {
  await browser.close();
  await served.close?.();
}
// The stream's own abort is the walk's doing, not the page's.
const rows = fold(events.filter((e) => !/\/api\/v1\/stream\b/.test(e.message)));
writeFileSync(out, JSON.stringify(rows, null, 2) + '\n');
console.log(`${rows.length} browser rows over ${paths.length} screens`);
for (const row of rows) console.log(`browser ${row.level} ×${row.count}: ${row.message}`);
