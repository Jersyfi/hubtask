// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An entry carried from a list onto a collection in the sidebar (UC-WRK-06 check 8, the navigation
// concept §5.1): within the hub it moves at once, the toast offers Undo, and Undo carries it back.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, GROCERIES, ITEMS, signedIn } from './fixture.mjs';

const served = await serve(join(dirname(fileURLToPath(import.meta.url)), '..', 'dist'));
test.after(() => served.close());

test('UC-WRK-06 check 8: an entry dragged from the list onto a collection of the sidebar moves there, with Undo', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1440, 900);
  t.after(() => context.close());
  const entry = ITEMS[0];
  // The hub's two collections, as its listing answers them.
  await page.route(new RegExp(`/api/v1/containers\\?.*parent_id=${COLLECTION.parent_id}`), (route) => route.fulfill({
    json: { data: [COLLECTION, GROCERIES], page: { next_cursor: null, has_more: false } },
  }));
  const moves = [];
  await page.route(new RegExp(`/api/v1/items/${entry.id}:move$`), (route) => {
    const body = route.request().postDataJSON();
    moves.push(body);
    return route.fulfill({ json: { item: { ...entry, collection_id: body.target_collection_id, version: entry.version + moves.length }, dropped_references: [] } });
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  const row = page.locator(`[data-row="${entry.id}"]`);
  await row.waitFor({ timeout: 15_000 });
  const target = page.locator('aside.sidenav [data-entry-target] [data-id]', { hasText: GROCERIES.name }).first();
  await target.waitFor({ timeout: 10_000 });

  await row.hover();
  const grip = await row.locator('[data-grip]').boundingBox();
  const onto = await target.boundingBox();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2);
  await page.mouse.down();
  await page.mouse.move(grip.x + grip.width / 2, grip.y + grip.height / 2 + 8, { steps: 3 });
  await page.mouse.move(onto.x + onto.width / 2, onto.y + onto.height / 2, { steps: 12 });
  // Before letting go the collection is outlined as the place the entry would go.
  await page.waitForFunction((name) => [...document.querySelectorAll('aside.sidenav [data-entry-target] [data-id]')].some((each) => each.textContent.includes(name) && each.matches('[data-drop="inside"]')), GROCERIES.name, { timeout: 5_000 })
    .catch(() => assert.fail('the collection is not outlined while the entry is over it'));
  await page.mouse.up();

  await page.getByRole('button', { name: 'Undo' }).waitFor({ timeout: 5_000 });
  assert.equal(moves.length, 1, JSON.stringify(moves));
  assert.equal(moves[0].target_collection_id, GROCERIES.id, JSON.stringify(moves[0]));
  assert.ok((await page.locator('[role="status"]').allInnerTexts()).some((each) => each.includes(`${entry.title} moved to ${GROCERIES.name}.`)));
  await page.getByRole('button', { name: 'Undo' }).click();
  await page.waitForFunction(() => document.body.innerText.includes('Undone.'), undefined, { timeout: 5_000 });
  assert.equal(moves.length, 2);
  assert.equal(moves[1].target_collection_id, COLLECTION.id);
  assert.deepEqual(failures, []);
});
