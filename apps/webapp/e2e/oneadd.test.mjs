// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One add per screen on a phone (design-system.md §11.8). At 390 px a list of one level adds
// through its capture line at the top, so its head pins no *Add an entry* over the rows and the
// add's other ways are in the page menu; a collection of several levels pins its one *Add an
// entry* above the bottom bar, draws no second one under the rows, and ends far enough down that
// its last row scrolls out from under that button. At 1280 px nothing of this changes: the head
// carries *Add an entry*, and a collection keeps its add under the rows.
//
// Chromium only: what is walked is layout, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, GROCERIES, resetGroceries, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
const browser = await chromium.launch();
test.after(async () => {
  await browser.close();
  await served.close();
});

async function open(width, path) {
  resetGroceries();
  const session = await signedIn(browser, width, width < 600 ? 844 : 900);
  await session.page.goto(`${served.origin}${path}`);
  await session.page.locator('[data-row]').first().waitFor();
  return session;
}

const adds = (page) => page.locator('main').getByRole('button', { name: 'Add an entry', exact: true });

test('390 px: a list adds through its capture line, and the head pins no second add', async () => {
  const { page, context, failures } = await open(390, `/collections/${GROCERIES.id}`);
  await page.getByRole('textbox', { name: 'New entries, one per line' }).waitFor();
  assert.equal(await adds(page).count(), 0);
  // The add's other ways stay, in the page menu the bar carries.
  await page.locator('[data-bar="menu"] button').click();
  await page.getByRole('menuitem', { name: 'From a template…' }).waitFor();
  assert.deepEqual(failures, []);
  await context.close();
});

test('390 px: a collection pins one add, and its last row scrolls clear of it', async () => {
  const { page, context, failures } = await open(390, `/collections/${COLLECTION.id}`);
  await adds(page).first().waitFor();
  assert.equal(await adds(page).count(), 1);
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
  await page.waitForTimeout(100);
  const pinned = await adds(page).boundingBox();
  const last = await page.locator('[data-row]').last().boundingBox();
  assert.ok(last.y + last.height <= pinned.y, `the last row ends at ${last.y + last.height}, under the add at ${pinned.y}`);
  assert.deepEqual(failures, []);
  await context.close();
});

test('1280 px: the head carries the add on a list and a collection, and a collection keeps its add under the rows', async () => {
  const list = await open(1280, `/collections/${GROCERIES.id}`);
  assert.equal(await adds(list.page).count(), 1);
  assert.deepEqual(list.failures, []);
  await list.context.close();

  const collection = await open(1280, `/collections/${COLLECTION.id}`);
  assert.equal(await adds(collection.page).count(), 2);
  assert.deepEqual(collection.failures, []);
  await collection.context.close();
});
