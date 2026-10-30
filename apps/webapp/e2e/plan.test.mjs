// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Plan screen's chart is the picture of the schedule the server answers, and a hand changes it
// (UC-PLN-02 checks 3 and 7, UC-PLN-03): a bar carried moves both dates, an end carried moves one,
// a knob drawn onto another row makes the dependency, and the table beside it writes the same
// things from the keyboard. Chromium only, as the other walks.

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

const DAY = 86_400_000;
const today = new Date().toISOString().slice(0, 10);
const addDays = (date, days) => new Date(Date.parse(`${date}T00:00:00Z`) + days * DAY).toISOString().slice(0, 10);
const dayIn = (iso, zone) => new Intl.DateTimeFormat('en-CA', { timeZone: zone, dateStyle: 'short' }).format(Date.parse(iso));

const [FIRST, SECOND, THIRD] = ITEMS;
const LINK = '01a0e2e0-0000-7000-8000-0000000f1a01';
const DRAWN = '01a0e2e0-0000-7000-8000-0000000f1a02';

/** One entry of the schedule, as `GET /plans/{id}/schedule` answers it. */
const entry = (item, start, finish, extra = {}) => ({
  item_id: item.id, title: item.title, parent_id: null, collection_id: COLLECTION.id, start, finish,
  duration_days: 3, moved: false, total_float_days: 0, free_float_days: 0, critical: true, summary: false,
  external: false, mode: 'AUTOMATIC', milestone: false, done: false, ...extra,
});

/** Three entries: the second waits on the first, the third stands alone and the scheduler would move it. */
const SCHEDULE = {
  zone: 'UTC',
  finish: addDays(today, 12),
  entries: [
    entry(FIRST, addDays(today, 1), addDays(today, 3)),
    entry(SECOND, addDays(today, 5), addDays(today, 7)),
    entry(THIRD, addDays(today, 10), addDays(today, 12), { critical: false, total_float_days: 4, moved: true }),
  ],
  dependencies: [{ link_id: LINK, type: 'depends_on', predecessor_id: FIRST.id, successor_id: SECOND.id, dependency_kind: 'FS', lag_days: 1 }],
  conflicts: [],
  unscheduled: [],
  critical_path: [FIRST.id, SECOND.id],
};

