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
const UNDATED = '01a0e2e0-0000-7000-8000-0000000f1a03';

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
  unscheduled: [UNDATED],
  undated: [{ item_id: UNDATED, title: 'Write the brief' }],
  critical_path: [FIRST.id, SECOND.id],
};

const BASELINE = { id: '01a0e2e0-0000-7000-8000-0000000f1b01', plan_id: COLLECTION.id, name: 'Agreed 1 Oct', taken_at: '2026-10-01T09:00:00Z', taken_by: null, entry_count: 3 };

/** The collection's plan, with every write recorded and answered with what it makes. */
async function plan(browser, width = 1280) {
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, width, 900, { timezoneId: 'UTC' });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    const body = request.postDataJSON();
    if (method !== 'GET') written.push({ method, path, body });
    if (path === `/plans/${COLLECTION.id}/schedule`) return route.fulfill({ json: SCHEDULE });
    if (path === `/plans/${COLLECTION.id}/baselines`) return route.fulfill({ json: { data: [BASELINE] } });
    if (path === `/baselines/${BASELINE.id}` && method === 'DELETE') return route.fulfill({ status: 204 });
    if (path === `/plans/${COLLECTION.id}/tolerance`) {
      return route.fulfill({ json: { time_tolerance_days: null, source: 'DEFAULT', locked_by: null, baseline_id: null, exception: null } });
    }
    if (path === `/plans/${COLLECTION.id}/schedule:apply`) {
      const moved = SCHEDULE.entries.filter((each) => body.item_ids.includes(each.item_id)).map(({ item_id, start, finish }) => ({ item_id, start, finish }));
      return route.fulfill({ json: { moved } });
    }
    // The undated entry's own page reads it as soon as it opens, however quickly the walk goes back.
    if (path === `/items/${UNDATED}` && method === 'GET') return route.fulfill({ json: { ...FIRST, id: UNDATED, title: 'Write the brief', start_at: null, due_at: null } });
    const item = ITEMS.find((each) => path === `/items/${each.id}`);
    if (item && method === 'GET') return route.fulfill({ json: item });
    if (item && method === 'PATCH') return route.fulfill({ json: { ...item, ...body, version: item.version + 1 } });
    const linked = /^\/items\/([^/]+)\/links$/.exec(path);
    if (linked && method === 'POST') {
      return route.fulfill({ status: 201, json: { id: DRAWN, type: body.type, source_id: linked[1], target_id: body.target_id, created_at: `${today}T09:00:00Z`, waits: 'SOURCE', dependency_kind: 'FS', lag_days: 0 } });
    }
    if (path === `/links/${DRAWN}/timing` && method === 'PUT') {
      return route.fulfill({ json: { id: DRAWN, type: 'depends_on', source_id: THIRD.id, target_id: FIRST.id, created_at: `${today}T09:00:00Z`, waits: 'SOURCE', ...body } });
    }
    if (path === `/links/${LINK}` && method === 'DELETE') return route.fulfill({ status: 204 });
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

  // The plan's views are one segmented control, one view at a time (page-system §9.7): the
  // dependencies, then the table, never stacked under the chart.
  const views = page.getByRole('radiogroup', { name: 'Views of the plan' });
  assert.equal(await page.locator('[data-page-kind="work"]').count(), 1, 'the plan is no work page');
  assert.equal(await page.getByRole('table', { name: 'The plan as a table' }).count(), 0, 'the table stands under the chart');
  await views.getByRole('radio', { name: 'Dependencies' }).click();
  await page.waitForURL(/\?view=dependencies$/u, { timeout: 5_000 });
  assert.equal(await page.locator('.timeline').count(), 0, 'the chart stays beside the dependencies');

  // A dependency's lag, typed and saved in its popover (§5.1), not in a field per row.
  const opening = page.getByRole('button', { name: `Change the timing of the dependency of ${SECOND.title}` });
  await opening.click();
  const lag = page.getByLabel(`Lag of the dependency of ${SECOND.title}`);
  await lag.fill('-2');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForTimeout(300);
  const timing = written.find((each) => each.method === 'PUT' && each.path === `/links/${LINK}/timing`);
  assert.deepEqual(timing?.body, { dependency_kind: 'FS', lag_days: -2 });
  assert.equal(await lag.count(), 0, 'the popover stays open after its save');

  // A lag past the bound is refused here, before anything is sent, and the popover stays.
  await opening.click();
  await lag.fill('1000');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await page.waitForTimeout(200);
  assert.equal(written.filter((each) => each.path === `/links/${LINK}/timing`).length, 1);
  assert.equal(await lag.count(), 1, 'the refused lag closed its popover');
  await page.keyboard.press('Escape');

  await views.getByRole('radio', { name: 'Table' }).click();
  await page.getByRole('table', { name: 'The plan as a table' }).waitFor({ timeout: 5_000 });

  // An entry made manual: choosing writes nothing - a closed select changes on every arrow key
  // (SC 3.2.2) - and the apply beside it writes the mode.
  const scheduling = page.getByLabel(`Scheduling of ${FIRST.title}`);
  await scheduling.selectOption('MANUAL');
  await page.waitForTimeout(300);
  assert.equal(written.some((each) => each.path.endsWith('/scheduling')), false, 'choosing a mode wrote it');
  await page.getByRole('button', { name: `Apply the scheduling of ${FIRST.title}` }).click();
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

