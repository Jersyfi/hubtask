// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One menu for an entry, wherever the entry is shown (UC-WRK-28 check 1): the list, the subtree on
// an entry's page, the board, the timeline and search offer the same items in the same order, by
// the ⋯, by a right click, by Shift+F10 and by a still long press on touch (ADR-0088) - the board
// adds its columns, and only the list and the board offer *Select*. One item per surface is pressed
// and the request it sends is asserted. Chromium only; the touch path goes through the browser's own
// touch input, as the board's walk does.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ARCHIVED, CHILDREN, COLLECTION, GROCERIES, ITEMS, signedIn, stub } from './fixture.mjs';
import { reveal } from './reveal.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The fixture's task with a due date, which is also on the timeline. */
const DATED = ITEMS.find((each) => each.due_at);

/** What every surface lists for an active task the owner may change, without *Select* and columns. */
const COMMON = [
  'Open', 'Mark as done', 'Change the due date', 'Person', 'Labels',
  'Move up', 'Move down', 'Move to the top', 'Move to the bottom',
  'Move inside', 'Move out one level', 'Move to another collection',
  'Duplicate', 'Split a part off', 'Merge into another', 'Share this entry', 'Copy link', 'Archive', 'Move to the trash',
];

/** The items' own labels, the reason under an unavailable one left out. */
const labelsOf = (menu) =>
  menu.getByRole('menuitem').evaluateAll((items) =>
    items.map((item) => (item.querySelector('.label')?.textContent ?? '').trim().replace(/…$/, '').replace(/^(Move inside).*$/, '$1')));

/** A signed-in tab with every write recorded, at a width and maybe with a touch screen. */
async function open(browser, path, { width = 1280, height = 900, hasTouch = false } = {}) {
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, width, height, { hasTouch });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    // The reads that are a POST - a query, a search, a pull - write nothing.
    if (request.method() !== 'GET' && !/(items:query|sync:pull|sync:snapshot|accounts:resolve|\/search)$/.test(path)) written.push({ method: request.method(), path });
    return stub(route);
  });
  await page.goto(`${served.origin}${path}`);
  return { page, context, failures, written, unstubbed };
}

/** A finger held still on a point and lifted, through the browser's own touch input. */
async function stillHold(page, context, locator) {
  const box = await locator.boundingBox();
  const point = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  const cdp = await context.newCDPSession(page);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [point] });
  await page.waitForTimeout(500);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await cdp.detach();
}

/**
 * Whether a write of `method` to a path ending in `suffix` was sent: waited for up to ten seconds,
 * because a loaded runner can take longer than any fixed pause to send it.
 */
async function sent(written, method, suffix) {
  const seen = () => written.some((w) => w.method === method && w.path.endsWith(suffix));
  for (const deadline = Date.now() + 10_000; !seen() && Date.now() < deadline; ) await new Promise((done) => setTimeout(done, 50));
  return seen();
}

/** Reads the open menu and closes it again. */
async function readMenu(page, title) {
  const menu = page.getByRole('menu', { name: `Actions for ${title}` });
  await menu.waitFor({ timeout: 5_000 });
  const read = await labelsOf(menu);
  const focused = await page.evaluate(() => document.activeElement?.getAttribute('role'));
  await page.keyboard.press('Escape');
  await menu.waitFor({ state: 'detached', timeout: 5_000 });
  return { read, focused };
}

/** The four ways in, each answering the same items. */
async function everyWayIn(page, row, title) {
  const trigger = page.getByRole('button', { name: `Actions for ${title}`, exact: true }).first();
  await reveal(trigger);
  const byButton = await readMenu(page, title);
  await row.click({ button: 'right' });
  const byRightClick = await readMenu(page, title);
  assert.equal(byRightClick.focused, 'menuitem', 'a right click does not land on the first item');
  await row.focus();
  await page.keyboard.press('Shift+F10');
  const byKey = await readMenu(page, title);
  assert.equal(byKey.focused, 'menuitem', 'Shift+F10 does not land on the first item');
  assert.deepEqual(byRightClick.read, byButton.read);
  assert.deepEqual(byKey.read, byButton.read);
  return byButton.read;
}

