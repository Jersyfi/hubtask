// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one panel *View* of a collection (work-surface concept §6.2, E-21, scene viewcfg): the
// layout, what is seen as the Hubtask query line, the order, the fields with a custom one, the
// board's columns, lanes and limits, the calendar's date and saving - a side pane beside the list
// on a desktop, a sheet of pages on a phone. Proves UC-WRK-17 checks 13 and 14, UC-WRK-05 check 12 and UC-WRK-43
// check 6. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, ITEMS, signedIn } from './fixture.mjs';

const served = await serve(join(dirname(fileURLToPath(import.meta.url)), '..', 'dist'));
test.after(() => served.close());

/** A custom field that groups: the room an entry is in, on three of the collection's entries. */
const ROOM = { id: '01a0e2e0-0000-7000-8000-0000000017b1', collection_id: COLLECTION.id, key: 'room', name: 'Room', kind: 'SELECT', options: ['Kitchen', 'Bath'], is_required: false, applies_to: ['TASK'], version: 1 };
const ROOMS = { [ITEMS[0].id]: 'Kitchen', [ITEMS[1].id]: 'Bath', [ITEMS[2].id]: 'Kitchen' };
const roomedItems = ITEMS.map((item) => (ROOMS[item.id] ? { ...item, custom_fields: { ...(item.custom_fields ?? {}), room: ROOMS[item.id] } } : item));
const PAGE = { next_cursor: null, has_more: false };

