// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The overview (F10-04; ADR-0063 decision 1): `/` stops listing the hubs the tree lists and
// becomes what is on the reader.
//
// Three things are walked here. **What it asks for**: every API request the first paint makes is
// counted, because "composed of reads that already exist" is only true if it can be counted — the
// number is in the pull request. **What it says when there is nothing**: each panel has a sentence
// of its own, so that "nothing is on you" cannot be mistaken for a screen that has not finished
// loading. And **where the line falls**: the server sorts by `due_at` and the client decides which
// of them are overdue, because that depends on the reader's time zone.
//
// Chromium only: this is a layout and a set of requests, neither of which has an engine-specific
// part; `engines.test.mjs` loads the bundle in all three.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { fallback, unstubbedSoFar } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e2-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Overview Walker', email: 'overview@example.invalid', status: 'ACTIVE', locale: 'en', time_zone: 'Europe/Berlin', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e2-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'House', order_key: 'a0', version: 1 };
const COLLECTION = { id: '01a0e2e2-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Kitchen', order_key: 'a0', version: 1 };

/** Yesterday and next week, from the walk's own clock: a fixture with a fixed date rots. */
const DAY = 24 * 60 * 60 * 1000;
const LATE = { id: '01a0e2e2-0000-7000-8000-000000000004', type: 'TASK', collection_id: COLLECTION.id, container_id: COLLECTION.id, parent_id: null, title: 'Call the plumber', due_at: new Date(Date.now() - DAY).toISOString(), due_date_only: false, completion: { is_completed: false }, order_key: 'a0', version: 1 };
const SOON = { id: '01a0e2e2-0000-7000-8000-000000000005', type: 'TASK', collection_id: COLLECTION.id, container_id: COLLECTION.id, parent_id: null, title: 'Return the drill', due_at: new Date(Date.now() + 7 * DAY).toISOString(), due_date_only: false, completion: { is_completed: false }, order_key: 'a1', version: 1 };

const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

/** What the walk decides per test: whether the reader has anything on them, or anything at all. */
let world = { mine: [LATE, SOON], jumble: [], hubs: [HUB] };
/** Every API path the page asked for, in order, so the first paint can be counted. */
let asked = [];
/** The body of the overview's search, kept so the filter it sends can be read. */
let searched;

async function stub(route) {
  const url = new URL(route.request().url());
  const path = url.pathname.replace(/^.*\/api\/v1/, '');
  asked.push(path + (url.search || ''));
  if (path === '/stream') return route.abort();
  if (path === '/sync:snapshot') {
    const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
  }
  if (path === '/sync:pull') return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  if (path === '/accounts/me') return route.fulfill({ json: ACCOUNT });
  if (path === '/meta/capabilities') {
    return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], query_fields: [{ field: 'assignee_id', operators: ['EQ'] }, { field: 'is_completed', operators: ['EQ'] }, { field: 'due_at', operators: ['IS_NULL'] }] } });
  }
  if (path === '/search') {
    searched = route.request().postDataJSON();
    return route.fulfill({ json: { ...PAGE, data: world.mine } });
  }
  if (path === '/jumble/entries') return route.fulfill({ json: { data: world.jumble, next_cursor: null } });
  if (path === '/containers' && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: world.hubs } });
  if (path === `/containers/${COLLECTION.id}`) return route.fulfill({ json: COLLECTION });
  if (path === '/containers') return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
  // The frame's own reads, in the shapes the API answers them, and a record of anything this
  // walk never prepared: a guess nobody notices is what `fallback` exists to prevent.
  return fallback(route, route.request(), path);
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  asked = [];
  await page.goto(`${served.origin}/`);
  return { page, failures, close: () => context.close() };
}

