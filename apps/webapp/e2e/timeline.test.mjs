// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The timeline shows time (F10-14, ADR-0063 decision 12). It was a list with one bar in it: a
// window of the current month, an unlabelled mark per day, the undated in a list beside the axis,
// and no way to change a date from the picture. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, ITEMS, PAGE, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The one entry the fixture dates, and the one every drag here takes hold of. */
const DATED = ITEMS[1];

const DAY = 86_400_000;
const dayOf = (date) => Date.parse(`${date}T00:00:00Z`);
const addDays = (date, days) => new Date(dayOf(date) + days * DAY).toISOString().slice(0, 10);

/**
 * Which day an instant is, in the zone it was written with.
 *
 * Not the first ten characters of the instant: an all-day date is midnight in *its* zone, which
 * in Berlin is 22:00 the day before in UTC. Reading the prefix is how a date ends up a day short.
 */
const dayIn = (iso, zone) => new Intl.DateTimeFormat('en-CA', { timeZone: zone, dateStyle: 'short' }).format(Date.parse(iso));

/**
 * The timeline of the collection, with every write recorded.
 *
 * `rows` lets a walk serve its own entries: what the window opens on is decided from what the
 * first read answers, so a collection whose work is a season away has to be a different answer
 * rather than a different assertion.
 */
async function timeline(browser, { width = 1280, rows } = {}) {
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, width, 900);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() !== 'GET') written.push({ method: request.method(), path, body: request.postDataJSON() });
    if (rows && path.endsWith('/api/v1/items:query') && !request.postDataJSON()?.group_by) {
      return route.fulfill({ json: { data: rows, groups: [], page: PAGE.page, total: rows.length } });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('radio', { name: 'Timeline' }).click();
  await page.locator('.timeline .scroller').waitFor({ timeout: 15_000 });
  return { page, failures, written, close: () => context.close(), unstubbed };
}

/** A pointer press carried from one point to another, in steps, as a hand would. */
async function carry(page, from, to, { steps = 12 } = {}) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  for (let step = 1; step <= steps; step += 1) {
    await page.mouse.move(from.x + ((to.x - from.x) * step) / steps, from.y + ((to.y - from.y) * step) / steps);
    await page.waitForTimeout(20);
  }
  await page.mouse.up();
  await page.waitForTimeout(300);
}

/** How wide one day is on the axis, and how many of them are drawn. */
async function axisOf(page) {
  return page.evaluate(() => {
    const axis = document.querySelector('.timeline .axis');
    if (!axis) return null;
    const box = axis.getBoundingClientRect();
    return { x: box.left, y: box.top, width: box.width / axis.children.length, days: axis.children.length };
  });
}