/** The collection's plan, with every write recorded and answered with what it makes. */
async function plan(browser) {
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900, { timezoneId: 'UTC' });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    const body = request.postDataJSON();
    if (method !== 'GET') written.push({ method, path, body });
    if (path === `/plans/${COLLECTION.id}/schedule`) return route.fulfill({ json: SCHEDULE });
    if (path === `/plans/${COLLECTION.id}/baselines`) return route.fulfill({ json: { data: [] } });
    if (path === `/plans/${COLLECTION.id}/tolerance`) {
      return route.fulfill({ json: { time_tolerance_days: null, source: 'DEFAULT', locked_by: null, baseline_id: null, exception: null } });
    }
    if (path === `/plans/${COLLECTION.id}/schedule:apply`) {
      const moved = SCHEDULE.entries.filter((each) => body.item_ids.includes(each.item_id)).map(({ item_id, start, finish }) => ({ item_id, start, finish }));
      return route.fulfill({ json: { moved } });
    }
    const item = ITEMS.find((each) => path === `/items/${each.id}`);
    if (item && method === 'GET') return route.fulfill({ json: item });
    if (item && method === 'PATCH') return route.fulfill({ json: { ...item, ...body, version: item.version + 1 } });
    const linked = /^\/items\/([^/]+)\/links$/.exec(path);
    if (linked && method === 'POST') {
      return route.fulfill({ status: 201, json: { id: DRAWN, type: body.type, source_id: linked[1], target_id: body.target_id, created_at: `${today}T09:00:00Z`, waits: 'SOURCE', dependency_kind: 'FS', lag_days: 0 } });
    }
    if (path === `/links/${LINK}/timing` && method === 'PUT') {
      return route.fulfill({ json: { id: LINK, type: 'depends_on', source_id: SECOND.id, target_id: FIRST.id, created_at: `${today}T09:00:00Z`, waits: 'SOURCE', ...body } });
    }
    const scheduling = /^\/items\/([^/]+)\/scheduling$/.exec(path);
    if (scheduling && method === 'PUT') return route.fulfill({ json: { item_id: scheduling[1], mode: 'AUTOMATIC', milestone: false, ...body } });
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}/plan`);
  await page.locator('.timeline .scroller').waitFor({ timeout: 15_000 });
  await page.getByLabel('Zoom').selectOption('day');
  await page.waitForTimeout(200);
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
  await page.waitForTimeout(400);
}

/** How wide one day is on the axis. */
const columnWidth = (page) =>
  page.evaluate(() => {
    const axis = document.querySelector('.timeline .axis');
    return axis.getBoundingClientRect().width / axis.children.length;
  });

const middleOf = async (locator) => {
  await locator.waitFor({ timeout: 5_000 });
  const box = await locator.boundingBox();
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
};

const rowOf = (page, item) => page.locator(`.timeline .rows [data-row="${item.id}"]`);

test('chromium: 1280 px — a bar carried moves both dates, an end carried moves one', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  // The whole bar, two days later: the start as a scalar, the due as an all-day date.
  {
    const { page, failures, written, close, unstubbed } = await plan(browser);
    t.after(close);
    const width = await columnWidth(page);
    const at = await middleOf(rowOf(page, FIRST).locator('.track .cell[data-filled]').nth(1));
    await carry(page, at, { x: at.x + width * 2, y: at.y });

    const start = written.find((each) => each.method === 'PATCH' && each.path === `/items/${FIRST.id}`);
    const due = written.find((each) => each.method === 'PUT' && each.path === `/items/${FIRST.id}/due`);
    assert.ok(start && due, `the bar wrote ${JSON.stringify(written.map((each) => `${each.method} ${each.path}`))}`);
    assert.equal(dayIn(start.body.start_at, 'UTC'), addDays(today, 3));
    assert.equal(dayIn(due.body.due_at, due.body.due_time_zone), addDays(today, 5));
    assert.equal(due.body.due_date_only, true);
    // The server schedules: nothing but the entry's own dates was written.
    assert.equal(written.some((each) => each.path.includes(':apply') || each.path.endsWith('/scheduling')), false);
    assert.deepEqual(failures, []);
    assert.deepEqual(unstubbed(), []);
  }

  // Only the due end, one day later: the start stays as it is.
  {
    const { page, failures, written, close, unstubbed } = await plan(browser);
    t.after(close);
    const width = await columnWidth(page);
    const at = await middleOf(rowOf(page, FIRST).getByRole('button', { name: 'Move the due date' }));
    await carry(page, at, { x: at.x + width, y: at.y });

    const due = written.find((each) => each.method === 'PUT' && each.path === `/items/${FIRST.id}/due`);
    assert.ok(due, `the end wrote ${JSON.stringify(written.map((each) => `${each.method} ${each.path}`))}`);
    assert.equal(dayIn(due.body.due_at, due.body.due_time_zone), addDays(today, 4));
    assert.equal(written.some((each) => each.method === 'PATCH'), false, 'a resized end moved the start too');
    assert.deepEqual(failures, []);
    assert.deepEqual(unstubbed(), []);
  }
});

test('chromium: 1280 px — a knob drawn onto another row makes that row wait', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await plan(browser);
  t.after(close);

  // The chart draws the dependency the schedule holds, and says it in words.
  assert.equal(await page.locator(`.timeline .arrows [data-dependency="${LINK}"]`).count(), 1);
  assert.equal(await page.locator(`.timeline [data-dependency-said="${LINK}"]`).count(), 1);

  // From the first bar's knob onto the third row: the third now waits for the first.
  const knob = await middleOf(rowOf(page, FIRST).getByRole('button', { name: 'Drag onto the entry that waits for this one' }));
  const onto = await middleOf(rowOf(page, THIRD).locator('.track'));
  await carry(page, knob, onto);

  const link = written.find((each) => each.method === 'POST' && each.path === `/items/${THIRD.id}/links`);
  assert.ok(link, `the knob wrote ${JSON.stringify(written.map((each) => `${each.method} ${each.path}`))}`);
  assert.deepEqual(link.body, { target_id: FIRST.id, type: 'depends_on' });
  // A drag let go on the row it began on makes nothing.
  assert.equal(written.filter((each) => each.path.endsWith('/links')).length, 1);
  await page.getByRole('status').filter({ hasText: `${THIRD.title} now waits for ${FIRST.title}.` }).first().waitFor({ timeout: 5_000 });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — the table writes what the chart draws, from the keyboard', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await plan(browser);
  t.after(close);

  // A dependency's lag, typed and saved.
  const lag = page.getByLabel(`Lag of the dependency of ${SECOND.title}`);
  await lag.fill('-2');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForTimeout(300);
  const timing = written.find((each) => each.method === 'PUT' && each.path === `/links/${LINK}/timing`);
  assert.deepEqual(timing?.body, { dependency_kind: 'FS', lag_days: -2 });

  // A lag past the bound is refused here, before anything is sent.
  await lag.fill('1000');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForTimeout(200);
  assert.equal(written.filter((each) => each.path === `/links/${LINK}/timing`).length, 1);

  // An entry made manual, and the one proposed move accepted.
  await page.getByLabel(`Scheduling of ${FIRST.title}`).selectOption('MANUAL');
  await page.waitForTimeout(300);
  const mode = written.find((each) => each.method === 'PUT' && each.path === `/items/${FIRST.id}/scheduling`);
  assert.deepEqual(mode?.body, { mode: 'MANUAL' });

  await page.getByRole('button', { name: 'Accept every move' }).click();
  await page.waitForTimeout(300);
  const applied = written.find((each) => each.method === 'POST' && each.path === `/plans/${COLLECTION.id}/schedule:apply`);
  assert.deepEqual(applied?.body?.item_ids, [THIRD.id]);

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});
