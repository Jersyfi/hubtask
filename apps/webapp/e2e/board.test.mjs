// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The board's card is the thing you move (F10-11, ADR-0063 decision 11). The gesture always
// worked; what failed was that it began on a 14 × 22 px grip beside the card and the card then
// travelled on one axis only, so carrying one across the board looked like nothing happening.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The board, with every write recorded. */
async function board(browser, { hasTouch = false } = {}) {
  const written = [];
  const { page, failures, context, close } = await signedIn(browser, 1280, 900, { hasTouch });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    if (request.method() !== 'GET') written.push({ method: request.method(), path: new URL(request.url()).pathname, body: request.postDataJSON() });
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('radio', { name: 'Board' }).click();
  await page.locator('[data-card]').first().waitFor({ timeout: 15_000 });
  return { page, failures, written, close };
}

/** A pointer press carried from one point to another, in steps, as a hand would. */
async function carry(page, from, to, { steps = 12, hold = 0 } = {}) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  if (hold > 0) await page.waitForTimeout(hold);
  for (let step = 1; step <= steps; step += 1) {
    await page.mouse.move(from.x + ((to.x - from.x) * step) / steps, from.y + ((to.y - from.y) * step) / steps);
    await page.waitForTimeout(20);
  }
  await page.mouse.up();
}

test('chromium: a card is carried from anywhere on it, and the click that ends the carry opens nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await board(browser);
  t.after(close);

  // No grip anywhere: the card is the handle.
  assert.equal(await page.locator('[data-card] [data-grip]').count(), 0, 'the board still draws a grip');

  const card = page.locator(`[data-card="${ITEMS[0].id}"]`);
  const doing = page.locator(`[data-column-zone="${BUCKETS[1].id}"]`);
  const from = await card.boundingBox();
  const to = await doing.boundingBox();

  // Carried from the middle of the card — not from a grip — into the next column.
  await carry(page, { x: from.x + from.width / 2, y: from.y + from.height / 2 }, { x: to.x + to.width / 2, y: to.y + 80 });
  await page.waitForTimeout(400);

  const move = written.find((each) => each.method === 'PATCH' && each.path.endsWith(`/items/${ITEMS[0].id}`) && 'bucket_id' in (each.body ?? {}));
  assert.equal(move?.body?.bucket_id, BUCKETS[1].id, `the card did not change column: ${JSON.stringify(written)}`);
  // And letting go did not also open the entry it had just filed.
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`, 'the drag ended by opening the entry');
  assert.deepEqual(failures, []);
});

test('chromium: a press that does not travel is still the click it always was', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await board(browser);
  t.after(close);

  await page.locator(`[data-card="${ITEMS[0].id}"] a`).first().click();
  await page.waitForTimeout(300);
  const opened = new URL(page.url());
  // From `large` an entry opens *beside* the list rather than at its own address (ADR-0061
  // decision 4's pane), so either shape is the entry being opened.
  assert.ok(
    opened.pathname === `/items/${ITEMS[0].id}` || opened.searchParams.get('item') === ITEMS[0].id,
    `a plain click no longer opens the entry: ${opened.pathname}${opened.search}`,
  );
  assert.deepEqual(written.filter((each) => each.method === 'PATCH'), [], 'a click wrote something');
  assert.deepEqual(failures, []);
});

test('chromium: a finger that moves at once is scrolling, and one that waits is carrying', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await board(browser, { hasTouch: true });
  t.after(close);

  const card = page.locator(`[data-card="${ITEMS[0].id}"]`);
  const doing = page.locator(`[data-column-zone="${BUCKETS[1].id}"]`);
  const from = await card.boundingBox();
  const to = await doing.boundingBox();
  const middle = { x: from.x + from.width / 2, y: from.y + from.height / 2 };
  const target = { x: to.x + to.width / 2, y: to.y + 80 };

  // Playwright's mouse is a `mouse` pointer whatever the context says, so the hold is asserted
  // through the touch screen, which is what a finger actually is.
  await page.touchscreen.tap(middle.x, middle.y).catch(() => {});
  await page.waitForTimeout(200);

  // Carried with the hold: it lands.
  await carry(page, middle, target, { hold: 400 });
  await page.waitForTimeout(400);
  const move = written.find((each) => each.method === 'PATCH' && 'bucket_id' in (each.body ?? {}));
  assert.ok(move !== undefined, `a held carry did not land: ${JSON.stringify(written)}`);
  assert.deepEqual(failures, []);
});
