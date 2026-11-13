// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A collection's sprints (UC-PLN-06, UC-PLN-07) and a hub's releases (UC-PLN-08) as a person uses
// them: the running sprint opens with its burndown as a table and the velocity beside it, an entry
// moves from the backlog into the sprint, an estimate is typed, the sprint closes into the next;
// a release takes entries of the hub's collection and goes out with its notes. The server counts
// and ships; the walk proves what the screens send. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, HUB, ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const DAY = 86_400_000;
const today = new Date().toISOString().slice(0, 10);
const addDays = (date, days) => new Date(Date.parse(`${date}T00:00:00Z`) + days * DAY).toISOString().slice(0, 10);

const sprint = (n, name, state, start, end, extra = {}) => ({
  id: `01a0e2e0-0000-7000-8000-0000000e5b0${n}`, collection_id: COLLECTION.id, name, goal: '', start_on: start, end_on: end,
  state, unit: 'POINTS', committed_value: 0, committed_count: 0, completed_value: 0, completed_count: 0,
  started_at: null, closed_at: null, version: 1, ...extra,
});
const CLOSED = sprint(1, 'Sprint 1', 'CLOSED', addDays(today, -17), addDays(today, -4), { committed_value: 8, committed_count: 2, completed_value: 5, completed_count: 1 });
const RUNNING = sprint(2, 'Sprint 2', 'ACTIVE', addDays(today, -3), addDays(today, 10), { goal: 'The worktop is in', committed_value: 8, committed_count: 2, started_at: `${addDays(today, -3)}T08:00:00Z` });
const NEXT = sprint(3, 'Sprint 3', 'PLANNED', addDays(today, 11), addDays(today, 24));
const SPRINTS = [CLOSED, RUNNING, NEXT];
const points = (value) => ({ unit: 'POINTS', value, size: null });
const inSprint = (item, value, extra = {}) => ({
  item_id: item.id, title: item.title, added_at: `${addDays(today, -3)}T08:00:00Z`, removed_at: null, committed: true,
  outcome: null, done: false, started: false, completed_at: null, estimate: points(value), ...extra,
});
// An entry of the sprint beyond the collection's first read page: its column is read on its own.
const BEYOND = { ...ITEMS[1], id: '01a0e2e0-0000-7000-8000-0000000e5bf1', title: 'Seal the grout', bucket_id: BUCKETS[1].id };
const DETAIL = {
  sprint: RUNNING,
  entries: [inSprint(ITEMS[0], 5, { done: true, completed_at: `${addDays(today, -1)}T15:00:00Z` }), inSprint(ITEMS[1], 3, { started: true }), inSprint(BEYOND, 0)],
  burndown: [
    { day: addDays(today, -3), remaining: 8, scope: 8, ideal: 8 },
    { day: addDays(today, -2), remaining: 8, scope: 8, ideal: 7.4 },
    { day: addDays(today, -1), remaining: 3, scope: 8, ideal: 6.8 },
  ],
};
const BACKLOG = { data: [{ item_id: ITEMS[3].id, title: ITEMS[3].title, estimate: null }, { item_id: ITEMS[4].id, title: ITEMS[4].title, estimate: points(2) }] };

const release = (n, name, state, on, extra = {}) => ({
  id: `01a0e2e0-0000-7000-8000-0000000e5c0${n}`, hub_id: HUB.id, name, description: '', release_on: on, state,
  released_at: null, notes_page_id: null, version: 1, done_count: 0, open_count: 0, ...extra,
});
const SHIPPED = release(1, '1.0', 'RELEASED', addDays(today, -20), { released_at: `${addDays(today, -20)}T12:00:00Z`, notes_page_id: '01a0e2e0-0000-7000-8000-0000000e5d01' });
const COMING = release(2, '1.1', 'PLANNED', addDays(today, 7), { description: 'The kitchen is usable.', done_count: 1, open_count: 0 });
const LATER = release(3, '1.2', 'PLANNED', addDays(today, 30));
const RELEASES = [SHIPPED, COMING, LATER];
const RELEASE_DETAIL = {
  ...COMING,
  entries: [{ item_id: ITEMS[0].id, title: ITEMS[0].title, type_name: 'Task', done: true, collection_id: COLLECTION.id, added_at: `${addDays(today, -5)}T09:00:00Z`, key: 'HOME-3', status_name: 'Fitted', status_category: 'DONE' }],
  done_count: 1,
  open_count: 0,
};

