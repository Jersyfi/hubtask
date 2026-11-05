// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Run R3 of test/load/PLAN.md: the built bundle walked against a running server holding the
// capacity dataset, every console error, uncaught error, failed request and 5xx counted. The
// capacity run starts it per measured workspace (HUBTASK_LOAD_WALK). Per screen it records S7's
// figures as well: the script and style bytes fetched for it and the time until it settled. By
// hand:
//
//   node e2e/scalewalk.mjs <api origin> <bearer> <out.json> <path>...
//
// The bundle is served from dist/ as the binary serves it; /api/v1 is forwarded to the server by
// the browser context, so the page's origin and the server's need no CORS between them.

import { chromium } from 'playwright';
import { writeFileSync } from 'node:fs';
import { serve } from './serve.mjs';
import { fold, record } from './scalewalk-errors.mjs';
import { screenOf, tally } from './scalewalk-screens.mjs';

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
const screens = [];
// The aborts the walk made itself: the browser logs each as a console error without its URL.
let walkAborts = 0;
try {
  const context = await browser.newContext();
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    // The stream is a long-lived connection the walk does not need; its absence is not an error.
    // Nor the snapshot: route.fetch holds a response whole before it hands it on, and the replica
    // of a million entries is a gigabyte the walk would hold instead of the screens it measures.
    // The engine then walks :pull page by page, which the walk forwards like any other request.
    if (url.pathname.endsWith('/api/v1/stream') || url.pathname.endsWith('/api/v1/sync:snapshot')) {
      walkAborts += 1;
      return route.abort();
    }
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
  // The bundle's files each screen fetched: what is cached from an earlier screen is not fetched
  // again, so a screen's bytes are what a first visit to it costs after the screens before it.
  let fetched = [];
  page.on('response', (response) => {
    const kind = response.request().resourceType();
    if ((kind === 'script' || kind === 'stylesheet') && response.url().startsWith(served.origin)) {
      fetched.push(response.body().then((body) => ({ kind, body }), () => ({ kind, body: Buffer.alloc(0) })));
    }
  });
  for (const path of paths) {
    fetched = [];
    const began = Date.now();
    let settledMs = null;
    try {
      await page.goto(served.origin + path, { waitUntil: 'load', timeout: SETTLE_MS });
      await settled(inFlight, Date.now() + SETTLE_MS);
      settledMs = Date.now() - began;
    } catch (error) {
      events.push({ level: 'ERROR', message: `did not settle: ${screenOf(path)} ${error.name}` });
    }
    screens.push({ screen: screenOf(path), settled_ms: settledMs, ...tally(await Promise.all(fetched)) });
  }
} finally {
  await browser.close();
  await served.close?.();
}
// The stream's and the snapshot's aborts are the walk's doing, not the page's.
const rows = fold(events.filter((e) => {
  if (/\/api\/v1\/(stream\b|sync:snapshot)/.test(e.message)) return false;
  if (walkAborts > 0 && e.message === 'Failed to load resource: net::ERR_FAILED') {
    walkAborts -= 1;
    return false;
  }
  return true;
}));
writeFileSync(out, JSON.stringify({ rows, screens }, null, 2) + '\n');
console.log(`${rows.length} browser rows over ${paths.length} screens`);
for (const s of screens) {
  console.log(`screen ${s.screen}: settled ${s.settled_ms ?? 'never'} ms, ${s.script_gzip_bytes} script and ${s.style_gzip_bytes} style gzip bytes in ${s.files} files`);
}
for (const row of rows) console.log(`browser ${row.level} ×${row.count}: ${row.message}`);
