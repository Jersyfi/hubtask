// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What takes effect at once offers *Undo* (UC-WRK-28 check 3): moving an entry to the trash and
// archiving it from its menu, taking a label off an entry, deleting a label from the collection and
// a bulk of either. Each message still stands after eight seconds - the frame's dwell for a message
// that offers something - and *Undo* sends the inverse write. Chromium only; the clock is
// Playwright's, so eight seconds take none.

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

/** The collection's list with every write recorded, and the page's clock in the walk's hand. */
async function list(browser) {
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  await page.clock.install();
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() !== 'GET' && !/(items:query|sync:pull|sync:snapshot|\/search)$/.test(path)) {
      written.push({ method: request.method(), path, body: request.postDataJSON() });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator(`[data-row="${ITEMS[2].id}"]`).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  return { page, context, failures, written, unstubbed };
}

/**
 * Presses *Undo* on the message that says `text`, after it has stood for eight seconds: the clock
 * is paused while the message stands, so the eight seconds are exactly that, then it runs again.
 */
async function undoAfterEightSeconds(page, text) {
  const message = page.locator('.toast', { hasText: text });
  await message.waitFor({ timeout: 5_000 });
  await page.mouse.move(1, 1);
  await page.clock.pauseAt(new Date((await page.evaluate(() => Date.now())) + 50));
  await page.clock.runFor(8_000 - 100);
  assert.equal(await message.count(), 1, `"${text}" left before eight seconds`);
  await message.getByRole('button', { name: 'Undo' }).click();
  await page.clock.resume();
}

test('chromium: 1280 px — the trash and the archive from an entry\'s menu take effect at once and offer Undo', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written, unstubbed } = await list(browser);
  t.after(() => context.close());
  const entry = ITEMS[2];
  const menu = page.getByRole('button', { name: `Actions for ${entry.title}`, exact: true }).first();

  await menu.click();
  await page.getByRole('menuitem', { name: 'Move to the trash' }).click();
  await undoAfterEightSeconds(page, `${entry.title} is in the trash.`);
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'DELETE' && w.path.endsWith(`/items/${entry.id}`)), 'the trash was not at once');
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/items/${entry.id}:restore`)), 'Undo sent no :restore');
  // The confirmation that it was undone leaves on its own, once nothing holds it: a pointer over a
  // message keeps it.
  await page.mouse.move(1, 1);
  await page.locator('main').first().focus().catch(() => {});
  await page.clock.runFor(10_000);
  await page.locator('.toast').first().waitFor({ state: 'detached', timeout: 5_000 });

  await menu.click();
  await page.getByRole('menuitem', { name: 'Archive' }).click();
  await undoAfterEightSeconds(page, `${entry.title} is archived.`);
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/items/${entry.id}:unarchive`)), 'Undo sent no :unarchive');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});