/** The collection with its room field, recording what is asked and what is saved. */
async function open(browser, width, height, { isMobile = false, longTitle } = {}) {
  const { context, page, failures } = await signedIn(browser, width, height, { hasTouch: width < 600, isMobile });
  const roomed = longTitle ? roomedItems.map((item, n) => (n === 0 ? { ...item, title: longTitle } : item)) : roomedItems;
  const queried = [];
  const saved = [];
  await page.route('**/api/v1/custom-fields?*', (route) => route.fulfill({ json: [ROOM] }));
  await page.route('**/api/v1/items:query', (route) => {
    const body = route.request().postDataJSON();
    if (body?.scope?.container_id !== COLLECTION.id) return route.fallback();
    queried.push(body);
    if (body.group_by) {
      const groups = [...BUCKETS.map((bucket) => bucket.id), null].map((key) => {
        const data = roomed.filter((item) => (item.bucket_id ?? null) === key);
        return { key, count: data.length, data, page: PAGE };
      });
      return route.fulfill({ json: { data: [], groups, page: PAGE, total: roomed.length } });
    }
    return route.fulfill({ json: { data: roomed, groups: [], page: PAGE, total: roomed.length } });
  });
  await page.route('**/api/v1/views', (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    const body = route.request().postDataJSON();
    saved.push(body);
    return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000017b2', version: 1, owner_id: '01a0e2e0-0000-7000-8000-000000000001', ...body } });
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator('[data-row]').first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(400);
  return { context, page, failures, queried, saved };
}

const panelOf = (page) => page.locator('[data-view-panel]');
const viewButton = (page) => page.locator('[data-opener="view"]');
const until = async (page, condition) => {
  for (let waited = 0; !condition() && waited < 50; waited++) await page.waitForTimeout(100);
};

test('chromium 1280: one panel holds layout, the query line, order, fields, board and saving; the changed tab carries a dot (UC-WRK-17 check 13, UC-WRK-43 check 6)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, queried, saved } = await open(browser, 1280, 900);
  t.after(() => context.close());

  // No separate filter, sort, fields or views menu: one button, *View*.
  for (const retired of ['Filter', 'Sort', 'Fields', 'Group', 'Views']) {
    assert.equal(await page.getByRole('main').getByRole('button', { name: retired, exact: true }).count(), 0, `${retired} still stands beside the panel`);
  }
  await viewButton(page).click();
  const panel = panelOf(page);
  await panel.waitFor({ timeout: 5_000 });

  // Layout: five cards - list, outline, board, timeline and the calendar marked new (§6.2). The
  // outline card and the list's *Show what is inside* are one switch: each says what the other did.
  assert.deepEqual(await panel.locator('[data-layout]').evaluateAll((cards) => cards.map((card) => card.dataset.layout)), ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE', 'CALENDAR']);
  assert.match(await panel.locator('[data-layout="CALENDAR"]').innerText(), /new/u);
  const inside = page.getByRole('button', { name: 'Show what is inside', exact: true });
  assert.equal(await inside.getAttribute('aria-pressed'), 'false');
  await panel.locator('[data-layout="LIST_EXPANDED"]').click();
  await page.waitForFunction(() => document.querySelector('[data-layout="LIST_EXPANDED"]')?.getAttribute('aria-pressed') === 'true');
  assert.equal(await inside.getAttribute('aria-pressed'), 'true', 'the outline card and the toggle disagree');
  await panel.locator('[data-layout="LIST_COLLAPSED"]').click();
  await page.waitForFunction(() => document.querySelector('[data-layout="LIST_COLLAPSED"]')?.getAttribute('aria-pressed') === 'true');
  assert.equal(await inside.getAttribute('aria-pressed'), 'false');

  // What is seen: the line, one row of chips; the count while typing; the same query as text.
  const line = panel.getByRole('combobox', { name: 'What is seen' });
  await line.click();
  await page.keyboard.type('is: open ');
  await panel.locator('.chip[data-kind="chip"]').first().waitFor({ timeout: 5_000 });
  await panel.locator('[data-view-count]').waitFor({ timeout: 5_000 });
  assert.match(await panel.locator('[data-view-count]').innerText(), /^\d+ entries$/u);
  assert.equal((await panel.locator('[data-view-line]').innerText()).trim(), 'is: open');
  const asked = () => queried.some((body) => JSON.stringify(body.filter ?? {}).includes('is_completed'));
  await until(page, asked);
  assert.ok(asked(), 'the line was not asked of the list');

  // A word that names no field is searched as words, said at its chip with the field it may have
  // meant as the fix (UC-WRK-43 check 6): a refused term carries the problem and its suggestion.
  await page.keyboard.type('titel: tiles ');
  const problem = panel.locator('.problems li').first();
  await problem.waitFor({ timeout: 5_000 });
  assert.match(await problem.innerText(), /names no field, so it is searched as words/u);
  assert.equal(await panel.locator('.chip[data-problem]').count(), 1, 'the chip carries no mark of its problem');
  await problem.getByRole('button', { name: 'Use title:' }).click();
  await panel.locator('.problems').waitFor({ state: 'detached', timeout: 5_000 });
  assert.match(await panel.locator('[data-view-line]').innerText(), /title: tiles/u);
  // Each piece stands over the list as a chip of its own.
  assert.equal(await page.locator('.query-chips li').count(), 2);

  // Order, then the fields with the custom one.
  await panel.getByLabel('Sort by').selectOption('title');
  await panel.getByRole('group', { name: 'Direction' }).getByRole('button', { name: 'Descending' }).click();
  const room = panel.locator('[data-view-field="cf.room"]');
  assert.equal(await room.count(), 1, 'the custom field is not offered as a column');
  await room.getByRole('checkbox').check();
  const head = page.locator('.fields-head .col-head[data-field="cf.room"]');
  await head.waitFor({ state: 'attached', timeout: 5_000 });
  assert.equal((await head.textContent()).trim(), 'Room');

  // The board: columns by, lanes by and the limits, in the panel's own section.
  await panel.locator('[data-layout="KANBAN"]').click();
  await panel.getByRole('heading', { name: 'Board only' }).waitFor({ timeout: 5_000 });
  for (const label of ['Columns by', 'Lanes by', 'Limits per column']) assert.equal(await panel.getByLabel(label).count(), 1, `${label} missing`);

  // The tab is changed and not saved: a dot on it, and the button counts the parts changed.
  assert.equal(await page.getByRole('navigation', { name: COLLECTION.name }).locator('[aria-current="page"] [data-changed-dot]').count(), 1, 'the changed tab carries no dot');
  assert.match(await viewButton(page).innerText(), /^View · \d+ changed$/u);

  // Saving: name, where it lives, who sees it.
  await panel.getByLabel('Name').fill('Rooms by board');
  await panel.locator('[data-scope="HUB"]').click();
  await panel.getByRole('group', { name: 'Who sees it' }).getByRole('button', { name: 'Everyone who reads the place' }).click();
  await panel.locator('[data-view-save]').click();
  await until(page, () => saved.length > 0);
  assert.equal(saved.length, 1, 'nothing was saved');
  assert.equal(saved[0].name, 'Rooms by board');
  assert.equal(saved[0].scope_type, 'HUB');
  assert.equal(saved[0].sharing, 'SCOPE');
  assert.equal(saved[0].layout, 'KANBAN');
  assert.deepEqual(saved[0].query.sort?.[0], { field: 'title', dir: 'DESC' });
  assert.deepEqual(failures, []);
});

test('chromium 1280: the panel stands beside the list, which stays usable and shows each change at once; Escape closes it and gives focus back (scene viewcfg)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await open(browser, 1280, 900);
  t.after(() => context.close());

  await viewButton(page).click();
  const panel = panelOf(page);
  await panel.waitFor({ timeout: 5_000 });
  // Non-modal: no dialog, no scrim, nothing behind it inert; the list narrows beside it.
  assert.equal(await page.locator('dialog[open]').count(), 0, 'the panel opened as a dialog');
  assert.equal(await page.locator('[inert]').count(), 0, 'something behind the panel is inert');
  const side = page.locator('[data-view-side]');
  const [list, pane] = [await page.locator('.split').boundingBox(), await side.boundingBox()];
  assert.ok(list && pane && list.x + list.width <= pane.x + 1, `the list lies under the panel: ${JSON.stringify({ list, pane })}`);

  // A change shows at once in the list beside it, and the list still takes a press.
  await panel.locator('[data-view-field="cf.room"]').getByRole('checkbox').check();
  await page.locator('.fields-head .col-head[data-field="cf.room"]').waitFor({ state: 'attached', timeout: 5_000 });
  await page.locator('[data-row]').first().getByRole('checkbox').first().focus();
  assert.ok(await side.isVisible(), 'the panel closed when the list took focus');

  // Focus is never trapped: Tab leaves the panel's last control for what follows it.
  await panel.locator('[data-view-save]').focus();
  for (let i = 0; i < 6; i++) await page.keyboard.press('Tab');
  assert.equal(await side.evaluate((element) => element.contains(document.activeElement)), false, 'Tab stays in the panel');

  // Escape closes it from inside and gives focus back to *View*.
  await panel.getByLabel('Sort by').focus();
  await page.keyboard.press('Escape');
  await side.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(await viewButton(page).evaluate((element) => element === document.activeElement), true, 'focus did not return to View');
  assert.deepEqual(failures, []);
});

test('chromium 1280: the calendar places the entries on the days of the date the view names, saved as grouping.date_field (UC-WRK-17 check 14)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, saved } = await open(browser, 1280, 900);
  t.after(() => context.close());

  await viewButton(page).click();
  const panel = panelOf(page);
  await panel.locator('[data-layout="CALENDAR"]').click();
  await panel.getByRole('heading', { name: 'Calendar only' }).waitFor({ timeout: 5_000 });
  assert.equal(await panel.getByLabel('Placed by').inputValue(), 'due_at', 'the calendar is not placed by the due date by default');
  const calendar = page.locator('[data-calendar="due_at"]');
  await calendar.waitFor({ timeout: 10_000 });

  // The dated entry stands on its day, and nowhere else.
  const dated = ITEMS.find((item) => item.due_at);
  await calendar.locator(`[data-day="${dated.due_at.slice(0, 10)}"] [data-entry="${dated.id}"]`).waitFor({ timeout: 5_000 });
  assert.equal(await calendar.locator(`[data-entry="${dated.id}"]`).count(), 1);

  await panel.getByLabel('Name').fill('Due by day');
  await panel.locator('[data-view-save]').click();
  await until(page, () => saved.length > 0);
  assert.equal(saved[0]?.layout, 'CALENDAR');
  assert.equal(saved[0]?.grouping?.date_field, 'due_at');
  assert.deepEqual(failures, []);
});

