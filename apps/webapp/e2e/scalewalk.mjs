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

const IDLE_MS = 500;

/** Resolves once nothing is in flight for IDLE_MS; rejects at the deadline. */
async function settled(inFlight, deadline) {
  let idleSince = inFlight.size === 0 ? Date.now() : undefined;
  for (;;) {
    if (inFlight.size > 0) idleSince = undefined;
    else idleSince ??= Date.now();
    if (idleSince !== undefined && Date.now() - idleSince >= IDLE_MS) return;
    if (Date.now() > deadline) throw Object.assign(new Error('requests still in flight'), { name: 'TimeoutError' });
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
}

const served = await serve(process.cwd() + '/dist');
const browser = await chromium.launch();
const events = [];
try {
  const context = await browser.newContext();
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    // The stream is a long-lived connection the walk does not need; its absence is not an error.
    // Nor the snapshot: route.fetch holds a response whole before it hands it on, and the replica
    // of a million entries is a gigabyte the walk would hold instead of the screens it measures.
    // The engine then walks :pull page by page, which the walk forwards like any other request.
    if (url.pathname.endsWith('/api/v1/stream') || url.pathname.endsWith('/api/v1/sync:snapshot')) return route.abort();
    try {
      const response = await route.fetch({ url: api + url.pathname + url.search, timeout: SETTLE_MS });
      return await route.fulfill({ response });
    } catch {
      // The page sees a failed request, which requestfailed records.
      return route.abort('failed').catch(() => {});
    }
  });
  await context.addInitScript((token) => sessionStorage.setItem('hubtask.bearer', token), bearer);
  const page = await context.newPage();
  record(page, events);
  // A screen has settled when none of its own requests is in flight for IDLE_MS. Not networkidle:
  // the replica's synchronisation runs for minutes in a workspace of a million entries, and the
  // screens read from the server meanwhile (offline-sync.md §1).
  const inFlight = new Set();
  const own = (request) => !/\/api\/v1\/sync[:/]/.test(request.url());
  page.on('request', (request) => own(request) && inFlight.add(request));
  for (const done of ['requestfinished', 'requestfailed']) page.on(done, (request) => inFlight.delete(request));
  for (const path of paths) {
    try {
      await page.goto(served.origin + path, { waitUntil: 'load', timeout: SETTLE_MS });
      await settled(inFlight, Date.now() + SETTLE_MS);
    } catch (error) {
      events.push({ level: 'ERROR', message: `did not settle: ${path.replace(/[0-9a-f-]{36}/g, '?')} ${error.name}` });
    }
  }
} finally {
  await browser.close();
  await served.close?.();
}
// The stream's and the snapshot's aborts are the walk's doing, not the page's.
const rows = fold(events.filter((e) => !/\/api\/v1\/(stream\b|sync:snapshot)/.test(e.message)));
writeFileSync(out, JSON.stringify(rows, null, 2) + '\n');
console.log(`${rows.length} browser rows over ${paths.length} screens`);
for (const row of rows) console.log(`browser ${row.level} ×${row.count}: ${row.message}`);