test('chromium: 1280 px — a scale, dated gridlines, today marked, and a tray that folds', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await timeline(browser);
  t.after(close);

  // The scale is the three the decision names, and it is a real control.
  const scale = page.getByLabel('Scale');
  assert.deepEqual((await scale.locator('option').allTextContents()).map((each) => each.trim()), ['Days', 'Weeks', 'Months']);

  // Dated gridlines: the axis says dates rather than drawing unlabelled marks.
  const labels = (await page.locator('.timeline .axis .tick').allTextContents()).filter((each) => each.trim() !== '');
  assert.ok(labels.length >= 4, `the axis carries ${labels.length} dated gridlines`);

  // Today is marked, on the axis and through every row, and said once in words.
  assert.equal(await page.locator('.timeline .axis .tick[data-today]').count(), 1);
  assert.ok(await page.locator('.timeline .track .cell[data-today]').count() >= 1, 'today is not marked through the rows');
  await page.getByText(/^Today is .*marked on the axis\.$/).waitFor({ timeout: 5_000 });

  // The dated entry is on the axis; the four without dates are in the tray, and it folds.
  const tray = page.locator('.timeline details.undated');
  assert.equal(await tray.evaluate((el) => el.open), true, 'the tray opens folded, so the undated are hidden until asked for');
  const trayed = (await tray.locator('.entry').allTextContents()).map((each) => each.trim());
  assert.deepEqual(trayed.sort(), ITEMS.filter((each) => !each.due_at).map((each) => each.title).sort());
  await page.locator('.timeline details.undated summary').click();
  assert.equal(await tray.evaluate((el) => el.open), false, 'the tray does not fold');

  // The one dated entry is a row on the axis, and its row opens the entry: that is the keyboard
  // path every drag here has (SC 2.5.7).
  const row = page.locator('.timeline .rows .row button.title');
  assert.equal(await row.count(), 1);
  await row.focus();
  await page.keyboard.press('Enter');
  await page.waitForFunction((id) => location.pathname === `/items/${id}`, DATED.id, { timeout: 5_000 })
    .catch(() => assert.fail(`the row led to ${page.url()} rather than to the entry`));

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — a point dragged moves the one date it has, and invents no other', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await timeline(browser);
  t.after(close);

  await page.getByLabel('Scale').selectOption('day');
  await page.waitForTimeout(200);

  const due = DATED.due_at.slice(0, 10);
  const at = await axisOf(page);
  const cell = page.locator('.timeline .track .cell[data-filled]').first();
  await cell.waitFor({ timeout: 5_000 });
  const box = await cell.boundingBox();
  assert.ok(box, 'the dated entry sits on no column');

  // Three columns later. The entry carries only a due date, so it is a point: the drag moves the
  // one date it has rather than inventing a start for it.
  await carry(page, { x: box.x + box.width / 2, y: box.y + box.height / 2 }, { x: box.x + box.width / 2 + at.width * 3, y: box.y + box.height / 2 });

  const wrote = written.find((each) => each.method === 'PUT' && each.path.endsWith(`/items/${DATED.id}/due`));
  assert.ok(wrote, `no due date was written: ${JSON.stringify(written.map((each) => `${each.method} ${each.path}`))}`);
  assert.equal(dayIn(wrote.body.due_at, wrote.body.due_time_zone), addDays(due, 3));
  // A bar is drawn in days, so the day it is dropped on is an all-day date rather than a time
  // this client invented.
  assert.equal(wrote.body.due_date_only, true);
  assert.equal(written.some((each) => each.method === 'PATCH' && 'start_at' in (each.body ?? {})), false);

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — a bar moves both dates and an end moves one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const today = new Date().toISOString().slice(0, 10);
  const span = { start: addDays(today, 1), due: addDays(today, 4) };
  const spanned = { ...ITEMS[0], start_at: `${span.start}T09:00:00Z`, due_at: `${span.due}T09:00:00Z` };

  /** One drag on the span, and the writes it made. */
  const dragged = async (which, columns) => {
    const { page, written, close, unstubbed } = await timeline(browser, { rows: [spanned] });
    t.after(close);
    await page.getByLabel('Scale').selectOption('day');
    await page.waitForTimeout(200);
    const at = await axisOf(page);
    const target =
      which === 'bar'
        ? page.locator('.timeline .track .cell[data-filled]').nth(1)
        : page.getByRole('button', { name: which === 'start' ? 'Move the start' : 'Move the due date' });
    await target.waitFor({ timeout: 5_000 });
    const box = await target.boundingBox();
    const middle = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
    await carry(page, middle, { x: middle.x + at.width * columns, y: middle.y });
    return written.filter((each) => each.method !== 'POST');
  };

  // The whole bar: both dates move, and the span keeps its length.
  const both = await dragged('bar', 2);
  const movedStart = both.find((each) => each.method === 'PATCH' && 'start_at' in (each.body ?? {}));
  const movedDue = both.find((each) => each.method === 'PUT' && each.path.endsWith('/due'));
  assert.ok(movedStart && movedDue, `a bar wrote ${JSON.stringify(both.map((each) => `${each.method} ${each.path}`))}`);
  assert.equal(dayIn(movedDue.body.due_at, movedDue.body.due_time_zone), addDays(span.due, 2));

  // One end: one date, and the other is not touched.
  const end = await dragged('due', 2);
  assert.equal(end.some((each) => each.method === 'PATCH' && 'start_at' in (each.body ?? {})), false, 'an end dragged moved the start too');
  const onlyDue = end.find((each) => each.method === 'PUT' && each.path.endsWith('/due'));
  assert.ok(onlyDue, `an end wrote ${JSON.stringify(end.map((each) => `${each.method} ${each.path}`))}`);
  assert.equal(dayIn(onlyDue.body.due_at, onlyDue.body.due_time_zone), addDays(span.due, 2));
});