test('chromium: 1280 px — the list, the board, the timeline, the subtree and search offer one menu', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  // The list.
  const list = await open(browser, `/collections/${COLLECTION.id}`);
  t.after(() => list.context.close());
  const listRow = list.page.locator(`[data-row="${DATED.id}"] a`).first();
  await listRow.waitFor({ timeout: 15_000 });
  await list.page.waitForTimeout(600);
  const onList = await everyWayIn(list.page, listRow, DATED.title);
  assert.deepEqual(onList, ['Select', ...COMMON]);
  // One item, and the request it sends.
  await reveal(list.page.getByRole('button', { name: `Actions for ${DATED.title}`, exact: true }).first());
  await list.page.getByRole('menuitem', { name: 'Archive' }).click();
  assert.ok(await sent(list.written, 'POST', `/items/${DATED.id}:archive`), 'Archive sent no :archive');

  // The board: the same, with the columns after the moves.
  await list.page.getByRole('radio', { name: 'Board' }).click();
  const card = list.page.locator(`[data-card="${DATED.id}"] a`).first();
  await card.waitFor({ timeout: 15_000 });
  const onBoard = await everyWayIn(list.page, card, DATED.title);
  const columns = onBoard.filter((label) => label.startsWith(`Move ${DATED.title} to`));
  assert.ok(columns.length > 0, 'the board offers no columns');
  assert.deepEqual(onBoard.filter((label) => !columns.includes(label)), ['Select', ...COMMON]);
  assert.equal(onBoard.indexOf(columns[0]), onBoard.indexOf('Move to another collection') + 1);
  await reveal(list.page.getByRole('button', { name: `Actions for ${DATED.title}`, exact: true }).first());
  await list.page.getByRole('menuitem', { name: 'Move to the trash' }).click();
  assert.ok(await sent(list.written, 'DELETE', `/items/${DATED.id}`), 'the trash sent no DELETE');

  // The timeline: no Select, and the moves wait for the level read when the menu opens.
  await list.page.getByRole('radio', { name: 'Timeline' }).click();
  const bar = list.page.locator(`.timeline .rows [data-row="${DATED.id}"] button.title`);
  await bar.waitFor({ timeout: 15_000 });
  const onTimeline = await everyWayIn(list.page, bar, DATED.title);
  assert.deepEqual(onTimeline, COMMON);
  await reveal(list.page.getByRole('button', { name: `Actions for ${DATED.title}`, exact: true }).first());
  await list.page.getByRole('menuitem', { name: 'Mark as done' }).click();
  assert.ok(await sent(list.written, 'POST', `/items/${DATED.id}:complete`), 'Mark as done sent no :complete');
  assert.deepEqual(list.failures, []);
  assert.deepEqual(list.unstubbed(), []);

  // The subtree on an entry's page: no Select, nothing to pick there.
  const entry = await open(browser, `/items/${ITEMS[0].id}`);
  t.after(() => entry.context.close());
  const child = CHILDREN[ITEMS[0].id][0];
  const childRow = entry.page.locator(`[data-row="${child.id}"] a`).first();
  await childRow.waitFor({ timeout: 15_000 });
  await entry.page.waitForTimeout(600);
  const inSubtree = await everyWayIn(entry.page, childRow, child.title);
  assert.equal(inSubtree.includes('Select'), false);
  assert.equal(inSubtree[0], 'Open');
  assert.deepEqual(entry.failures, []);

  // Search: the same menu on a hit, once the role along its collection has been read.
  const search = await open(browser, '/search');
  t.after(() => search.context.close());
  await search.page.getByRole('searchbox', { name: 'Search everything you can see' }).fill('electrician');
  const hit = search.page.locator(`[data-hit="${DATED.id}"] a`).first();
  await hit.waitFor({ timeout: 10_000 });
  await search.page.waitForTimeout(600);
  const inSearch = await everyWayIn(search.page, hit, DATED.title);
  assert.deepEqual(inSearch, COMMON);
  await reveal(search.page.getByRole('button', { name: `Actions for ${DATED.title}`, exact: true }).first());
  await search.page.getByRole('menuitem', { name: 'Copy link' }).waitFor();
  assert.deepEqual(search.failures, []);
});

