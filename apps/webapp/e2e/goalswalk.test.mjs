// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Goals clicked through from an empty workspace (UC-GOL-01, UC-GOL-02, UC-GOL-04): the first
// period, the first goal in it, its page, a check-in, its title changed, and the first dashboard -
// with no console error and no refused or failed answer on the way.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION } from './fixture.mjs';
import { ids, productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The goals module's routes, kept in memory as the server would keep them. */
function goalsBackend({ seed } = {}) {
  const id = ids('00000a');
  const periods = [...(seed?.periods ?? [])];
  const goals = [...(seed?.goals ?? [])];
  const checkIns = [];
  const dashboards = [...(seed?.dashboards ?? [])];
  const page = (data) => ({ data, page: { next_cursor: null, has_more: false } });
  return (method, path, query, body) => {
    if (path === '/goal-periods' && method === 'GET') return { json: page(periods) };
    if (path === '/goal-periods' && method === 'POST') {
      const made = { id: id(), state: 'OPEN', version: 1, ...body };
      periods.push(made);
      return { status: 201, json: made };
    }
    if (path === '/goals' && method === 'GET') {
      const period = query.get('period_id');
      return { json: page(goals.filter((each) => !period || each.period_id === period)) };
    }
    if (path === '/goals' && method === 'POST') {
      const made = {
        id: id(), hub_id: null, parent_id: null, description: '', owner_id: ACCOUNT.id, status: 'NONE', confidence: null, grade: null,
        key_results: [], entries: { total: 0, done: 0 }, child_count: 0, progress: 0, version: 1, linked_entries: [], check_ins: [], ...body,
      };
      goals.push(made);
      return { status: 201, json: made };
    }
    const one = /^\/goals\/([^/]+)(\/check-ins)?$/.exec(path);
    const goal = one && goals.find((each) => each.id === one[1]);
    if (goal && !one[2] && method === 'GET') return { json: goal };
    if (goal && !one[2] && method === 'PUT') {
      Object.assign(goal, body, { version: goal.version + 1 });
      return { json: goal };
    }
    if (goal && one[2] && method === 'GET') return { json: page(checkIns.filter((each) => each.goal_id === goal.id)) };
    if (goal && one[2] && method === 'POST') {
      const made = { id: id(), goal_id: goal.id, author_id: ACCOUNT.id, created_at: '2026-11-01T09:00:00Z', ...body };
      checkIns.unshift(made);
      Object.assign(goal, { status: body.status, confidence: body.confidence, check_ins: [made, ...goal.check_ins], version: goal.version + 1 });
      return { status: 201, json: goal };
    }
    if (path === '/portfolios' && method === 'GET') return { json: page(seed?.portfolios ?? []) };
    const folio = /^\/portfolios\/([^/]+)(\/updates)?$/.exec(path);
    const portfolio = folio && (seed?.portfolios ?? []).find((each) => each.id === folio[1]);
    if (portfolio && !folio[2] && method === 'GET') return { json: portfolio };
    if (portfolio && folio[2] && method === 'GET') return { json: page([]) };
    if (path.startsWith('/dashboards') && !path.slice('/dashboards'.length).startsWith('/') && method === 'GET') return { json: page(dashboards) };
    if (path === '/dashboards' && method === 'POST') {
      const made = { id: id(), hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], widgets: [], version: 1, ...body };
      dashboards.push(made);
      return { status: 201, json: made };
    }
    const board = /^\/dashboards\/([^/]+)(\/figures|\/digest)?$/.exec(path);
    const dashboard = board && dashboards.find((each) => each.id === board[1]);
    if (dashboard && !board[2] && method === 'GET') return { json: dashboard };
    if (dashboard && board[2] === '/figures') {
      return { json: { dashboard_id: dashboard.id, computed_at: '2026-11-01T09:00:00Z', widgets: dashboard.widgets.map((each) => ({ id: each.id, data: [], truncated: false, error: null, compare: null, remaining: null })) } };
    }
    if (dashboard && board[2] === '/digest' && method === 'GET') {
      return { json: { dashboard_id: dashboard.id, cadence: 'NONE', weekday: null, hour: null, time_zone: 'UTC', next_at: null, last_sent_at: null } };
    }
    return undefined;
  };
}