test('chromium: 1280 px — a bar end is a target, and a one-day bar has no ends to take', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const today = new Date().toISOString().slice(0, 10);
  /** The handles a row of these dates draws, at each scale, as `width x height`. */
  const handlesOf = async (start, due) => {
    const { page, close } = await timeline(browser, {
      rows: [{ ...ITEMS[0], start_at: `${start}T09:00:00Z`, due_at: `${due}T09:00:00Z` }],
    });
    t.after(close);
    const measured = {};
    for (const scale of ['day', 'week', 'month']) {
      await page.getByLabel('Scale').selectOption(scale);
      await page.waitForTimeout(200);
      measured[scale] = await page
        .locator('.timeline .end')
        .evaluateAll((ends) => ends.map((end) => {
          const box = end.getBoundingClientRect();
          return `${Math.round(box.width)}x${Math.round(box.height)}`;
        }));
    }
    return measured;
  };

  // The target is the cell, not the mark drawn in it. At the day scale that is 24 x 24, which is
  // where design-system.md §6 rule 1 puts the floor (SC 2.5.8). Narrower columns cannot reach it
  // and rest on 2.5.8's Equivalent clause, which §11 names - but the block axis holds at every
  // scale, and the row does not get taller for it.
  const span = await handlesOf(addDays(today, 1), addDays(today, 4));
  assert.deepEqual(span.day, ['24x24', '24x24'], 'a bar end is under the target floor at the day scale');
  assert.deepEqual(span.week, ['8x24', '8x24']);
  assert.deepEqual(span.month, ['4x24', '4x24']);

  // One column wide, a bar has no two ends: the same cell would be both, so it is dragged as a bar
  // - which is the only reading a one-day span has.
  const oneDay = await handlesOf(addDays(today, 2), addDays(today, 2));
  assert.deepEqual([oneDay.day, oneDay.week, oneDay.month], [[], [], []]);
});

test('chromium: 1280 px — a tray entry carried across the axis is given its first dates', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await timeline(browser);
  t.after(close);

  await page.getByLabel('Scale').selectOption('day');
  await page.waitForTimeout(200);

  const undatedEntry = ITEMS.find((each) => !each.due_at && !each.completion?.is_completed);
  const entry = page.locator('.timeline details.undated .entry', { hasText: undatedEntry.title });
  const box = await entry.boundingBox();
  const axis = await axisOf(page);
  const width = axis.width;

  // From the tray onto the axis, across four columns: the drag begins where the pointer went down,
  // which is the column under the tray entry, and ends four columns along.
  const start = { x: axis.x + width * 2.5, y: box.y + box.height / 2 };
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(start.x, start.y);
  await carryFrom(page, start, { x: axis.x + width * 6.5, y: axis.y + 40 });

  const patched = written.find((each) => each.method === 'PATCH' && 'start_at' in (each.body ?? {}));
  const dued = written.find((each) => each.method === 'PUT' && each.path.endsWith(`/items/${undatedEntry.id}/due`));
  assert.ok(patched, `no start was written: ${JSON.stringify(written.map((each) => `${each.method} ${each.path}`))}`);
  assert.ok(dued, 'no due date was written');
  assert.ok(patched.body.start_at < dued.body.due_at, 'the entry was given a due date before its start');

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

/** The second half of a carry, for a gesture that began somewhere else. */
async function carryFrom(page, from, to, { steps = 12 } = {}) {
  for (let step = 1; step <= steps; step += 1) {
    await page.mouse.move(from.x + ((to.x - from.x) * step) / steps, from.y + ((to.y - from.y) * step) / steps);
    await page.waitForTimeout(20);
  }
  await page.mouse.up();
  await page.waitForTimeout(300);
}

test('chromium: 1280 px — a collection whose work is a season away opens on it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  // Far enough out that no window around today reaches it, which is the case the walk of
  // 2026-09-22 found: the entry was outside the window with nothing saying so.
  const today = new Date().toISOString().slice(0, 10);
  const far = addDays(today, 240);
  const { page, failures, close, unstubbed } = await timeline(browser, {
    rows: [{ ...ITEMS[0], due_at: `${far}T09:00:00Z`, start_at: `${addDays(far, -4)}T09:00:00Z` }],
  });
  t.after(close);

  // The entry is drawn, which is the whole of the acceptance: a window that opened on today would
  // hold nothing at all.
  await page.locator('.timeline .rows .row').first().waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('.timeline .rows .row').count(), 1);
  assert.ok(await page.locator('.timeline .track .cell[data-filled]').count() >= 1, 'the entry is on no column');
  // And today is not in that window, so nothing pretends it is.
  assert.equal(await page.locator('.timeline .axis .tick[data-today]').count(), 0);

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 375 px — the scale is reachable and the axis shows a usable range', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await timeline(browser, { width: 375 });
  t.after(close);

  await page.getByLabel('Scale').selectOption('day');
  await page.waitForTimeout(200);

  // A fortnight is drawn, whatever the width: the range is the scale's, and the scroller is what
  // makes it usable rather than a narrower window nobody chose.
  assert.equal(await page.locator('.timeline .axis .tick').count(), 14);
  // And the page does not scroll sideways because of it - the timeline scrolls inside itself.
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, 'the timeline widens the page');

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});
