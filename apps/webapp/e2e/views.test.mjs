// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The collection's one *Views* menu (ux-inventory.md §E4, §E9 "Second Saved views entry"): the
// saved views that apply here, each opened by its row with its query and layout; saving what is on
// screen; changing a view; exporting the list as it is filtered without saving a view first; the
// calendar subscriptions. The page menu no longer carries a second way to the views. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** A view no other fixture record uses: the tiling, as a board. */
const OPEN_WORK = {
  id: '01a0e2e0-0000-7000-8000-0000000017a1',
  scope_type: 'COLLECTION',
  scope_id: COLLECTION.id,
  owner_id: '01a0e2e0-0000-7000-8000-000000000001',
  name: 'Open work',
  layout: 'KANBAN',
  query: { scope_container_id: COLLECTION.id, include_descendants: false, filter: { field: 'title', op: 'CONTAINS', value: 'tile' } },
  sharing: 'PRIVATE',
  version: 1,
};

const labelsOf = (menu) => menu.getByRole('menuitem').evaluateAll((items) => items.map((item) => item.querySelector('.label')?.textContent.trim() ?? ''));

test('chromium: 1280 px — one Views menu opens, saves, exports the filtered list and subscribes', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());

  const queried = [];
  const exported = [];
  await page.route('**/api/v1/views?container_id=*', (route) => route.fulfill({ json: [OPEN_WORK] }));
  await page.route('**/api/v1/items:query', (route) => {
    queried.push(route.request().postDataJSON());
    return route.fallback();
  });
  await page.route('**/api/v1/items:export', (route) => {
    exported.push(route.request().postDataJSON());
    return route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/csv; charset=utf-8', 'Content-Disposition': 'attachment; filename="kitchen.csv"' },
      body: 'id,type,title\n',
    });
  });

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  await page.waitForTimeout(600);

  // No second way: the page menu carries no views.
  await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).click();
  const pageMenu = page.getByRole('menu', { name: `Actions for ${COLLECTION.name}` });
  assert.ok(!(await labelsOf(pageMenu)).some((label) => /view/i.test(label)), `the page menu still names a view: ${await labelsOf(pageMenu)}`);
  await page.keyboard.press('Escape');

  // The one menu, in its order: the views that apply here, then the acts.
  const trigger = page.locator('[data-opener="views"]');
  assert.equal(await trigger.count(), 1, 'no Views menu in the head');
  await trigger.click();
  const menu = page.getByRole('menu', { name: 'Views' });
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual(await labelsOf(menu), ['Show Open work', 'Save this view…', 'Rename, share or delete a view…', 'Export this list…', 'Calendar subscriptions…']);

  // Opening a view applies its query and its layout together.
  const before = queried.length;
  await menu.getByRole('menuitem', { name: 'Show Open work' }).click();
  for (let waited = 0; queried.length === before && waited < 50; waited++) await page.waitForTimeout(100);
  const applied = queried.slice(before).find((body) => body.filter);
  assert.deepEqual(applied?.filter, OPEN_WORK.query.filter, 'the view’s filter was not applied');
  await page.locator('.board').first().waitFor({ timeout: 5_000 }).catch(() => assert.fail('the view’s board layout was not applied'));

  // Export without a saved view: the list as it is filtered now.
  await trigger.click();
  await page.getByRole('menuitem', { name: 'Export this list…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Export this list' });
  await dialog.waitFor({ timeout: 5_000 });
  const download = page.waitForEvent('download', { timeout: 5_000 });
  await dialog.getByRole('button', { name: 'Export' }).click();
  await download;
  assert.equal(exported.length, 1, 'nothing was exported');
  assert.equal(exported[0].format, 'CSV');
  assert.deepEqual(exported[0].query.scope, { container_id: COLLECTION.id, include_descendants: false });
  assert.deepEqual(exported[0].query.filter, OPEN_WORK.query.filter, 'the export left the filter behind');
  await dialog.getByText('kitchen.csv was saved.').waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });

  // Saving, changing and the subscriptions each open their own dialog.
  for (const [row, title] of [['Save this view…', 'Save this view'], ['Rename, share or delete a view…', 'Saved views'], ['Calendar subscriptions…', 'Calendar subscriptions']]) {
    await trigger.click();
    await page.getByRole('menuitem', { name: row }).click();
    const opened = page.getByRole('dialog', { name: title });
    await opened.waitFor({ timeout: 5_000 }).catch(() => assert.fail(`${row} opened no ${title}`));
    await page.keyboard.press('Escape');
    await opened.waitFor({ state: 'hidden', timeout: 5_000 });
  }
  assert.deepEqual(failures, []);
});
