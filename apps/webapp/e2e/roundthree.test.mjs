// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The owner's third round of findings (2026-10-25), each where it was found:
//
// * the bar's search opens centred on the window and grows to both sides without moving down; below
//   `expanded` it is a way into the search over the page, never under the logo;
// * a collection's head has its menu and no ⚙ beside it;
// * *Position…* moves any place of the level, not only the one it was opened from;
// * a list draws the twist only on an entry with parts, and grouped by column it offers one add;
// * a click anywhere on a board card opens it;
// * an entry's column is changed from the entry.
//
// Chromium, large unless a width is the point.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { BUCKETS, COLLECTION, GROCERIES, HUB, ITEMS, MANIFEST, PAGE, signedIn, stub, chooseView } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

async function open(width, path, { answer } = {}) {
  const browser = await chromium.launch();
  const { context, page, failures } = await signedIn(browser, width, 900);
  const written = [];
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const read = url.pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() !== 'GET') written.push({ call: `${request.method()} ${read}`, body: request.postDataJSON?.() ?? null });
    const own = answer?.(route, request, read);
    if (own) return own;
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  await page.getByRole('main').waitFor();
  await page.waitForTimeout(700);
  return { page, failures, written, close: async () => { await context.close(); await browser.close(); } };
}

test('chromium: the bar search opens on the window\'s middle, grows to both sides and stays at its height', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, `/collections/${COLLECTION.id}`);
  try {
    const float = page.locator('form.entry > .float');
    const closed = await float.boundingBox();
    await page.getByRole('combobox', { name: /Search/ }).first().click();
    await page.waitForTimeout(450);
    const opened = await float.boundingBox();
    assert.ok(opened.width > closed.width + 100, `it grew: ${closed.width} → ${opened.width}`);
    assert.ok(Math.abs(opened.x + opened.width / 2 - 640) <= 1, `centred on the window: ${opened.x + opened.width / 2}`);
    assert.ok(Math.abs(opened.y - closed.y) <= 0.5, `it stayed at its height: ${closed.y} → ${opened.y}`);
  } finally {
    await close();
  }
});

for (const width of [760, 620]) {
  test(`chromium: ${width} px — the bar holds a way into the search that never runs under the logo`, { timeout: 60_000 }, async () => {
    const { page, close } = await open(width, `/collections/${COLLECTION.id}`);
    try {
      const logo = await page.getByRole('link', { name: 'Hubtask' }).boundingBox();
      const seek = page.locator('button.seek');
      const way = await seek.boundingBox();
      assert.ok(way.x >= logo.x + logo.width, `the way starts at ${way.x}, the logo ends at ${logo.x + logo.width}`);
      await seek.click();
      await page.getByRole('dialog').getByRole('combobox').first().waitFor({ timeout: 5_000 });
    } finally {
      await close();
    }
  });
}

test('chromium: no ⚙ beside the collection\'s menu, and *Position…* moves any collection of the hub', { timeout: 60_000 }, async () => {
  const { page, written, close } = await open(1280, `/collections/${COLLECTION.id}`, {
    answer: (route, request, read) =>
      read === '/containers' && new URL(request.url()).searchParams.get('parent_id') === HUB.id
        ? route.fulfill({ json: { ...PAGE, data: [COLLECTION, GROCERIES] } })
        : undefined,
  });
  try {
    assert.equal(await page.locator('[data-opener="structure"]').count(), 0);
    await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).first().click();
    await page.getByRole('menuitem', { name: /position/i }).click();
    const order = page.locator('[data-position-order]');
    await order.waitFor({ timeout: 5_000 });
    const rows = await order.locator('.name').allInnerTexts();
    assert.ok(rows.length >= 2, `the level: ${rows}`);
    // The other collection, not the one the dialog came from.
    const other = rows.findIndex((row) => !row.includes(COLLECTION.name));
    const name = rows[other].trim();
    const step = other === 0 ? `Move ${name} down` : `Move ${name} up`;
    await order.getByRole('button', { name: step }).click();
    await page.waitForTimeout(400);
    assert.ok(written.some((each) => /\/containers\/[^/]+:move$/.test(each.call) && !each.call.includes(COLLECTION.id)), `moved: ${written.map((each) => each.call)}`);
  } finally {
    await close();
  }
});

