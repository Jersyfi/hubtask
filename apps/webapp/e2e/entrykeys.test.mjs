// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One key format everywhere (user-experience.md §5.6): a list row, a board card, the entry's panel
// and the collection's settings read the key one way - the prefix and the number where the collection has
// a prefix, the handle in capitals where it has none - and the settings change the prefix
// (UC-JUM-12 check 1).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, ITEMS, openSettings, signedIn, stub, chooseView } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

async function opened(browser) {
  const written = [];
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    if (request.method() !== 'GET') written.push({ method: request.method(), path: new URL(request.url()).pathname, body: request.postDataJSON() });
    return stub(route);
  });
  return { page, failures, written, close: () => context.close() };
}

test('a row, a card and the panel draw the key with the prefix, or the handle where there is none', async () => {
  const browser = await chromium.launch();
  try {
    for (const [prefix, key] of [[undefined, 'KITCHEN-31'], ['KIT', 'KIT-31']]) {
      COLLECTION.key_prefix = prefix ?? null;
      const { page, failures, close } = await opened(browser);
      await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
      // The key is a field of the list; the board's cards draw what the list's fields say.
      await page.getByRole('button', { name: 'Fields' }).click();
      await page.getByRole('menuitem', { name: 'Key' }).click();
      await page.locator(`[data-field="key"] [data-entry-key]`).getByText(key, { exact: true }).first().waitFor({ timeout: 15_000 });
      await chooseView(page, 'Board');
      const card = page.locator(`[data-card="${ITEMS[0].id}"]`);
      await card.locator('[data-entry-key]').getByText(key, { exact: true }).waitFor({ timeout: 15_000 });
      await page.goto(`${served.origin}/items/${ITEMS[0].id}`);
      await page.getByRole('button', { name: `Copy the key ${key}` }).waitFor({ timeout: 10_000 });
      assert.deepEqual(failures, []);
      await close();
    }
  } finally {
    delete COLLECTION.key_prefix;
    await browser.close();
  }
});

test('the settings change the prefix, typed in any case, and show what the keys will read', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser);
    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    await openSettings(page, COLLECTION.name);
    const keys = page.locator('[data-key-settings]');
    await keys.getByText('Keys read like KITCHEN-42.').waitFor({ timeout: 10_000 });
    await keys.getByLabel('Key prefix').fill('inc');
    await keys.getByText('Keys read like INC-42.').waitFor();
    const sent = page.waitForRequest((request) => request.method() === 'PATCH', { timeout: 10_000 });
    await keys.getByRole('button', { name: 'Change the prefix' }).click();
    const patch = await sent;
    assert.equal(new URL(patch.url()).pathname, `/api/v1/containers/${COLLECTION.id}`);
    assert.deepEqual(patch.postDataJSON(), { key_prefix: 'INC' });
    assert.deepEqual(written.filter((write) => write.method === 'PATCH').length, 1);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
