// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Two controls that were offered and did nothing, or were refused (UC-WRK-07 check 1, UC-WRK-17
// check 7):
//
// * *Duplicate* in an entry page's subtree opens the copy dialog, as it does in the collection.
// * A view saved at hub scope names the hub - the server refuses one naming the collection with
//   `views.scope_container_mismatched`.
//
// Against the shared fixture. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { CHILDREN, COLLECTION, HUB, ITEMS, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

test('chromium: Duplicate in an entry page’s subtree opens the copy of that entry', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());

  await page.goto(`${served.origin}/items/${ITEMS[0].id}`);
  const child = CHILDREN[ITEMS[0].id][0];
  await page.getByRole('button', { name: `Move ${child.title}` }).click();
  await page.getByRole('menuitem', { name: 'Duplicate' }).click();
  await page.getByRole('dialog', { name: `Duplicate ${child.title}` }).waitFor();
  assert.deepEqual(failures, []);
});

test('chromium: a view saved at hub scope names the hub', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280);
  t.after(() => context.close());
  const saved = [];
  await page.route('**/api/v1/views', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    saved.push(route.request().postDataJSON());
    return route.fulfill({ status: 201, json: { id: 'v1', version: 1, ...route.request().postDataJSON() } });
  });

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator('[data-opener="views"]').click();
  const dialog = page.getByRole('dialog', { name: 'Saved views' });
  await dialog.getByLabel('Name').fill('Everything in the house');
  await dialog.getByLabel('Where it lives').selectOption('HUB');
  await dialog.getByRole('button', { name: 'Save what is on screen' }).click();
  for (let waited = 0; saved.length === 0 && waited < 50; waited++) await page.waitForTimeout(100);

  assert.equal(saved.length, 1, 'nothing was saved');
  assert.equal(saved[0].scope_type, 'HUB');
  assert.equal(saved[0].scope_id, HUB.id, 'the view names something other than the hub');
  assert.deepEqual(failures, []);
});
