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
import { COLLECTION, HUB, ITEMS, signedIn, stub } from './fixture.mjs';

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
  outcome: null, done: false, completed_at: null, estimate: points(value), ...extra,
});
const DETAIL = {
  sprint: RUNNING,
  entries: [inSprint(ITEMS[0], 5, { done: true, completed_at: `${addDays(today, -1)}T15:00:00Z` }), inSprint(ITEMS[1], 3)],
  burndown: [
    { day: addDays(today, -3), remaining: 8, scope: 8, ideal: 8 },
    { day: addDays(today, -2), remaining: 8, scope: 8, ideal: 7.4 },
    { day: addDays(today, -1), remaining: 3, scope: 8, ideal: 6.8 },
  ],
};
const BACKLOG = { data: [{ item_id: ITEMS[3].id, title: ITEMS[3].title, estimate: null }, { item_id: ITEMS[4].id, title: ITEMS[4].title, estimate: points(2) }] };

const release = (n, name, state, on, extra = {}) => ({
  id: `01a0e2e0-0000-7000-8000-0000000e5c0${n}`, hub_id: HUB.id, name, description: '', release_on: on, state,
  released_at: null, notes_page_id: null, version: 1, ...extra,
});
const SHIPPED = release(1, '1.0', 'RELEASED', addDays(today, -20), { released_at: `${addDays(today, -20)}T12:00:00Z`, notes_page_id: '01a0e2e0-0000-7000-8000-0000000e5d01' });
const COMING = release(2, '1.1', 'PLANNED', addDays(today, 7), { description: 'The kitchen is usable.' });
const LATER = release(3, '1.2', 'PLANNED', addDays(today, 30));
const RELEASES = [SHIPPED, COMING, LATER];
const RELEASE_DETAIL = {
  ...COMING,
  entries: [{ item_id: ITEMS[0].id, title: ITEMS[0].title, type_name: 'Task', done: true, collection_id: COLLECTION.id, added_at: `${addDays(today, -5)}T09:00:00Z` }],
  done_count: 1,
  open_count: 0,
};

/** A screen of the planning area, with every write recorded and answered with what it makes. */
async function open(browser, at) {
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900, { timezoneId: 'UTC' });
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    const body = request.postDataJSON();
    if (method !== 'GET') written.push({ method, path, body });

    if (path === `/collections/${COLLECTION.id}/sprints` && method === 'GET') return route.fulfill({ json: { data: SPRINTS } });
    if (path === `/collections/${COLLECTION.id}/sprints` && method === 'POST') return route.fulfill({ status: 201, json: sprint(4, body.name, 'PLANNED', body.start_on, body.end_on, { goal: body.goal ?? '' }) });
    if (path === `/collections/${COLLECTION.id}/sprint-backlog`) return route.fulfill({ json: BACKLOG });
    if (path === `/collections/${COLLECTION.id}/velocity`) return route.fulfill({ json: { unit: 'POINTS', average: 5, sprints: [CLOSED] } });
    const one = SPRINTS.find((each) => path === `/sprints/${each.id}`);
    if (one && method === 'GET') return route.fulfill({ json: one === RUNNING ? DETAIL : { sprint: one, entries: [], burndown: [] } });
    if (path === `/sprints/${RUNNING.id}/entries` && method === 'POST') return route.fulfill({ json: RUNNING });
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
  return { page, failures, written, close: () => context.close(), unstubbed };
}

const wrote = (written, method, path) => written.find((each) => each.method === method && each.path === path);
const settle = (page) => page.waitForTimeout(300);

