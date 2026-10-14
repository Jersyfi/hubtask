// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The trash says when an entry goes (UC-LIF-02 check 6): the days left are counted to the date the
// server gives the entry - the retention pass's own, the trash period and the offline window
// together - and not to the trash period alone, which the pass outlasts when the window is longer.
//
// Chromium only: what this asserts is the sentence a reader reads.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, COLLECTION, HUB, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const DAY = 24 * 60 * 60 * 1000;

const served = await serve(DIST);
test.after(() => served.close());

test('chromium: the trash counts down to the day the automatic removal may take an entry', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());

  const deleted = new Date(Date.now() - 10 * DAY);
  const entry = {
    kind: 'ITEM', id: '01a0e2e0-0000-7000-8000-0000000009a1', trash_batch_id: '01a0e2e0-0000-7000-8000-0000000009b1',
    deleted_at: deleted.toISOString(),
    // A 30-day period and a 90-day offline window: the pass waits for the window.
    purge_after: new Date(deleted.getTime() + 90 * DAY).toISOString(),
    deleted_by: { type: 'USER', id: ACCOUNT.id }, title: 'Order the old tiles', subtype: 'TASK',
    hub_id: HUB.id, collection_id: COLLECTION.id, parent_id: null, version: 3,
  };
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/trash') return route.fulfill({ json: { data: [entry], page: { next_cursor: null, has_more: false } } });
    // The policy this reader may read says 30 days; the entry's own date is what counts.
    if (path === '/retention-policies') {
      return route.fulfill({ json: [{ id: 'p1', data_kind: 'TRASH', retain_days: 30, action: 'DELETE', enabled: true, scope: { kind: 'TENANT' } }] });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/trash`);
  await page.getByText(entry.title).waitFor({ timeout: 15_000 });
  await page.getByText('Days left before it goes for good: 80.').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByText('Days left before it goes for good: 20.').count(), 0, 'the trash counted from the period alone');
});