test('chromium: goals from nothing - a period, a goal, a check-in, a change, a dashboard - without an error', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend() });
  t.after(walk.close);
  const { page } = walk;

  // The current quarter is offered in one press where no period is set yet (user-experience.md §4).
  await page.goto(`${served.origin}/goals`);
  const now = new Date();
  const q = Math.floor(now.getUTCMonth() / 3) + 1;
  await page.getByRole('button', { name: `Start ${now.getUTCFullYear()} Q${q}` }).click();
  await page.getByRole('button', { name: 'New goal' }).first().waitFor({ timeout: 5_000 });
  const period = walk.written.find((each) => each.path === '/goal-periods');
  assert.equal(period.body.name, `${now.getUTCFullYear()} Q${q}`);
  assert.equal(period.body.start_on, `${now.getUTCFullYear()}-${String((q - 1) * 3 + 1).padStart(2, '0')}-01`);

  await page.getByRole('button', { name: 'New goal' }).first().click();
  const goalDialog = page.getByRole('dialog', { name: 'New goal' });
  await goalDialog.getByLabel('Title').fill('Ship the suite');
  await goalDialog.getByRole('button', { name: 'Set goal' }).click();
  await goalDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.waitForURL(/\/goals\/[0-9a-f-]+$/, { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Ship the suite' }).first().waitFor({ timeout: 5_000 });

  await page.getByRole('button', { name: 'Check in' }).first().click();
  const checkDialog = page.getByRole('dialog', { name: /Check in/ });
  await checkDialog.getByLabel('Confidence').fill('8');
  await checkDialog.getByLabel('Note').fill('First release out.');
  await checkDialog.getByRole('button', { name: 'Check in' }).click();
  await checkDialog.waitFor({ state: 'detached', timeout: 5_000 });
  // UC-GOL-02 check 6: the check-in made from the goal's page is in its history there.
  const history = page.getByRole('table', { name: 'Check-ins' });
  await history.getByRole('cell', { name: 'First release out.' }).waitFor({ timeout: 5_000 });
  assert.equal(await history.getByRole('cell', { name: '8 of 10' }).count(), 1, 'the history shows the confidence given');

  await page.getByRole('button', { name: 'Change' }).first().click();
  const changeDialog = page.getByRole('dialog', { name: 'Change' });
  await changeDialog.getByLabel('Title').fill('Ship the suite to everyone');
  await changeDialog.getByRole('button', { name: 'Save' }).click();
  await changeDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByRole('heading', { name: 'Ship the suite to everyone' }).first().waitFor({ timeout: 5_000 });

  // The goal's title in the list is a link in the design system's colour, in light and dark -
  // never the browser's blue - and so is every other link in the region.
  await page.goto(`${served.origin}/goals`);
  const titleLink = page.locator('main a', { hasText: 'Ship the suite to everyone' }).first();
  await titleLink.waitFor({ timeout: 5_000 });
  for (const colorScheme of ['light', 'dark']) {
    await page.emulateMedia({ colorScheme });
    const colours = await page.evaluate(() => {
      const probe = document.createElement('span');
      probe.style.color = 'var(--text-brand)';
      document.body.append(probe);
      const brand = getComputedStyle(probe).color;
      probe.remove();
      return { brand, links: [...document.querySelectorAll('main a')].map((each) => getComputedStyle(each).color) };
    });
    assert.ok(colours.links.length > 0, 'no link in the list');
    for (const colour of colours.links) {
      assert.ok(!['rgb(0, 0, 238)', 'rgb(85, 26, 139)'].includes(colour), `${colorScheme}: a link keeps the browser's colour ${colour}`);
    }
    assert.equal(await titleLink.evaluate((each) => getComputedStyle(each).color), colours.brand, `${colorScheme}: the goal's title is not in the brand colour`);
  }
  await page.emulateMedia({ colorScheme: 'light' });

  await page.goto(`${served.origin}/goals?tab=dashboards`);
  await page.getByRole('heading', { level: 1, name: 'Dashboards' }).waitFor({ timeout: 5_000 });
  await page.getByText('No dashboard yet.').waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'New dashboard' }).first().click();
  const boardDialog = page.getByRole('dialog', { name: 'New dashboard' });
  await boardDialog.getByLabel('Name').fill('Leadership');
  await boardDialog.getByRole('button', { name: 'Create' }).click();
  await boardDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.waitForURL(/\/dashboards\/[0-9a-f-]+$/, { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Leadership' }).first().waitFor({ timeout: 5_000 });
  // A dashboard without widgets says so and offers the first one (§5.5).
  await page.getByText('This dashboard has no widget yet.').waitFor({ timeout: 5_000 });
  assert.ok(await page.getByRole('button', { name: 'Add widget' }).first().isVisible());

  const wrote = walk.written.map((each) => `${each.method} ${each.path}`);
  for (const want of [/^POST \/goal-periods$/, /^POST \/goals$/, /^POST \/goals\/[^/]+\/check-ins$/, /^PUT \/goals\/[^/]+$/, /^POST \/dashboards$/]) {
    assert.ok(wrote.some((each) => want.test(each)), `nothing matched ${want}: ${JSON.stringify(wrote)}`);
  }
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

const PERIOD = { id: '01a0e2e0-0000-7000-8000-00000000a001', name: '2026 Q4', start_on: '2026-10-01', end_on: '2026-12-31', state: 'OPEN', version: 1 };
const result = (n, title, kind, current, target, unit, progress) => ({
  id: `01a0e2e0-0000-7000-8000-00000000a1${n}0`, title, kind, start: 0, target, current, unit, source: 'MANUAL', view_id: null, entry_id: null, progress, live: false,
});
const seeded = (n, title, extra) => ({
  id: `01a0e2e0-0000-7000-8000-00000000a20${n}`, period_id: PERIOD.id, hub_id: null, parent_id: null, title, description: '', owner_id: ACCOUNT.id,
  status: 'NONE', confidence: null, grade: null, key_results: [], entries: { total: 0, done: 0 }, child_count: 0, progress: null, version: 1, ...extra,
});
const GOALS = [
  seeded(1, 'Customers renew because the product helps them every day', {
    status: 'AT_RISK', confidence: 6, child_count: 1, progress: 0.45,
    key_results: [result(1, 'Renewal rate', 'PERCENT', 82, 90, '', 0.4), result(2, 'Support tickets per customer', 'NUMBER', 3, 1, 'tickets', 0.5)],
  }),
  seeded(2, 'Onboarding takes a day', { parent_id: '01a0e2e0-0000-7000-8000-00000000a201', status: 'ON_TRACK', progress: 1, key_results: [result(3, 'Guide published', 'BINARY', 1, 1, '', 1)] }),
];

for (const width of [1280, 375]) {
  test(`chromium: ${width} px - each goal is a card with its key results as bars and numbers, the head says what needs a look`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { periods: [PERIOD], goals: GOALS } }), width, height: 900 });
    t.after(walk.close);
    const { page } = walk;

    await page.goto(`${served.origin}/goals`);
    await page.getByText(/^2 goals · 1 needs a look · until Dec 31, 2026$/).waitFor({ timeout: 10_000 });
    const results = page.getByRole('list', { name: 'Key results of Customers renew because the product helps them every day' });
    await results.getByText('82 % of 90 %').waitFor({ timeout: 5_000 });
    await results.getByText('3 tickets of 1').waitFor({ timeout: 5_000 });
    await page.getByText('Reached', { exact: true }).waitFor({ timeout: 5_000 });
    assert.equal(await page.getByRole('progressbar').count(), 5, 'two goals and three key results');
    assert.equal(await page.getByRole('table').count(), 0);
    const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.ok(wide <= 0, `the page scrolls ${wide} px sideways`);
    assert.deepEqual(walk.errors(), []);
    assert.deepEqual(walk.unstubbed(), []);
  });
}