test('chromium: 1280 px — the running sprint, its burndown and velocity, a move, an estimate and a close', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await open(browser, `/collections/${COLLECTION.id}/sprints`);
  t.after(close);

  // It opens on the running sprint, not the first in the list, with its goal said.
  const chooser = page.getByLabel('Sprint', { exact: true });
  await chooser.waitFor({ timeout: 15_000 });
  assert.equal(await chooser.inputValue(), RUNNING.id);
  await page.getByText(`Goal: ${RUNNING.goal}`).waitFor({ timeout: 5_000 });

  // The burndown is a picture with a table of the same days (UC-PLN-07 check 5), and the velocity
  // is the server's average over the closed sprints.
  assert.equal(await page.getByRole('img', { name: /^Burndown: 3 points left/ }).count(), 1);
  const days = page.getByRole('table', { name: 'Burndown' }).locator('tbody tr');
  assert.equal(await days.count(), DETAIL.burndown.length);
  await page.getByRole('heading', { name: 'Velocity: 5 points a sprint' }).waitFor({ timeout: 5_000 });

  // An entry of the backlog picked and moved into the sprint.
  await page.getByLabel(`Pick ${ITEMS[3].title}`).check();
  await page.getByRole('button', { name: 'Into the sprint (1)' }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/sprints/${RUNNING.id}/entries`)?.body, { item_ids: [ITEMS[3].id] });

  // An estimate typed in hours, and one past the bound refused before anything is sent.
  const field = page.getByLabel(`Estimate of ${ITEMS[3].title}`);
  await field.fill('1.5h');
  await field.locator('xpath=ancestor::td').getByRole('button', { name: 'Save' }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'PUT', `/items/${ITEMS[3].id}/estimate`)?.body, { unit: 'HOURS', value: 1.5 });
  await field.fill('20000');
  await field.locator('xpath=ancestor::td').getByRole('button', { name: 'Save' }).click();
  await settle(page);
  assert.equal(written.filter((each) => each.path.endsWith('/estimate')).length, 1);

  // Closed, its unfinished entries carried into the next planned sprint the dialog offers first.
  await page.getByRole('button', { name: 'Close sprint' }).click();
  const dialog = page.getByRole('dialog');
  assert.equal(await dialog.getByLabel('Unfinished entries go to').inputValue(), NEXT.id);
  await dialog.getByRole('button', { name: 'Close sprint' }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/sprints/${RUNNING.id}:close`)?.body, { carry_to_sprint_id: NEXT.id });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — a sprint planned with a goal and a time box', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close, unstubbed } = await open(browser, `/collections/${COLLECTION.id}/sprints`);
  t.after(close);

  await page.getByRole('button', { name: 'Plan a sprint' }).click();
  const dialog = page.getByRole('dialog');
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
  const { page, failures, written, close, unstubbed } = await open(browser, `/hubs/${HUB.id}/releases`);
  t.after(close);

  // It opens on the next planned release, with what is done and open counted by the server.
  const chooser = page.getByLabel('Release', { exact: true });
  await chooser.waitFor({ timeout: 15_000 });
  assert.equal(await chooser.inputValue(), COMING.id);
  await page.getByText('1 done, 0 open').waitFor({ timeout: 5_000 });

  // The collection's entries not in the release yet are offered; one is taken in.
  const offered = page.getByRole('table', { name: 'Entries to add' });
  await offered.waitFor({ timeout: 5_000 });
  assert.equal(await offered.getByRole('link', { name: ITEMS[0].title }).count(), 0, 'an entry already in the release is offered again');
  await page.getByLabel(`Pick ${ITEMS[1].title}`).check();
  await page.getByRole('button', { name: 'Add 1 to the release' }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/releases/${COMING.id}/entries`)?.body, { item_ids: [ITEMS[1].id] });

  // Released, its open entries carried into the next planned one, the notes written as a page.
  await page.getByRole('button', { name: 'Release', exact: true }).click();
  const dialog = page.getByRole('dialog');
  assert.equal(await dialog.getByLabel('Open entries go to').inputValue(), LATER.id);
  assert.equal(await dialog.getByLabel('Write the release notes as a page').isChecked(), true);
  await dialog.getByRole('button', { name: 'Release', exact: true }).click();
  await settle(page);
  assert.deepEqual(wrote(written, 'POST', `/releases/${COMING.id}:publish`)?.body, { write_notes: true, carry_to: LATER.id });

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});
