// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection without a column still holds entries: the board draws them in the lane of the
// entries without a column (ux-inventory.md E5), so switching from the list to the board never
// makes the work disappear. The empty state is only for a board with nothing on it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The fixture's collection with no column at all, and `entries` on it. */
async function withoutColumns(browser, entries) {
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/buckets')) return route.fulfill({ json: [] });
    if (path.endsWith('/api/v1/items:query') && request.postDataJSON()?.group_by) {
      const groups = entries.length > 0 ? [{ key: null, count: entries.length, data: entries, page: { next_cursor: null, has_more: false } }] : [];
      return route.fulfill({ json: { data: [], groups, page: { next_cursor: null, has_more: false }, total: entries.length } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('radio', { name: 'Board' }).click();
  return { page, failures, close: () => context.close() };
}

test('chromium: entries without a column are drawn in their lane while the collection has no column', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const entries = ITEMS.slice(0, 2).map((item) => ({ ...item, bucket_id: null }));
  const { page, failures, close } = await withoutColumns(browser, entries);
  t.after(close);

  const lane = page.locator('[data-column-zone="none"]');
  await lane.waitFor({ timeout: 15_000 });
  assert.equal(await lane.locator('[data-card]').count(), entries.length, 'the lane does not hold every entry');
  assert.equal(await page.getByText('No columns in this collection yet', { exact: false }).count(), 0, 'the empty state hides the entries');
  assert.equal(await page.getByRole('button', { name: 'Create column' }).count(), 1, 'no way to make the first column');
  assert.deepEqual(failures, []);
});

test('chromium: a board with neither a column nor an entry says what a column is', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await withoutColumns(browser, []);
  t.after(close);

  await page.getByText('No columns in this collection yet', { exact: false }).waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('[data-column-zone]').count(), 0, 'an empty board draws a lane');
  assert.deepEqual(failures, []);
});