test('chromium: 1280 px — a dependency is added and removed without a pointer (SC 2.1.1, 2.5.7)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await plan(browser);
  t.after(close);

  // The knob the chart draws has no key bindings; the dependencies' view is the keyboard's way.
  await page.getByRole('radiogroup', { name: 'Views of the plan' }).getByRole('radio', { name: 'Dependencies' }).click();
  const adding = page.locator('[data-add-dependency]');
  await adding.getByLabel('Waiting entry').selectOption(THIRD.id);
  await adding.getByLabel('Waits for').selectOption(FIRST.id);
  await adding.getByLabel('Kind').selectOption('SS');
  await adding.getByLabel('Lag (working days)').fill('2');
  await adding.getByRole('button', { name: 'Add a dependency' }).focus();
  await page.keyboard.press('Enter');
  await page.waitForTimeout(400);

  const link = written.find((each) => each.method === 'POST' && each.path === `/items/${THIRD.id}/links`);
  assert.deepEqual(link?.body, { target_id: FIRST.id, type: 'depends_on' }, `wrote ${JSON.stringify(written.map((each) => `${each.method} ${each.path}`))}`);
  const timing = written.find((each) => each.method === 'PUT' && each.path === `/links/${DRAWN}/timing`);
  assert.deepEqual(timing?.body, { dependency_kind: 'SS', lag_days: 2 });

  // Each dependency the table lists can be taken back from its ⋯ (PS-6).
  await page.getByRole('button', { name: `More for the dependency of ${SECOND.title} on ${FIRST.title}` }).first().focus();
  await page.keyboard.press('Enter');
  await page.getByRole('menuitem', { name: `Remove the dependency of ${SECOND.title} on ${FIRST.title}` }).click();
  await page.waitForTimeout(300);
  assert.ok(written.some((each) => each.method === 'DELETE' && each.path === `/links/${LINK}`), 'the dependency was not removed');
  await page.getByRole('status').filter({ hasText: `${SECOND.title} no longer waits for ${FIRST.title}.` }).first().waitFor({ timeout: 5_000 });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// user-experience.md §7.5: the chart opens on the whole plan, a critical bar is named as such, an
// entry without dates waits in the tray with *Give it dates*, and a baseline is deleted behind
// *Undo* - nothing is sent until the message leaves without it.
test('chromium: 1280 px — the plan fits, names its critical chain, trays the undated and deletes a baseline softly', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await plan(browser);
  t.after(close);

  // The name is the title and the word, however the engine joins a hidden span.
  await page.getByRole('button', { name: new RegExp(`^${FIRST.title}\\s*,\\s*critical$`) }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('button', { name: new RegExp(`^${THIRD.title}\\s*,\\s*critical$`) }).count(), 0);
  // The titles take the room the chart leaves: where the chart is narrower than the screen, the
  // title column opens wider than its narrow default, and the chart still does not scroll.
  await page.getByRole('button', { name: 'Fit' }).click();
  await page.waitForTimeout(200);
  const room = await page.locator('.timeline').evaluate((node) => {
    const scroller = node.querySelector('.scroller');
    return { step: Number(node.dataset.title), scrolls: scroller.scrollWidth > scroller.clientWidth + 1 };
  });
  assert.ok(room.step >= 2 && !room.scrolls, `the title column stays narrow beside an empty chart: ${JSON.stringify(room)}`);
  // What has no dates is a view of its own, the chart with its tray, not a tray under the chart.
  assert.equal(await page.getByRole('button', { name: 'Give Write the brief dates' }).count(), 0, 'the tray stands under the chart');
  await page.getByRole('radiogroup', { name: 'Views of the plan' }).getByRole('radio', { name: /^No dates yet/u }).click();
  await page.getByRole('button', { name: 'Give Write the brief dates' }).click();
  await page.waitForURL(`**/items/${UNDATED}`, { timeout: 5_000 });
  await page.goBack();
  await page.locator('.timeline .scroller').waitFor({ timeout: 15_000 });

  await page.getByRole('button', { name: 'More for the plan' }).click();
  await page.getByRole('menuitem', { name: 'Plan settings' }).click();
  await page.getByRole('button', { name: 'Delete the baseline “Agreed 1 Oct”' }).click();
  await page.getByRole('button', { name: 'Undo' }).click();
  await page.waitForTimeout(300);
  assert.equal(written.some((each) => each.method === 'DELETE'), false, 'Undo sent the deletion');
  const settings = async () => {
    await page.getByRole('button', { name: 'More for the plan' }).click();
    await page.getByRole('menuitem', { name: 'Plan settings' }).click();
  };
  await settings();
  await page.getByRole('button', { name: 'Delete the baseline “Agreed 1 Oct”' }).waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Delete the baseline “Agreed 1 Oct”' }).click();
  await settings();
  assert.equal(await page.getByRole('button', { name: 'Delete the baseline “Agreed 1 Oct”' }).count(), 0, 'the baseline stayed on the screen');
  await page.keyboard.press('Escape');
  assert.equal(written.some((each) => each.method === 'DELETE'), false, 'deleted before the message left');
  await page.getByRole('button', { name: 'Dismiss' }).last().click();
  await page.waitForTimeout(300);
  assert.ok(written.some((each) => each.method === 'DELETE' && each.path === `/baselines/${BASELINE.id}`), 'the deletion was never sent');

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// §7.5 on a phone, the chart first: each title stands above its bar as wide as the screen and
// stays in view while the chart scrolls within itself; the bars start at the screen's start.
test('chromium: 375 px — the chart takes the width and its titles stand above the bars', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await plan(browser, 375);
  t.after(close);
  const at = await page.evaluate(() => {
    const scroller = document.querySelector('.timeline .scroller');
    const box = scroller.getBoundingClientRect();
    const title = document.querySelector('.timeline .lead').getBoundingClientRect();
    const axis = document.querySelector('.timeline .axis').getBoundingClientRect();
    const lanes = document.querySelector('.timeline .lanes').getBoundingClientRect();
    const bars = [...document.querySelectorAll('.timeline .cell[data-filled]')].map((each) => each.getBoundingClientRect())
      .filter((each) => each.right > box.left && each.left < box.right);
    return { width: box.width, title: title.width, titleBottom: title.bottom, lanesTop: lanes.top, axis: axis.left, lanes: lanes.left, bars: bars.length, sideways: document.documentElement.scrollWidth - document.documentElement.clientWidth };
  });
  assert.ok(at.title >= at.width * 0.95, `the title takes ${Math.round(at.title)} of ${Math.round(at.width)} px`);
  assert.ok(at.titleBottom <= at.lanesTop + 1, 'the title stands above its bar');
  assert.equal(Math.round(at.lanes), Math.round(at.axis), 'the bars start where the axis does');
  assert.ok(at.bars > 0, 'bars are drawn in view');
  assert.ok(at.sideways <= 0, `the page scrolls ${at.sideways} px sideways`);
  const scrolled = await page.evaluate(() => {
    const scroller = document.querySelector('.timeline .scroller');
    scroller.scrollLeft = scroller.scrollWidth;
    const box = scroller.getBoundingClientRect();
    const title = document.querySelector('.timeline .lead').getBoundingClientRect();
    return { box: box.left, title: title.left };
  });
  assert.equal(Math.round(scrolled.title), Math.round(scrolled.box), 'the title stays in view as the chart scrolls');
});