test('chromium: the twist only on an entry with parts, and a list grouped by column adds at each column\'s end, not again at its foot', { timeout: 60_000 }, async () => {
  const counted = ITEMS.map((item) => ({ ...item, counts: item.counts ?? { parts: 0, parts_done: 0, comments: 0, attachments: 0 } }));
  const { page, close } = await open(1280, `/collections/${COLLECTION.id}`, {
    answer: (route, request, read) => {
      if (read !== '/items:query') return undefined;
      const body = request.postDataJSON();
      if (body.group_by || body.scope?.item_id || body.scope?.container_id !== COLLECTION.id) return undefined;
      return route.fulfill({ json: { data: counted, groups: [], page: { next_cursor: null, has_more: false }, total: counted.length } });
    },
  });
  try {
    await page.locator(`[data-row="${ITEMS[0].id}"]`).waitFor({ timeout: 10_000 });
    assert.equal(await page.locator(`[data-row="${ITEMS[0].id}"]`).getByRole('button', { name: /Show what is inside/ }).count(), 1, 'the entry with parts has its twist');
    assert.equal(await page.locator(`[data-row="${ITEMS[2].id}"]`).getByRole('button', { name: /Show what is inside/ }).count(), 0, 'an entry without parts has none');

    // Grouped by column in the one panel *View* (E-21), which replaced the Group menu.
    await page.locator('[data-opener="view"]').click();
    await page.locator('[data-view-panel] [data-grouping="column"]').click();
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);
    // Each column's group ends in its own add, said with the column (L-2, L-6); no second add at
    // the list's foot.
    assert.deepEqual(
      (await page.locator('[data-opener="add-entry"]').allTextContents()).map((each) => each.trim()),
      ['Entry in “To do”', 'Entry in “Doing”', 'Entry in “Done”'],
      'one add per column, at its end',
    );
    assert.equal(await page.getByRole('button', { name: 'Add an entry' }).count(), 0, 'a second add at the foot');
  } finally {
    await close();
  }
});

test('chromium: a click anywhere on a board card opens it', { timeout: 60_000 }, async () => {
  const { page, close } = await open(1280, `/collections/${COLLECTION.id}`);
  try {
    await chooseView(page, 'Board');
    const card = page.locator(`[data-card="${ITEMS[2].id}"]`);
    await card.waitFor({ timeout: 15_000 });
    const box = await card.boundingBox();
    // The card's padding above its title: its own surface, none of its controls.
    const hit = await page.evaluate(([x, y]) => document.elementFromPoint(x, y)?.closest('a, button') === null, [box.x + box.width / 2, box.y + 4]);
    assert.ok(hit, 'the point is the card, not one of its controls');
    await page.mouse.click(box.x + box.width / 2, box.y + 4);
    await page.waitForTimeout(600);
    assert.ok(page.url().includes(ITEMS[2].id), `after the click: ${page.url()}`);
  } finally {
    await close();
  }
});

// The status button is the entry's one control for its column (UC-FLW-06/5): the board's columns
// are its states, and no second column menu stands beside it.
test('chromium: the entry\'s column is changed from the entry', { timeout: 60_000 }, async () => {
  const columns = BUCKETS.map((bucket) => ({ id: bucket.id, name: bucket.name, completes: bucket.is_done_bucket }));
  // The server answers an entry's steps only where the workflows module is on (`module.off`
  // otherwise), so the board is such a place.
  const modulesOn = [...(MANIFEST.modules_on ?? []), 'WORKFLOWS'];
  const answer = (route, request, read) => {
    if (read === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: modulesOn } });
    if (/^\/containers\/[^/]+\/modules$/.test(read)) return route.fulfill({ json: { modules_on: modulesOn } });
    if (read === `/items/${ITEMS[2].id}/steps`) {
      return route.fulfill({ json: { state_mode: 'COLUMNS', columns, column_id: BUCKETS[0].id, workflow: null, status: null, steps: [], blocked: [], done_route: [], by_default: true, may_design: false } });
    }
    return null;
  };
  const { page, written, close } = await open(1280, `/items/${ITEMS[2].id}`, { answer });
  try {
    const status = page.locator('[data-steps]');
    await status.waitFor({ timeout: 10_000 });
    assert.equal(await page.locator('[data-column-mark]').count(), 0, 'no second control sets the column');
    await status.getByRole('button', { name: /Change status/ }).click();
    await page.getByRole('menuitemradio', { name: new RegExp(`^${BUCKETS[1].name}`) }).click();
    await page.waitForTimeout(400);
    const moved = written.find((each) => each.call === `POST /items/${ITEMS[2].id}:drop`);
    assert.equal(moved?.body?.bucket_id, BUCKETS[1].id);
  } finally {
    await close();
  }
});