test('chromium 1280: a board in lanes by a custom field, each lane a row of the same columns with its count, the empty lane last, saved as grouping.lanes_by (UC-WRK-05 check 12)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, saved } = await open(browser, 1280, 900);
  t.after(() => context.close());

  await viewButton(page).click();
  const panel = panelOf(page);
  await panel.locator('[data-layout="KANBAN"]').click();
  await panel.getByLabel('Lanes by').selectOption('custom_fields.room');
  const board = page.locator('.board[data-lanes]');
  await board.waitFor({ timeout: 10_000 });

  const heads = (await board.locator('.lane-head').allInnerTexts()).map((each) => each.replace(/\s+/gu, ' ').trim());
  const count = (value) => roomedItems.filter((item) => (item.custom_fields?.room ?? '') === value).length;
  assert.deepEqual(heads.slice(0, -1).sort(), [`Bath ${count('Bath')}`, `Kitchen ${count('Kitchen')}`].sort(), `the lanes: ${heads}`);
  assert.ok(heads.at(-1).endsWith(` ${count('')}`), `the lane without a value is not last: ${heads}`);
  assert.equal(await board.locator('.lane-row[data-lane=""]').count(), 1);
  // One sideways scroll for all lanes: each lane's columns on one line, at the same positions in
  // every lane, never wrapped under each other.
  const placed = await board.locator('.lane-row').evaluateAll((lanes) => lanes.map((lane) => [...lane.querySelectorAll('[data-column-zone]')].map((zone) => {
    const box = zone.getBoundingClientRect();
    return { zone: zone.dataset.columnZone, x: Math.round(box.left), y: Math.round(box.top) };
  })));
  for (const lane of placed) {
    assert.equal(new Set(lane.map((each) => each.y)).size, 1, `a lane wraps its columns: ${JSON.stringify(lane)}`);
    assert.deepEqual(lane.map((each) => [each.zone, each.x]), placed[0].map((each) => [each.zone, each.x]), 'the lanes draw their columns at different positions');
  }
  const scroll = await board.evaluate((element) => ({ overflow: getComputedStyle(element).overflowX, lanes: [...element.querySelectorAll('.lane-row')].map((lane) => lane.scrollWidth <= lane.clientWidth + 1) }));
  assert.equal(scroll.overflow, 'auto', 'the board does not scroll sideways as one');
  assert.ok(scroll.lanes.every(Boolean), 'a lane scrolls on its own');

  await panel.getByLabel('Name').fill('By room');
  await panel.locator('[data-view-save]').click();
  await until(page, () => saved.length > 0);
  assert.equal(saved[0]?.layout, 'KANBAN');
  assert.equal(saved[0]?.grouping?.lanes_by, 'custom_fields.room');
  assert.deepEqual(failures, []);
});