test('chromium: 1280 px — the entry\'s own page carries the same menu in its head, without Open (UC-WRK-28 check 2)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const entry = await open(browser, `/items/${DATED.id}`);
  t.after(() => entry.context.close());
  await entry.page.getByRole('heading', { name: DATED.title }).first().waitFor({ timeout: 15_000 });
  await entry.page.waitForTimeout(600);
  const head = entry.page.locator('[data-opener="entry-menu"]').first();
  await head.click();
  const { read } = await readMenu(entry.page, DATED.title);
  assert.deepEqual(read, COMMON.filter((label) => label !== 'Open'));

  // The level was read when the menu was reached for: the second of five moves up.
  await head.click();
  await entry.page.waitForFunction(() => [...document.querySelectorAll('[role="menuitem"]')].find((each) => each.textContent?.includes('Move up'))?.getAttribute('aria-disabled') !== 'true', null, { timeout: 5_000 })
    .catch(() => assert.fail('Move up stays unavailable on the page'));
  await entry.page.keyboard.press('Escape');

  // What it does is what every surface's menu does: archived at once, with its request.
  await head.click();
  await entry.page.getByRole('menuitem', { name: 'Archive' }).click();
  assert.ok(await sent(entry.written, 'POST', `/items/${DATED.id}:archive`), 'Archive sent no :archive');
  // The trash leaves the page for the collection, where the entry was.
  await entry.page.goto(`${served.origin}/items/${ITEMS[0].id}`);
  await entry.page.getByRole('heading', { name: ITEMS[0].title }).first().waitFor({ timeout: 15_000 });
  await entry.page.waitForTimeout(600);
  await entry.page.locator('[data-opener="entry-menu"]').first().click();
  await entry.page.getByRole('menuitem', { name: 'Move to the trash' }).click();
  await entry.page.waitForURL(`**/collections/${COLLECTION.id}`, { timeout: 5_000 });
  assert.ok(entry.written.some((w) => w.method === 'DELETE' && w.path.endsWith(`/items/${ITEMS[0].id}`)), 'the trash sent no DELETE');
  assert.deepEqual(entry.failures, []);
  assert.deepEqual(entry.unstubbed(), []);
});

test('chromium: 1280 px — a row\'s tools show where the pointer or the keyboard is, its chips always', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const list = await open(browser, `/collections/${COLLECTION.id}`);
  t.after(() => list.context.close());
  const row = list.page.locator(`[data-row="${ITEMS[0].id}"]`);
  await row.waitFor({ timeout: 15_000 });
  await list.page.waitForTimeout(600);
  await list.page.mouse.move(0, 0);
  /**
   * Whether every tool of the row - the grip, the labels, ＋ and ⋯ - is drawn (1) or out of sight
   * (0). Out of sight the grip is faded and the others take no width: clipped, not removed, so
   * they stay in the reading order.
   */
  const waitFor = (wanted) => list.page.waitForFunction(([id, value]) => {
    const element = document.querySelector(`[data-row="${id}"]`);
    const grip = Number(getComputedStyle(element.querySelector('.grip')).opacity) === value;
    // The tools are one group over the row's end: clipped out of sight, or drawn.
    const tools = element.querySelector('.tools');
    const drawn = getComputedStyle(tools).clipPath === 'none' && tools.getBoundingClientRect().width > 4;
    return grip && drawn === (value === 1);
  }, [ITEMS[0].id, wanted], { timeout: 5_000 });
  await waitFor(0);
  assert.equal(await row.locator('.tools .on-hover').count(), 3, 'labels, ＋ and ⋯');
  // The label chip stays: it is what the row says.
  assert.equal(await row.locator('[data-token]').first().evaluate((chip) => Number(getComputedStyle(chip).opacity)), 1);
  // The pointer over the row.
  await row.hover();
  await waitFor(1);
  // The keyboard in the row: the tools come back without a pointer.
  await list.page.mouse.move(0, 0);
  await waitFor(0);
  await row.locator('a').first().focus();
  await waitFor(1);
  assert.deepEqual(list.failures, []);
});

