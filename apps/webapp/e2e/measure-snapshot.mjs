// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Measures the initial synchronisation in a real engine (F6-04, offline-sync.md §12 SY-B): the
// built bundle, an account and a snapshot file `hubctl sync snapshot --out` wrote, and the time
// from navigation until the replica holds the cursor - which is written after the last record.
//
//   node e2e/measure-snapshot.mjs <snapshot.ndjson> [chromium|firefox|webkit]
//
// Not a test: it prints a measurement, and what it measures depends on the file and the machine.

import { chromium, firefox, webkit } from 'playwright';
import { readFileSync } from 'node:fs';
import { serve } from './serve.mjs';
const [file, engineName = 'chromium'] = process.argv.slice(2);
const body = readFileSync(file, 'utf8');
const served = await serve(process.cwd() + '/dist');
const browser = await ({ chromium, firefox, webkit })[engineName].launch();
const context = await browser.newContext();
const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'E', status: 'ACTIVE', locale: 'en' };
await context.route('**/api/v1/**', async (route) => {
  const url = new URL(route.request().url());
  if (url.pathname.endsWith('/api/v1/stream')) return route.abort();
  if (url.pathname.endsWith('/api/v1/sync:snapshot')) return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body });
  if (url.pathname.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false } });
  if (url.pathname.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
  return route.fulfill({ json: { data: [], items: [], page: { next_cursor: null, has_more: false } } });
});
await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e'); sessionStorage.setItem('hubtask.refresh', 'e2e'); });
const page = await context.newPage();
const t0 = Date.now();
await page.goto(served.origin + '/');
const database = `hubtask:${served.origin}:${ACCOUNT.id}`;
const look = () => page.evaluate((name) => new Promise((resolve) => {
  const open = indexedDB.open(name);
  open.onerror = () => resolve({ count: 0, cursor: false });
  open.onsuccess = () => {
    const db = open.result;
    try {
      const store = db.transaction('records', 'readonly').objectStore('records');
      const all = store.getAll();
      all.onsuccess = () => { const rows = all.result; db.close(); resolve({ count: rows.length, cursor: rows.some((r) => r.collection === 'meta' && r.id === 'position' && r.value?.cursor) }); };
      all.onerror = () => { db.close(); resolve({ count: 0, cursor: false }); };
    } catch { db.close(); resolve({ count: 0, cursor: false }); }
  };
}), database);
let cursorAt;
let last = { count: -1 };
let stableSince = Date.now();
for (;;) {
  const now = await look();
  if (now.cursor && cursorAt === undefined) cursorAt = (Date.now() - t0) / 1000;
  if (now.count !== last.count) { last = now; stableSince = Date.now(); }
  if (cursorAt !== undefined && Date.now() - stableSince > 2000) break;
  if (Date.now() - t0 > 600_000) break;
  await new Promise((r) => setTimeout(r, 200));
}
console.log(`${engineName}: ${last.count} records in the store; the cursor held after ${cursorAt?.toFixed(1)} s from navigation`);
await browser.close(); await served.close();