test('chromium: the overview says what is on the reader, from one read', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  world = { mine: [LATE, SOON], jumble: [], hubs: [HUB] };
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.getByRole('link', { name: LATE.title }).waitFor({ timeout: 15_000 });

  // What it asked for. Recorded rather than pinned to a number: the point is that the overview
  // adds one read — the search — to what the frame already makes, and that nothing here reads the
  // hubs a second time.
  const overviewReads = asked.filter((path) => path === '/search' || path === '/jumble/entries');
  assert.deepEqual(overviewReads, ['/search', '/jumble/entries'], `the overview asked ${JSON.stringify(asked)}`);
  assert.equal(asked.filter((path) => path.startsWith('/containers?type=HUB')).length, 1, `the hubs were read ${asked.filter((p) => p.startsWith('/containers?type=HUB')).length} times`);
  console.log(`F10-04: the first paint of the overview made ${asked.length} API requests: ${asked.join(', ')}`);

  // Mine, open, dated — and `@me` resolved on the server rather than by this client.
  assert.equal(searched.q, undefined, `the overview sent words: ${JSON.stringify(searched)}`);
  assert.deepEqual(searched.filter, {
    op: 'AND',
    nodes: [
      { op: 'EQ', field: 'assignee_id', value: '@me' },
      { op: 'EQ', field: 'is_completed', value: false },
      { op: 'NOT', nodes: [{ op: 'IS_NULL', field: 'due_at' }] },
    ],
  }, `the filter sent was ${JSON.stringify(searched.filter)}`);

  // The line between the two, drawn on the client because it depends on the reader's zone.
  const mine = page.getByRole('region', { name: 'What is on you' }).or(page.locator('section').filter({ hasText: 'What is on you' })).first();
  await mine.getByRole('heading', { name: /^Overdue/ }).waitFor({ timeout: 5_000 });
  assert.equal(await mine.getByRole('heading', { name: 'Overdue (1)' }).count(), 1, 'the overdue band does not count what is in it');
  assert.equal(await mine.getByRole('heading', { name: 'Due next' }).count(), 1, 'nothing is due next');

  // **Measured, not assumed** (issue 1022): the band's word is flush left - the same start as the
  // panel's own heading and as the rows' boxes. The text's own box, not the heading's: the heading
  // is as wide as the panel, and padding on it is what would move the word.
  const flush = await mine.evaluate((panel) => {
    const textX = (el) => { const range = document.createRange(); range.selectNodeContents(el); return Math.round(range.getBoundingClientRect().x); };
    const band = [...panel.querySelectorAll('h3')].map(textX);
    const row = panel.querySelector('[class*="task-row"] [class*="row"]') ?? panel.querySelector('[class*="task-row"]');
    return { band, head: textX(panel.querySelector('h2')), rowBox: Math.round(row.getBoundingClientRect().x) };
  });
  for (const x of flush.band) {
    assert.equal(x, flush.head, `a band's word is at ${x} and the panel's heading at ${flush.head}`);
    assert.equal(x, flush.rowBox, `a band's word is at ${x} and the rows under it at ${flush.rowBox}`);
  }

  // The three panels, and the two that are empty saying so rather than drawing nothing.
  assert.equal(await page.getByText('Nothing is waiting in the jumble.').count(), 1, 'the empty jumble is a blank');
  assert.equal(await page.getByText('What you open is listed here, on this device only.').count(), 1, 'the empty recents panel is a blank');

  // And the hubs the tree lists are not listed again here.
  const main = page.locator('main');
  assert.equal(await main.getByRole('link', { name: 'House' }).count(), 0, 'the overview still lists the hubs');

  assert.deepEqual(failures, []);
});

test('chromium: nothing on the reader is a sentence, and an empty workspace is one action', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  world = { mine: [], jumble: [], hubs: [HUB] };
  const empty = await open(browser);
  t.after(empty.close);
  await empty.page.getByText('Nothing assigned to you has a date on it.').waitFor({ timeout: 15_000 });
  assert.deepEqual(empty.failures, []);
  await empty.close();

  // A workspace with no hub at all: one sentence and one action, not three empty panels.
  world = { mine: [], jumble: [], hubs: [] };
  const fresh = await open(browser);
  t.after(fresh.close);
  await fresh.page.locator('main').getByText('No hubs yet. A hub holds the collections you work in.').waitFor({ timeout: 15_000 });
  assert.equal(await fresh.page.getByText('In the jumble').count(), 0, 'an empty workspace still draws the panels');
  assert.equal(await fresh.page.locator('main').getByRole('button', { name: 'Create hub' }).count() > 0, true, 'no way to start a workspace');
  assert.deepEqual(fresh.failures, []);
});

test('chromium: what was opened is remembered on this device, and only here', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  world = { mine: [], jumble: [], hubs: [HUB] };
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.getByText('What you open is listed here, on this device only.').waitFor({ timeout: 15_000 });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  // Waited on the list rather than on a heading: what this proves is that opening a level is what
  // writes the row, and the row is written when the level is named.
  await page.waitForFunction(
    (id) => Object.keys(localStorage).some((key) => key.startsWith('hubtask.recent.') && (localStorage.getItem(key) ?? '').includes(id)),
    COLLECTION.id,
    { timeout: 10_000 },
  );
  await page.goto(`${served.origin}/`);

  const recent = page.locator('section').filter({ hasText: 'What you had open' });
  await recent.getByRole('link', { name: COLLECTION.name }).waitFor({ timeout: 10_000 });

  // It is the account's, kept on this device, and it is written under a key nothing else reads.
  const kept = await page.evaluate(() => Object.keys(localStorage).filter((key) => key.startsWith('hubtask.recent.')));
  assert.deepEqual(kept, [`hubtask.recent.${ACCOUNT.id}`], `the list is kept under ${JSON.stringify(kept)}`);

  assert.deepEqual(failures, []);
});