test('chromium: a widget with nothing to show names why, by its source', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const widget = (n, title, source, extra = {}) => ({
    id: `01a0e2e0-0000-7000-8000-00000000a30${n}`, title, chart: 'BAR', source, view_id: null, query: null, goal_ids: [], compare: null, cumulative: false, x: (n - 1) * 4, y: 0, w: 4, h: 3, ...extra,
  });
  const board = {
    id: '01a0e2e0-0000-7000-8000-00000000a401', name: 'Delivery', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], version: 1,
    widgets: [widget(1, 'Our goals', 'GOALS', { chart: 'PROGRESS' }), widget(2, 'Chosen goals', 'GOALS', { chart: 'PROGRESS', goal_ids: ['01a0e2e0-0000-7000-8000-00000000a999'] }), widget(3, 'Open by status', 'QUERY', { query: { aggregate: { count: true } } })],
  };
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { dashboards: [board] } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  await page.getByText('No goal is chosen for this widget yet; change it to choose some.').waitFor({ timeout: 10_000 });
  await page.getByText('None of its goals is one you may read, or they were deleted.').waitFor({ timeout: 5_000 });
  await page.getByText('No entry matches what this widget counts yet.').waitFor({ timeout: 5_000 });
  assert.equal(await page.getByText('Nothing to count.', { exact: true }).count(), 0);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