/** A screen of the planning area, with every write recorded and answered with what it makes. */
async function open(browser, at, width = 1280) {
  const written = [];
  const reads = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, width, width < 600 ? 844 : 900, { timezoneId: 'UTC' });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    const body = request.postDataJSON();
    if (method !== 'GET') written.push({ method, path, body });
    else reads.push(path);

    if (path === `/collections/${COLLECTION.id}/sprints` && method === 'GET') return route.fulfill({ json: { data: SPRINTS, runs_sprints: true } });
    if (path === `/collections/${COLLECTION.id}/sprints` && method === 'POST') return route.fulfill({ status: 201, json: sprint(4, body.name, 'PLANNED', body.start_on, body.end_on, { goal: body.goal ?? '' }) });
    if (path === `/collections/${COLLECTION.id}/sprint-backlog`) return route.fulfill({ json: BACKLOG });
    if (path === `/collections/${COLLECTION.id}/velocity`) return route.fulfill({ json: { unit: 'POINTS', average: 5, sprints: [CLOSED] } });
    const one = SPRINTS.find((each) => path === `/sprints/${each.id}`);
    if (one && method === 'GET') return route.fulfill({ json: one === RUNNING ? DETAIL : { sprint: one, entries: [], burndown: [] } });
    const into = SPRINTS.find((each) => path === `/sprints/${each.id}/entries`);
    if (into && method === 'POST') return route.fulfill({ json: into });
    if (path === `/items/${BEYOND.id}` && method === 'GET') return route.fulfill({ json: BEYOND });
    const ranked = /^\/items\/([^/:]+):reorder$/.exec(path);
    if (ranked && method === 'POST') return route.fulfill({ json: { ...ITEMS.find((each) => each.id === ranked[1]), version: 2 } });
    if (path === `/sprints/${RUNNING.id}:close` && method === 'POST') {
      return route.fulfill({ json: { sprint: { ...RUNNING, state: 'CLOSED', closed_at: `${today}T17:00:00Z` }, carried: [ITEMS[1].id], returned: [], carried_to: body.carry_to_sprint_id ?? null } });
    }
    const estimated = /^\/items\/([^/]+)\/estimate$/.exec(path);
    if (estimated && method === 'PUT') return route.fulfill({ json: { item_id: estimated[1], size: null, ...body } });

    if (path === `/hubs/${HUB.id}/releases` && method === 'GET') return route.fulfill({ json: { data: RELEASES } });
    const shown = RELEASES.find((each) => path === `/releases/${each.id}`);
    if (shown && method === 'GET') return route.fulfill({ json: shown === COMING ? RELEASE_DETAIL : { ...shown, entries: [], done_count: 0, open_count: 0 } });
    if (path === `/releases/${COMING.id}/entries` && method === 'POST') return route.fulfill({ json: COMING });
    if (path === `/releases/${COMING.id}:publish` && method === 'POST') {
      return route.fulfill({ json: { ...COMING, state: 'RELEASED', released_at: `${today}T12:00:00Z`, notes_page_id: '01a0e2e0-0000-7000-8000-0000000e5d02' } });
    }
    // The entries of the hub's collection that the release offers to take in.
    if (path === '/items' && url.searchParams.get('collection_id') === COLLECTION.id) return route.fulfill({ json: { data: ITEMS.slice(0, 3), page: { next_cursor: null, has_more: false } } });
    return stub(route);
  });
  await page.goto(`${served.origin}${at}`);
  return { page, failures, written, reads, close: () => context.close(), unstubbed };
}

const wrote = (written, method, path) => written.find((each) => each.method === method && each.path === path);
const settle = (page) => page.waitForTimeout(300);

