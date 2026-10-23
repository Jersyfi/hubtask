// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The layout a person chooses for a collection is kept for them on this device (UC-WRK-28 check 6,
// design-system.md §11.10): chosen, reloaded, still there; another collection keeps its own; a new
// device - a fresh browser context - starts at the collection's default. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ARCHIVED, COLLECTION, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const chosen = (page) => page.locator('[role="radiogroup"] [role="radio"][aria-checked="true"]').first().textContent();

test('chromium: 1280 px — a chosen layout survives a reload, per collection, on this device only', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('radio', { name: 'Board' }).click();
  await page.locator('[data-card]').first().waitFor({ timeout: 15_000 });

  await page.reload();
  await page.locator('[data-card]').first().waitFor({ timeout: 15_000 });
  assert.equal((await chosen(page)).trim(), 'Board', 'the board was not kept across a reload');

  await page.goto(`${served.origin}/collections/${ARCHIVED.id}`);
  await page.getByRole('radio', { name: 'Board' }).waitFor({ timeout: 15_000 });
  assert.notEqual((await chosen(page)).trim(), 'Board', 'another collection took this one\'s layout');
  assert.deepEqual(failures, []);

  // Another device: nothing kept there.
  const elsewhere = await signedIn(browser, 1280, 900);
  t.after(() => elsewhere.context.close());
  await elsewhere.page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await elsewhere.page.getByRole('radio', { name: 'Board' }).waitFor({ timeout: 15_000 });
  assert.notEqual((await chosen(elsewhere.page)).trim(), 'Board', 'the choice travelled to another device');
});