test('chromium: 375 px - a portfolio names its projects\' days in words and stacks them; an empty one says what fills it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const folio = (n, items) => ({ id: `01a0e2e0-0000-7000-8000-00000000a50${n}`, parent_id: null, name: `Programme ${n}`, description: '', owner_id: ACCOUNT.id, status: 'NONE', items, progress: null, version: 1 });
  const full = folio(1, [{ container_id: COLLECTION.id, kind: 'COLLECTION', name: 'Kitchen', archived: false, start_on: '2027-01-04', end_on: '2027-03-26', status: 'AT_RISK', entries: 12, done: 5, progress: 0.42 }]);
  const empty = folio(2, []);
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { portfolios: [full, empty] } }), width: 375, height: 812 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/portfolios/${full.id}`);
  await page.getByText('Jan 4, 2027', { exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByText('2027-01-04').count(), 0, 'no ISO day reaches the reader');
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 0, `the page scrolls ${wide} px sideways`);

  await page.goto(`${served.origin}/portfolios/${empty.id}`);
  await page.getByText('No project in this portfolio yet.').waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('table').count(), 0, 'an empty portfolio draws no empty table');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// UX-5: an address after /goals/ that names no goal - not an id, or one the server does not know -
// says so with a way back to Goals; a malformed id is not even asked for.
test('chromium: an address naming no goal is a not-found state with a way back', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const asked = [];
  const answer = goalsBackend();
  const walk = await productWalk(browser, {
    modules: ['GOALS'],
    answer: (method, path, ...rest) => {
      if (/^\/goals\/[^/]+$/u.test(path)) asked.push(path);
      if (path === '/goals/01a0e2e0-0000-7000-8000-00000000a9f0') return { status: 404, json: { type: 'about:blank', title: 'not_found', status: 404, code: 'not_found' } };
      return answer(method, path, ...rest);
    },
  });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/goals/dashboards`);
  await page.getByText('No goal at this address').waitFor({ timeout: 10_000 });
  assert.equal(await page.getByText('cannot read').count(), 0);
  assert.deepEqual(asked, [], 'a malformed id was asked for');

  await page.goto(`${served.origin}/goals/01a0e2e0-0000-7000-8000-00000000a9f0`);
  await page.getByText('No goal at this address').waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Go to Goals' }).click();
  await page.waitForFunction(() => location.pathname === '/goals', null, { timeout: 5_000 });
});