test('chromium: 1280 px — the running sprint\'s bar, the filter line, its columns with their points, cards with key and points chip, and a close', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await open(browser, `/collections/${COLLECTION.id}/sprints`);
  t.after(close);

  // UC-PLN-06 check 7, UC-PLN-07 check 5: it opens on the running sprint, not the first in the
  // list, in one bar (work-surface §6.2, E-22): its goal, the days left, the points done and the burndown, and *Close sprint*.
  const bar = page.getByRole('region', { name: 'The sprint' });
  const chooser = bar.getByLabel('Sprint', { exact: true });
  await chooser.waitFor({ timeout: 15_000 });
  assert.equal(await chooser.inputValue(), RUNNING.id);
  // The chooser names the sprints alone; the state is the chip beside it, said once (scene sprint 2).
  assert.deepEqual(await chooser.locator('option').allInnerTexts(), SPRINTS.map((each) => each.name));
  assert.equal(await bar.locator('[data-state="ACTIVE"]').innerText(), 'running');
  await bar.getByText(RUNNING.goal).waitFor({ timeout: 5_000 });
  await bar.getByText('11 days', { exact: true }).waitFor({ timeout: 5_000 });
  assert.match(await bar.locator('[data-sprint-done]').innerText(), /5 \/ 8 pt/u);
  assert.equal(await bar.getByRole('img', { name: /^Burndown/ }).count(), 1, 'the burndown is not in the bar');
  assert.equal(await bar.getByRole('button', { name: 'Close sprint' }).count(), 1, 'the bar does not close the sprint');

  // The burndown is a picture with a table of the same days (UC-PLN-07 check 5), and the velocity
  // is the server's average over the closed sprints.
  assert.equal(await page.getByRole('img', { name: /^Burndown: 3 points left/ }).count(), 1);
  // The picture stands in the bar; its press unfolds it large, with its table and the velocity.
  assert.equal(await page.locator('[data-burndown]').count(), 0, 'the burndown is unfolded before its press');
  await bar.getByRole('button', { name: /Burndown/ }).click();
  const large = page.locator('[data-burndown]');
  assert.equal(await large.getByRole('img', { name: /^Burndown, large/ }).count(), 1);
  const days = large.getByRole('table', { name: 'Burndown' }).locator('tbody tr');
  assert.equal(await days.count(), DETAIL.burndown.length);
  await large.getByRole('heading', { name: 'Velocity: 5 points a sprint' }).waitFor({ timeout: 5_000 });

  // The sprint's columns are the board's, which carry the workflow's statuses (§6.2): each entry in
  // the column it stands in, a done one in the done column, each column with its points; the
  // backlog is its own tab, no column of the sprint (E-22).
  const board = page.locator('[data-sprint-board]');
  await board.locator('[data-card]').first().waitFor({ timeout: 5_000 });
  await page.locator('[data-sprint-board][data-by-columns]').waitFor({ timeout: 5_000 });
  assert.deepEqual(await board.getByRole('region').evaluateAll((all) => all.map((each) => each.getAttribute('aria-label'))), BUCKETS.map((bucket) => bucket.name));
  // The done column folded narrow, as on the board (scene sprint); its head unfolds it.
  const doneLane = board.locator(`[data-lane="${BUCKETS[2].id}"]`);
  assert.equal(await doneLane.getAttribute('data-folded'), '', 'the done column is not folded');
  await doneLane.getByTitle(`Show ${BUCKETS[2].name}`).click();
  await doneLane.locator('[data-card]').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(await board.locator('[data-points]').allInnerTexts(), ['0 pt', '3 pt', '5 pt']);
  // One row of columns, never wrapped onto a second (scene sprint 3).
  assert.equal(new Set(await board.locator('[data-lane]').evaluateAll((all) => all.map((each) => Math.round(each.getBoundingClientRect().top)))).size, 1, 'the columns wrap');
  assert.equal(await board.locator(`[data-lane="${BUCKETS[1].id}"] [data-card="${ITEMS[1].id}"]`).count(), 1, 'the entry is not in its column');
  assert.equal(await board.locator(`[data-lane="${BUCKETS[1].id}"] [data-card="${BEYOND.id}"]`).count(), 1, 'an entry beyond the first page is not in its column');
  assert.equal(await board.locator(`[data-lane="${BUCKETS[2].id}"] [data-card="${ITEMS[0].id}"]`).count(), 1, 'the done entry is not in the done column');
  assert.equal(await board.locator(`[data-lane="${BUCKETS[0].id}"] [data-card]`).count(), 0);

  // The card as scene sprint 3 draws it: its key, its title and the points chip; no field on it.
  const card = board.locator(`[data-card="${ITEMS[1].id}"]`);
  assert.equal(await card.locator('[data-entry-key]').innerText(), 'KITCHEN-32');
  assert.equal(await card.getByRole('textbox').count(), 0, 'the card carries an estimate field');
  // The chip is the keyboard's way to the estimate: pressed, it is a field; Enter keeps it.
  await card.getByRole('button', { name: `Estimate of ${ITEMS[1].title}: 3 pt, change` }).click();
  const field = card.getByLabel(`Estimate of ${ITEMS[1].title}`);
  await field.fill('5');
  await field.press('Enter');
  await settle(page);
  assert.deepEqual(wrote(written, 'PUT', `/items/${ITEMS[1].id}/estimate`)?.body, { unit: 'POINTS', value: 5 });

  // The filter line (scene sprint): words narrow the cards in their columns.
  const filter = page.getByRole('search', { name: 'Filter the sprint' });
  await filter.getByRole('searchbox').fill('grout');
  await settle(page);
  assert.deepEqual(await board.locator('[data-card]').evaluateAll((all) => all.map((each) => each.getAttribute('data-card'))), [BEYOND.id]);
  await filter.getByRole('searchbox').fill('');

  // Closed from the bar, its unfinished entries carried into the next planned sprint the dialog offers first.
  await bar.getByRole('button', { name: 'Close sprint' }).click();
  const dialog = page.getByRole('dialog');
  assert.equal(await dialog.getByLabel('Unfinished entries go to').inputValue(), NEXT.id);
  await dialog.getByRole('button', { name: 'Close sprint' }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/sprints/${RUNNING.id}:close`)?.body, { carry_to_sprint_id: NEXT.id });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — the backlog tab: the running sprint folded, the planned one against the velocity, a ranking to plan from and a line to type', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await open(browser, `/collections/${COLLECTION.id}/backlog`);
  t.after(close);

  // UC-PLN-06 check 7 (E-22, scene backlog): the running sprint folded on top, with its way in.
  const running = page.getByRole('region', { name: 'The running sprint' });
  await running.waitFor({ timeout: 15_000 });
  assert.match(await running.innerText(), /Sprint 2/u);
  assert.equal(await running.getByRole('link', { name: 'Open sprint' }).getAttribute('href'), `/collections/${COLLECTION.id}/sprint`);
  assert.equal(await page.locator('[data-sprint-bar]').count(), 0, 'the sprint\'s bar stands over the backlog');

  // The planned sprint against the velocity of the last three, and its start - which says why it
  // waits while another sprint runs (P-05).
  const planned = page.getByRole('region', { name: 'The planned sprint' });
  assert.match(await planned.innerText(), /Sprint 3/u);
  assert.match(await planned.locator('[data-load]').innerText(), /0 of ≈ 5 pt · velocity of the last 3/u);
  const start = planned.getByRole('button', { name: 'Start sprint' });
  assert.equal(await start.count(), 1);
  assert.equal(await start.getAttribute('aria-disabled'), 'true', 'a second sprint starts beside the running one');

  // UC-PLN-07: the backlog as a ranking with its sum, no per-person capacity anywhere (E-22).
  const backlog = page.locator('[data-backlog]');
  assert.match(await backlog.locator('[data-backlog-figure]').innerText(), /2 entries · 2 pt estimated · 1 without estimate/u);
  assert.deepEqual(await backlog.locator('[data-backlog-row] .title').allInnerTexts(), [ITEMS[3].title, ITEMS[4].title]);
  assert.equal(await page.getByText(/capacity/iu).count(), 0);
  // A row as scene backlog 4: the key, the title, the points chip; the filter line narrows it.
  assert.equal(await backlog.locator(`[data-backlog-row="${ITEMS[4].id}"] [data-entry-key]`).innerText(), 'KITCHEN-35');
  assert.equal(await backlog.locator(`[data-backlog-row="${ITEMS[4].id}"] [data-points-chip]`).innerText(), '2 pt');
  const filter = page.getByRole('search', { name: 'Filter the backlog' });
  await filter.getByRole('button', { name: 'Without estimate' }).click();
  assert.deepEqual(await backlog.locator('[data-backlog-row] .title').allInnerTexts(), [ITEMS[3].title]);
  await filter.getByRole('button', { name: 'Without estimate' }).click();
  // *Plan a sprint* stands at the backlog's head, where a sprint is planned from (scene backlog 4).
  assert.equal(await backlog.getByRole('button', { name: 'Plan a sprint' }).count(), 1);

  // Ranked by the keyboard: Alt+Down on the first row's handle puts it after the second.
  await backlog.getByRole('button', { name: `Rank of ${ITEMS[3].title}: Alt with the arrow keys` }).press('Alt+ArrowDown');
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/items/${ITEMS[3].id}:reorder`)?.body, { before_item_id: null });

  // Planned in by the row's arrow, the keyboard's way of the drag.
  await backlog.getByRole('button', { name: `Plan ${ITEMS[3].title} into Sprint 3` }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/sprints/${NEXT.id}/entries`)?.body, { item_ids: [ITEMS[3].id] });

  // An estimate typed in hours into the row's chip and kept by Enter, no Save beside it; one past
  // the bound is refused at the chip before anything is sent. The chip is a field once pressed.
  const chip = () => backlog.locator(`[data-backlog-row="${ITEMS[3].id}"] [data-points-chip]`);
  assert.equal(await chip().innerText(), 'estimate');
  await chip().click();
  const field = page.getByLabel(`Estimate of ${ITEMS[3].title}`);
  await field.fill('1.5h');
  await field.press('Enter');
  await settle(page);
  assert.deepEqual(wrote(written, 'PUT', `/items/${ITEMS[3].id}/estimate`)?.body, { unit: 'HOURS', value: 1.5 });
  await chip().click();
  await field.fill('20000');
  await field.press('Enter');
  await settle(page);
  assert.equal(written.filter((each) => each.path.endsWith('/estimate')).length, 1);
  assert.equal(await field.getAttribute('aria-invalid'), 'true', 'the refusal is not at the chip');

  // The line to type at the backlog's foot is capture in place (C-4): Enter adds to the collection.
  const line = backlog.locator('[data-backlog-capture]').getByRole('combobox').or(backlog.locator('[data-backlog-capture]').getByRole('textbox')).first();
  await line.fill('Order the handles');
  await line.press('Enter');
  await settle(page);
  const made = written.find((each) => each.method === 'POST' && each.path === '/items');
  assert.equal(made?.body?.title, 'Order the handles');
  assert.equal(made?.body?.collection_id ?? made?.body?.container_id, COLLECTION.id);

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 390 px — the sprint as a card over a strip of its columns, one column shown; the backlog without sideways scroll', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await open(browser, `/collections/${COLLECTION.id}/sprint`, 390);
  t.after(close);

  // Scene sprint 4 (§7): the bar a card, the columns a strip with their counts, one column drawn.
  await page.locator('[data-sprint-bar]').waitFor({ timeout: 15_000 });
  // The bar a card: the state, the days and the points on one line, no chooser; the rest under ⋯.
  const card = page.locator('[data-sprint-bar]');
  await card.locator('[data-sprint-done]').getByText('11 days left · 5 / 8 pt').waitFor({ timeout: 5_000 });
  assert.equal(await card.getByLabel('Sprint', { exact: true }).count(), 0, 'the phone\'s card carries the chooser');
  assert.equal(await card.getByRole('button', { name: 'More for the sprint' }).count(), 1);
  const strip = page.locator('[data-sprint-strip]');
  await strip.waitFor({ timeout: 5_000 });
  await page.locator('[data-sprint-board][data-by-columns]').waitFor({ timeout: 5_000 });
  assert.deepEqual(await strip.getByRole('radio').evaluateAll((all) => all.map((each) => each.getAttribute('aria-label') ?? each.textContent.trim())), ['To do 0', 'Doing 2', 'Done 1']);
  const lanes = page.locator('[data-sprint-board] [data-lane]');
  assert.equal(await lanes.count(), 1);
  assert.equal(await lanes.first().getAttribute('data-lane'), BUCKETS[1].id, 'the strip does not open on the first column with work');
  await strip.getByRole('radio', { name: 'Done 1' }).click();
  assert.equal(await lanes.first().getAttribute('data-lane'), BUCKETS[2].id);
  for (const target of await strip.getByRole('radio').all()) assert.ok(((await target.boundingBox())?.height ?? 0) >= 44, 'a strip target is under 44 px (§7)');
  assert.equal(new Set(await strip.getByRole('radio').evaluateAll((all) => all.map((each) => Math.round(each.getBoundingClientRect().top)))).size, 1, 'the strip wraps onto a second row');
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true, 'the sprint scrolls sideways');

  await page.goto(page.url().replace(/\/sprint$/u, '/backlog'));
  await page.locator('[data-backlog-row]').first().waitFor({ timeout: 15_000 });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true, 'the backlog scrolls sideways');
  for (const button of await page.locator('[data-backlog-row] button').all()) assert.ok(((await button.boundingBox())?.height ?? 0) >= 44, 'a row\'s control is under 44 px');

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — a sprint planned with a goal and a time box', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await open(browser, `/collections/${COLLECTION.id}/sprints`);
  t.after(close);

  // Planned from the bar's ⋯, where the sprint's own verbs are (scene sprint 2).
  await page.getByRole('button', { name: 'More for the sprint' }).click();
  await page.getByRole('menuitem', { name: 'Plan a sprint' }).click();
  const dialog = page.getByRole('dialog');
  // §5.2: the dialog names where the sprint is planned.
  await dialog.locator('[data-chip="place"]').filter({ hasText: `${HUB.name} › ${COLLECTION.name}` }).waitFor({ timeout: 5_000 });
  await dialog.getByLabel('Name').fill('Sprint 4');
  await dialog.getByLabel('Goal').fill('The floor is down');
  await dialog.getByLabel('First day').fill(addDays(today, 25));
  await dialog.getByLabel('Last day').fill(addDays(today, 38));
  await dialog.getByRole('button', { name: 'Plan a sprint' }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/collections/${COLLECTION.id}/sprints`)?.body, {
    name: 'Sprint 4', goal: 'The floor is down', start_on: addDays(today, 25), end_on: addDays(today, 38), unit: 'POINTS',
  });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — a release takes an entry of the hub and goes out with its notes', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, reads, close, unstubbed } = await open(browser, `/hubs/${HUB.id}/releases`);
  t.after(close);

  // UC-PLN-08 check 7. It opens on the next planned release, with what is done and open counted by the server.
  const chosen = page.locator(`[data-release-chosen="${COMING.id}"]`);
  await chosen.waitFor({ timeout: 15_000 });
  await page.locator('[data-release-done]').getByText('1 done, 0 open').waitFor({ timeout: 5_000 });
  // The head says what is coming, the next one and what went out (scene planrel 1).
  await page.getByText(/^2 planned · next: 1\.1 on .+ · 1 released( · .+)?$/).waitFor({ timeout: 5_000 });

  // Each entry with its key and its status as its workflow names it; the query that finds them
  // in the foot (scene planrel 4).
  const row = page.getByRole('row', { name: ITEMS[0].title });
  await row.getByText('HOME-3', { exact: true }).waitFor({ timeout: 5_000 });
  await row.getByText('Fitted', { exact: true }).waitFor({ timeout: 5_000 });
  await page.locator('[data-release-query]').getByText(`plan.release: ${COMING.id}`, { exact: true }).waitFor({ timeout: 5_000 });

  // The releases stand on an axis, each a mark that chooses it (scene planrel 2), and as cards,
  // their counts from the list itself: no release is read for them.
  const axis = page.locator('[data-release-axis]');
  await axis.locator(`[data-release-mark="${COMING.id}"]`).getByText(/· 1 of 1$/).waitFor({ timeout: 5_000 });
  assert.deepEqual(reads.filter((path) => path === `/releases/${LATER.id}` || path === `/releases/${SHIPPED.id}`), [], 'a release not chosen was read for its counts');
  assert.equal(await axis.locator('[data-release-mark]').count(), 3);
  assert.equal(await axis.locator(`[data-release-mark="${COMING.id}"]`).getAttribute('aria-pressed'), 'true');
  assert.equal(await page.locator('[data-release-card]').count(), 3);

  // A release has no trash: deleting it asks, naming it, and keeping it sends nothing (§5.9).
  await page.getByRole('button', { name: 'More for this release' }).click();
  await page.getByRole('menuitem', { name: 'Delete' }).click();
  const asked = page.getByRole('dialog', { name: 'Delete the release "1.1"?' });
  await asked.getByText('Its 1 entry is in no release afterwards.', { exact: false }).waitFor({ timeout: 5_000 });
  await asked.getByRole('button', { name: 'Keep it' }).click();
  await asked.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(wrote(written, 'DELETE', `/releases/${COMING.id}`), undefined, 'a release was deleted without being asked');

  // *Add entries* offers the entries of every collection not in the release yet (scene planrel 4).
  await page.locator('[data-release-add]').click();
  const adding = page.getByRole('dialog', { name: 'Add entries' });
  const offered = adding.getByRole('list', { name: 'Entries to add' });
  await offered.waitFor({ timeout: 5_000 });
  assert.equal(await offered.getByText(ITEMS[0].title, { exact: true }).count(), 0, 'an entry already in the release is offered again');
  await adding.getByLabel(ITEMS[1].title, { exact: true }).check();
  await adding.getByRole('button', { name: 'Add 1 to the release' }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/releases/${COMING.id}/entries`)?.body, { item_ids: [ITEMS[1].id] });

  // *Publish* stands on the chosen release (scene planrel 3): its open entries carried into the
  // next planned one, the notes written as a page.
  await chosen.getByRole('button', { name: 'Release', exact: true }).click();
  const dialog = page.getByRole('dialog');
  assert.equal(await dialog.getByLabel('Open entries go to').inputValue(), LATER.id);
  assert.deepEqual((await dialog.locator('footer.actions').getByRole('button').allInnerTexts()).map((each) => each.trim()), ['Cancel', 'Release'], 'the primary in the foot, Cancel beside it');
  assert.equal(await dialog.getByLabel('Write the release notes as a page').isChecked(), true);
  await dialog.getByRole('button', { name: 'Release', exact: true }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/releases/${COMING.id}:publish`)?.body, { write_notes: true, carry_to: LATER.id });

  // A shipped release, chosen from its card, says so and offers no publishing.
  await page.locator(`[data-release-card="${SHIPPED.id}"]`).click();
  const shipped = page.locator(`[data-release-chosen="${SHIPPED.id}"]`);
  await shipped.locator('.lozenge').getByText('Released', { exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await shipped.locator('[data-release-publish]').count(), 0, 'a released one offers publishing');
  await shipped.getByRole('link', { name: 'Read the release notes' }).waitFor({ timeout: 5_000 });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// Scene plansprints (UC-PLN-02 check 10): the hub's Sprints tab gathers every collection's sprints.
test('chromium: 1280 px — the hub\'s sprints: running with its burndown, planned against the velocity, closed committed against done', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await open(browser, `/hubs/${HUB.id}/plan/sprints`);
  t.after(close);

  // A running sprint is a card: its goal, days left, done of committed, entries, velocity and burndown.
  const card = page.locator(`[data-running-sprint="${RUNNING.id}"]`);
  await card.waitFor({ timeout: 15_000 });
  await card.getByText('Goal: The worktop is in').waitFor({ timeout: 5_000 });
  await card.locator('dd', { hasText: '5 of 8 Points' }).waitFor({ timeout: 5_000 });
  assert.equal(await card.locator('[data-burndown] polyline').count(), 2, 'the burndown draws ideal and remaining');
  assert.equal(await card.getByRole('img', { name: `Burndown of ${RUNNING.name}` }).count(), 1);

  // A planned one stands against the velocity of its collection, with the way into its backlog.
  const next = page.locator(`[data-planned-sprint="${NEXT.id}"]`);
  await next.locator('[data-load]').getByText('0 of ≈ 5 Points planned').waitFor({ timeout: 5_000 });
  await next.getByRole('button', { name: 'Plan in the backlog' }).click();
  await page.waitForURL(`**/collections/${COLLECTION.id}/backlog`, { timeout: 5_000 });
  await page.goBack();

  // The closed ones: committed against done, the velocity named once.
  const closed = page.locator(`[data-closed-sprint="${CLOSED.id}"]`);
  await closed.waitFor({ timeout: 10_000 });
  assert.deepEqual((await closed.locator('td').allInnerTexts()).slice(2, 4), ['8', '5']);
  assert.equal(await closed.getByRole('img', { name: '8 committed, 5 done' }).count(), 1);
  await page.getByText('Velocity: 5 Points a sprint').waitFor({ timeout: 5_000 });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});