test('chromium 390: the panel is a sheet whose sections are pages of their own (UC-WRK-17 check 13)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await open(browser, 390, 844);
  t.after(() => context.close());

  // A phone reaches the panel from *Filter* on the chip's line (scene board 4).
  await page.locator('[data-opener="filter"]').click();
  await viewButton(page).click();
  const sheet = page.locator('dialog[open]').filter({ has: page.locator('[data-view-panel]') });
  await sheet.waitFor({ timeout: 5_000 });
  const pages = sheet.locator('[data-view-page]');
  assert.deepEqual(await pages.evaluateAll((each) => each.map((row) => row.dataset.viewPage)), ['layout', 'query', 'order', 'fields', 'save']);
  for (const row of await pages.all()) {
    const box = await row.boundingBox();
    assert.ok(box.height >= 43.5, `a page's row is ${box.height} px high`);
  }
  // A page shows its section alone, and *Back* returns to the pages.
  await sheet.locator('[data-view-page="query"]').click();
  await sheet.getByRole('combobox', { name: 'What is seen' }).waitFor({ timeout: 5_000 });
  assert.equal(await sheet.locator('[data-layout]').count(), 0, 'the layout stands on the query\'s page');
  await sheet.getByRole('button', { name: 'Back' }).click();
  await sheet.locator('[data-view-page="layout"]').click();
  await sheet.locator('[data-layout="KANBAN"]').click();
  await sheet.getByRole('button', { name: 'Back' }).click();
  assert.equal(await sheet.locator('[data-view-page="board"]').count(), 1, 'the board\'s page is not offered on a board');
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'the sheet scrolls sideways');
  assert.deepEqual(failures, []);
});