test('chromium: 390 px with a touch screen — a still long press opens the menu, Select first, and Select picks', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, `/collections/${COLLECTION.id}`, { width: 390, hasTouch: true });
  t.after(() => context.close());
  const row = page.locator(`[data-row="${DATED.id}"] .title`).first();
  await row.waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);

  await stillHold(page, context, row);
  const menu = page.getByRole('menu', { name: `Actions for ${DATED.title}` });
  await menu.waitFor({ timeout: 5_000 });
  assert.equal((await labelsOf(menu))[0], 'Select');
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`, 'the lift also opened the entry');
  await page.getByRole('menuitem', { name: 'Select' }).click();
  await page.getByRole('checkbox', { name: `Select ${DATED.title}` }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('checkbox', { name: `Select ${DATED.title}` }).isChecked(), true);

  // On the board, the card's still hold opens its menu too; a hold that moves is the drag's.
  await page.keyboard.press('Escape');
  await page.getByRole('radio', { name: 'Board' }).click();
  // One column at this width: the strip chooses the one this card is in.
  await page.getByRole('radiogroup', { name: 'Which column is shown' }).getByRole('radio', { name: /^To do/u }).click();
  const card = page.locator(`[data-card="${ITEMS[2].id}"]`).first();
  await card.waitFor({ timeout: 15_000 });
  await page.waitForTimeout(300);
  await stillHold(page, context, card.locator('.title'));
  await page.getByRole('menu', { name: `Actions for ${ITEMS[2].title}` }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 375 × 812 — the menu of the last row on screen stays on screen, every item reachable', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, `/collections/${COLLECTION.id}`, { width: 375, height: 812 });
  t.after(() => context.close());
  await page.locator('[data-row] a').first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);

  // The last ⋯ wholly on screen: the row nearest the foot, where the room below is the least.
  const triggers = page.getByRole('button', { name: /^Actions for / });
  const boxes = await triggers.evaluateAll((all) => all.map((each) => each.getBoundingClientRect().toJSON()));
  // On screen means above the bottom bar, which stands over the page's foot.
  const foot = await page.evaluate(() => document.querySelector('nav[aria-label="Sections"]')?.getBoundingClientRect().top ?? innerHeight);
  const last = boxes.findLastIndex((box) => box.height > 0 && box.bottom <= foot);
  assert.ok(last >= 0, 'no row with a ⋯ on screen');
  assert.ok(boxes[last].top > foot / 2, `the last row on screen is in the upper half (${boxes[last].top})`);
  const scrolled = await page.evaluate(() => scrollY);
  // Below `medium` a row's tools are always drawn, so the ⋯ is pressed where it stands.
  await triggers.nth(last).click();
  const menu = page.getByRole('menu');
  await menu.waitFor({ timeout: 5_000 });
  await page.waitForTimeout(300);

  const surface = await menu.evaluate((list) => list.parentElement.getBoundingClientRect().toJSON());
  assert.ok(surface.top >= 0 && surface.bottom <= 812, `the menu runs past the viewport: ${surface.top}–${surface.bottom}`);
  assert.ok(surface.bottom <= boxes[last].top, 'the menu did not open above its ⋯, where the room is larger');
  // Every item comes into view inside the menu, and the page does not move for it.
  const items = menu.getByRole('menuitem');
  const count = await items.count();
  assert.ok(count >= COMMON.length, `the menu has ${count} items`);
  for (let index = 0; index < count; index += 1) {
    // The surface measured with the item: the engine may settle the scrolled surface anew.
    const { box, around } = await items.nth(index).evaluate((item) => {
      item.scrollIntoView({ block: 'nearest' });
      return { box: item.getBoundingClientRect().toJSON(), around: item.closest('[role="menu"]').parentElement.getBoundingClientRect().toJSON() };
    });
    assert.ok(around.top >= 0 && around.bottom <= 812, `the menu runs past the viewport: ${around.top}–${around.bottom}`);
    assert.ok(box.top >= around.top - 1 && box.bottom <= around.bottom + 1, `item ${index} is outside the menu: ${box.top}–${box.bottom}`);
  }
  assert.equal(await page.evaluate(() => scrollY), scrolled, 'reaching the items scrolled the page');
  assert.deepEqual(failures, []);
});

// UC-WRK-28 check 4: Ctrl or ⌘-click selects in the list and on the board; on touch, the still
// hold's Select does (the 390 px test above).
test('chromium: 1280 px — Ctrl or ⌘-click picks a card on the board, as it picks a row in the list', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, `/collections/${COLLECTION.id}`);
  t.after(() => context.close());
  const row = page.locator(`[data-row="${DATED.id}"] .title`).first();
  await row.waitFor({ timeout: 15_000 });
  await row.click({ modifiers: ['ControlOrMeta'] });
  const picked = page.getByRole('checkbox', { name: `Select ${DATED.title}` });
  await picked.waitFor({ timeout: 5_000 });
  assert.equal(await picked.isChecked(), true);
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`, 'the click also followed the row');
  await picked.click();
  await page.getByRole('radio', { name: 'Board' }).click();
  const card = page.locator(`[data-card="${ITEMS[2].id}"] a`).first();
  await card.waitFor({ timeout: 15_000 });
  await card.click({ modifiers: ['ControlOrMeta'] });
  const box = page.getByRole('checkbox', { name: `Select ${ITEMS[2].title}` });
  await box.waitFor({ timeout: 5_000 });
  assert.equal(await box.isChecked(), true);
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`, 'the click also followed the card');
  // Switching to the list keeps what was picked (design-system.md §11.9).
  await page.getByRole('radio', { name: 'List' }).first().click();
  await page.getByRole('checkbox', { name: `Select ${ITEMS[2].title}` }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('checkbox', { name: `Select ${ITEMS[2].title}` }).isChecked(), true);
  assert.deepEqual(failures, []);
});

test('chromium: the pane opens from a card and a timeline row at 1280 px, and the page below large', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const [width, beside] of [[1280, true], [1024, false]]) {
    const { page, context, failures } = await open(browser, `/collections/${COLLECTION.id}`, { width });
    t.after(() => context.close());
    await page.getByRole('radio', { name: 'Board' }).click();
    const card = page.locator(`[data-card="${DATED.id}"] a`).first();
    await card.waitFor({ timeout: 15_000 });
    await card.click();
    await page.waitForFunction(
      ([id, isBeside]) => (isBeside ? new URLSearchParams(location.search).get('item') === id : location.pathname === `/items/${id}`),
      [DATED.id, beside],
      { timeout: 5_000 },
    );
    if (beside) {
      assert.equal(await page.locator(`[data-card="${DATED.id}"] a[aria-current="true"]`).count(), 1, 'the open card is not current');
      await page.getByRole('radio', { name: 'Timeline' }).click();
      const row = page.locator(`.timeline .rows [data-row="${DATED.id}"] button.title`);
      await row.waitFor({ timeout: 15_000 });
      assert.equal(await row.getAttribute('aria-current'), 'true', 'the open entry is not current on the timeline');
    }
    assert.deepEqual(failures, []);
  }
});

test('chromium: 1280 px — Move to another collection and Duplicate ask first and write nothing until confirmed', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await open(browser, `/collections/${COLLECTION.id}`);
  t.after(() => context.close());
  const trigger = page.getByRole('button', { name: `Actions for ${ITEMS[2].title}`, exact: true }).first();
  await trigger.waitFor({ timeout: 15_000 });
  for (const item of ['Move to another collection', 'Duplicate']) {
    await reveal(trigger);
    await page.getByRole('menuitem', { name: item }).click();
    await page.getByRole('dialog').waitFor({ timeout: 5_000 });
    assert.deepEqual(written.filter((w) => w.method !== 'GET'), [], `${item} wrote before it was confirmed`);
    await page.keyboard.press('Escape');
    await page.getByRole('dialog').waitFor({ state: 'detached', timeout: 5_000 });
  }
  assert.deepEqual(failures, []);
});

// UC-WRK-06 checks 5 and 6: *Move to another collection* offers the collections the reader can
// see that are neither archived nor the current one, and what the destination could not keep is
// listed by kind once the move is answered.
test('chromium: 1280 px — a move elsewhere offers the other open collections and lists what it left behind by kind', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, `/collections/${COLLECTION.id}`);
  t.after(() => context.close());
  const entry = ITEMS[2];
  // The hub's collections, an archived one among them as a listing that includes archived ones
  // answers it: the picker leaves it out itself.
  await page.route(new RegExp(`/api/v1/containers\\?.*parent_id=${COLLECTION.parent_id}`), (route) => route.fulfill({
    json: { data: [COLLECTION, GROCERIES, { ...ARCHIVED, effective_archived: true }], page: { next_cursor: null, has_more: false } },
  }));
  const moves = [];
  await page.route(new RegExp(`/api/v1/items/${entry.id}:move$`), (route) => moves.push(route.request().postDataJSON()) && route.fulfill({
    json: {
      item: { ...entry, collection_id: GROCERIES.id, version: entry.version + 1 },
      dropped_references: [
        { item_id: entry.id, kind: 'LABEL', id: '01a0e2e0-0000-7000-8000-0000000009a1', code: 'labels.not_in_collection' },
        { item_id: entry.id, kind: 'BUCKET', id: '01a0e2e0-0000-7000-8000-0000000009a2', code: 'buckets.not_in_collection' },
      ],
    },
  }));
  const trigger = page.getByRole('button', { name: `Actions for ${entry.title}`, exact: true }).first();
  await trigger.waitFor({ timeout: 15_000 });
  await reveal(trigger);
  await page.getByRole('menuitem', { name: 'Move to another collection' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor({ timeout: 5_000 });
  await dialog.locator("select option").nth(1).waitFor({ state: "attached", timeout: 10_000 }).catch(async () => assert.fail(`the dialog reads ${(await dialog.innerHTML()).slice(0, 1500)}`));
  const offered = (await dialog.locator('select option').allTextContents()).map((each) => each.trim());
  assert.ok(offered.some((each) => each.endsWith(GROCERIES.name)), `the open collection is not offered: ${offered}`);
  assert.ok(!offered.some((each) => each.endsWith(COLLECTION.name)), `the current collection is offered: ${offered}`);
  assert.ok(!offered.some((each) => each.endsWith(ARCHIVED.name)), `an archived collection is offered: ${offered}`);
  await dialog.locator('select').selectOption({ label: offered.find((each) => each.endsWith(GROCERIES.name)) });
  await dialog.getByRole('button', { name: 'Move', exact: true }).click();
  const left = page.getByText(`Moving ${entry.title} left some things behind:`);
  await left.waitFor({ timeout: 5_000 });
  const listed = (await page.locator('.dropped li strong').allTextContents()).map((each) => each.trim());
  assert.deepEqual(listed, ['Label', 'Column']);
  assert.equal(moves.length, 1, 'not one move was sent');
  assert.equal(moves[0].target_collection_id, GROCERIES.id);
  assert.equal(moves[0].target_parent_id, null, "the entry does not arrive at the top level");
  assert.deepEqual(failures, []);
});

// UC-WRK-07 checks 2 and 5: *Duplicate* offers a title (the original's when left empty), the
// subtree, and the collections of the same hub; what the destination cannot hold is listed by kind.
test('chromium: 1280 px — Duplicate offers a title, the subtree and the hub\'s collections, and lists what it left behind by kind', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await open(browser, `/collections/${COLLECTION.id}`);
  t.after(() => context.close());
  const entry = ITEMS[2];
  await page.route(new RegExp(`/api/v1/containers\\?.*parent_id=${COLLECTION.parent_id}`), (route) => route.fulfill({
    json: { data: [COLLECTION, GROCERIES], page: { next_cursor: null, has_more: false } },
  }));
  const copies = [];
  await page.route(new RegExp(`/api/v1/items/${entry.id}:duplicate$`), (route) => copies.push(route.request().postDataJSON()) && route.fulfill({
    status: 201,
    json: {
      item: { ...entry, id: '01a0e2e0-0000-7000-8000-0000000009b1', collection_id: GROCERIES.id, version: 1 },
      copied: 1,
      dropped_references: [
        { item_id: entry.id, kind: 'LABEL', id: '01a0e2e0-0000-7000-8000-0000000009a1', code: 'labels.not_in_collection' },
        { item_id: entry.id, kind: 'ASSIGNEE', id: '01a0e2e0-0000-7000-8000-0000000009a3', code: 'items.assignee_cannot_see_item' },
      ],
    },
  }));
  const trigger = page.getByRole('button', { name: `Actions for ${entry.title}`, exact: true }).first();
  await trigger.waitFor({ timeout: 15_000 });
  await reveal(trigger);
  await page.getByRole('menuitem', { name: 'Duplicate' }).click();
  const dialog = page.getByRole('dialog', { name: `Duplicate ${entry.title}` });
  await dialog.waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByRole('textbox', { name: 'Title of the copy' }).inputValue(), '');
  await dialog.getByText('The original’s is kept when this is left empty.').waitFor();
  assert.equal(await dialog.getByRole('checkbox', { name: 'Copy everything under it too' }).isChecked(), false);
  const select = dialog.getByRole('combobox', { name: 'Into which collection' });
  await select.locator('option').nth(2).waitFor({ state: 'attached', timeout: 10_000 });
  const offered = (await select.locator('option').allTextContents()).map((each) => each.trim());
  assert.deepEqual(offered, ['Beside the original', COLLECTION.name, GROCERIES.name]);
  await dialog.getByRole('checkbox', { name: 'Copy everything under it too' }).check();
  await select.selectOption(GROCERIES.id);
  await dialog.getByRole('button', { name: 'Duplicate', exact: true }).click();
  await dialog.getByText(`The copy of ${entry.title} could not carry these over:`).waitFor({ timeout: 5_000 });
  assert.deepEqual((await dialog.locator('.dropped li strong').allTextContents()).map((each) => each.trim()), ['Label', 'Assignee']);
  assert.deepEqual(copies, [{ include_subtree: true, target_collection_id: GROCERIES.id, target_parent_id: null }]);
  assert.deepEqual(failures, []);
});