test('chromium 390 phone: a board in lanes keeps the phone\'s width, one column a lane', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const longTitle = 'Offline mode: orders placed without a connection are queued and sent once the phone is back online';
  const { context, page, failures } = await open(browser, 390, 844, { isMobile: true, longTitle });
  t.after(() => context.close());

  await page.locator('[data-opener="filter"]').click();
  await viewButton(page).click();
  const sheet = page.locator('dialog[open]').filter({ has: page.locator('[data-view-panel]') });
  await sheet.locator('[data-view-page="layout"]').click();
  await sheet.locator('[data-layout="KANBAN"]').click();
  await sheet.getByRole('button', { name: 'Back' }).click();
  await sheet.locator('[data-view-page="board"]').click();
  await sheet.getByLabel('Lanes by').selectOption('custom_fields.room');
  await page.keyboard.press('Escape');
  await page.locator('.board[data-lanes] .lane-row').first().waitFor({ timeout: 10_000 });
  await page.waitForTimeout(300);
  // A phone's browser widens its layout viewport to content that does not fit.
  assert.equal(await page.evaluate(() => window.innerWidth), 390, 'the lanes widened the phone\'s page');
  assert.deepEqual(failures, []);
});

test('chromium 390: the query line over the list is a strip with "+n" and a filter sheet, Reset and Show n entries (scene qline 5)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await open(browser, 390, 844);
  t.after(() => context.close());

  // *Filter* on the view chip's line opens the sheet (scene board 4); nothing asked, no line of its own.
  const strip = page.locator('[data-query-strip]');
  const change = page.locator('[data-opener="filter"]');
  await change.waitFor({ timeout: 15_000 });
  assert.equal((await change.innerText()).trim(), 'Filter');
  assert.ok(((await change.boundingBox())?.height ?? 0) >= 32, 'Filter is too small to tap');
  const chip = await page.locator('[data-compact] button').first().boundingBox();
  const filter = await change.boundingBox();
  assert.ok(Math.abs(chip.y + chip.height / 2 - (filter.y + filter.height / 2)) < 4, 'Filter is not on the view chip\'s line');
  await change.click();
  const sheet = page.locator('dialog[open]').filter({ hasText: 'Filter' }).last();
  await sheet.waitFor({ timeout: 5_000 });
  const typed = sheet.getByRole('combobox', { name: 'What is seen' });
  await typed.click();
  await page.keyboard.type('is: open ');
  await page.keyboard.type('label: Tiles ');
  await page.keyboard.type('due: overdue ');
  await sheet.locator('.rows li').nth(2).waitFor({ timeout: 5_000 });
  assert.equal(await sheet.locator('.rows li').count(), 3, 'a condition has no row of its own');
  await sheet.getByRole('button', { name: /^Show / }).click();
  await sheet.waitFor({ state: 'hidden', timeout: 5_000 });

  // The strip: two chips and "+1", the count beside the act.
  assert.equal(await strip.locator('.strip .chip:not(.more)').count(), 2);
  assert.equal((await strip.locator('.strip .chip.more').innerText()).trim(), '+1');
  await strip.locator('[data-view-count]').waitFor({ timeout: 5_000 });
  assert.equal((await change.innerText()).trim(), 'Filter (3)');
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'the strip scrolls the page sideways');

  // Reset empties the line.
  await change.click();
  await sheet.waitFor({ timeout: 5_000 });
  await sheet.getByRole('button', { name: 'Reset' }).click();
  await sheet.getByRole('button', { name: /^Show / }).click();
  await sheet.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(await strip.locator('.strip .chip').count(), 0);
  assert.deepEqual(failures, []);
});
