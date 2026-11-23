// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Goals clicked through from an empty workspace (UC-GOL-01, UC-GOL-02, UC-GOL-04): the first
// period, the first goal in it, its page, a check-in, its title changed, and the first dashboard -
// with no console error and no refused or failed answer on the way.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { ACCOUNT, COLLECTION, HUB, OTHER } from './fixture.mjs';
import { ids, productWalk } from './productwalk.mjs';
import { chipsOf, describedBy, focused, pick } from './several.mjs';

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
    if (path === '/entry-types' && method === 'GET' && seed?.types) return { json: page(seed.types) };
    if (path === '/portfolios' && method === 'GET') return { json: page(seed?.portfolios ?? []) };
    const folio = /^\/portfolios\/([^/]+)(\/updates)?$/.exec(path);
    const portfolio = folio && (seed?.portfolios ?? []).find((each) => each.id === folio[1]);
    if (portfolio && !folio[2] && method === 'GET') return { json: portfolio };
    if (portfolio && folio[2] && method === 'GET') return { json: page(seed?.updates?.[portfolio.id] ?? []) };
    if (path.startsWith('/dashboards') && !path.slice('/dashboards'.length).startsWith('/') && method === 'GET') return { json: page(dashboards) };
    if (path === '/dashboards' && method === 'POST') {
      const made = { id: id(), hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], widgets: [], version: 1, ...body };
      dashboards.push(made);
      return { status: 201, json: made };
    }
    // An unsaved widget computed as it stands: 9 once it has a line of its own, 5 without.
    const drafted = /^\/dashboards\/([^/]+)\/figures:draft$/.exec(path);
    if (drafted && method === 'POST') {
      seed?.drafts?.push(body);
      return { json: { dashboard_id: drafted[1], computed_at: '2026-11-01T09:00:00Z', widgets: body.widgets.map((each) => ({ id: each.id ?? 'new', data: [{ group: null, period: null, count: each.query?.filter ? 9 : 5, value: null }], truncated: false, error: null, compare: null, remaining: null })) } };
    }
    const board = /^\/dashboards\/([^/]+)(\/figures|\/digest)?$/.exec(path);
    const dashboard = board && dashboards.find((each) => each.id === board[1]);
    if (dashboard && !board[2] && method === 'GET') return { json: dashboard };
    if (dashboard && !board[2] && method === 'PUT') {
      Object.assign(dashboard, { ...body, version: dashboard.version + 1, widgets: body.widgets.map((each, i) => ({ id: each.id ?? `01a0e2e0-0000-7000-8000-0000000b000${i}`, view_id: null, query: null, goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], ...each })) });
      seed?.puts?.push(body);
      return { json: dashboard };
    }
    if (path === '/items:aggregate' && method === 'POST' && seed?.preview) return { json: { data: [{ group: 'todo', period: null, count: 4, value: null }, { group: 'done', period: null, count: 2, value: null }] } };
    if (path === '/items:query' && method === 'POST' && seed?.preview) return { json: page([]) };
    if (dashboard && board[2] === '/figures') {
      return { json: { dashboard_id: dashboard.id, computed_at: '2026-11-01T09:00:00Z', widgets: dashboard.widgets.map((each) => ({ id: each.id, data: seed?.figures?.[each.id] ?? [], truncated: false, error: null, compare: null, remaining: null })) } };
    }
    if (dashboard && board[2] === '/digest' && method === 'GET') {
      return { json: { dashboard_id: dashboard.id, cadence: 'NONE', weekday: null, hour: null, time_zone: 'UTC', next_at: null, last_sent_at: null } };
    }
    return undefined;
  };
}

test('chromium: goals from nothing - a period, a goal, a check-in, a change, a dashboard - without an error', async (t) => {
  const browser = await sharedChromium(t);
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend() });
  t.after(walk.close);
  const { page } = walk;

  // With no period yet the goals open on their start page (scene emptygoals): the first goal is one
  // press, which begins the current quarter and asks for the goal (user-experience.md §4, UC-GOL-01 check 10).
  await page.goto(`${served.origin}/goals`);
  await page.getByRole('heading', { level: 1, name: 'Goals' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'Create a dashboard' }).count(), 1, 'the side way is missing');
  const now = new Date();
  const q = Math.floor(now.getUTCMonth() / 3) + 1;
  await page.getByRole('button', { name: 'Set the first goal' }).click();
  const goalDialog = page.getByRole('dialog', { name: 'New goal' });
  await goalDialog.waitFor({ timeout: 5_000 });
  const period = walk.written.find((each) => each.path === '/goal-periods');
  assert.equal(period?.body.name, `${now.getUTCFullYear()} Q${q}`);
  assert.equal(period.body.start_on, `${now.getUTCFullYear()}-${String((q - 1) * 3 + 1).padStart(2, '0')}-01`);
  await goalDialog.getByLabel('Title').fill('Ship the suite');
  await goalDialog.getByRole('button', { name: 'Set goal' }).click();
  await goalDialog.waitFor({ state: 'detached', timeout: 5_000 });
  await page.waitForURL(/\/goals\/[0-9a-f-]+$/, { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Ship the suite' }).first().waitFor({ timeout: 5_000 });
  // WB-1: the goal's head is every object's head - its key, the check-in as the next step.
  const head = page.locator('header.object-head');
  assert.match(await head.locator('.key').innerText(), /Goal/);
  assert.equal(await head.getByRole('button', { name: 'Check in' }).count(), 1, 'the check-in is not the head\'s next step');

  await page.getByRole('button', { name: 'Check in' }).first().click();
  const checkDialog = page.getByRole('dialog', { name: /Check in/ });
  await checkDialog.getByLabel('Confidence').fill('8');
  await checkDialog.getByLabel('Note').fill('First release out.');
  // The dialog ladder's foot (page-system §5.1): Cancel beside the primary, named with its verb.
  assert.deepEqual((await checkDialog.locator('footer.actions').getByRole('button').allInnerTexts()).map((each) => each.trim()), ['Cancel', 'Check in']);
  await checkDialog.getByRole('button', { name: 'Check in' }).click();
  await checkDialog.waitFor({ state: 'detached', timeout: 5_000 });
  // UC-GOL-02 check 6: the check-in made from the goal's page is in its history there.
  const history = page.getByRole('table', { name: 'Check-ins' });
  await history.getByRole('cell', { name: 'First release out.' }).waitFor({ timeout: 5_000 });
  assert.equal(await history.getByRole('cell', { name: '8 of 10' }).count(), 1, 'the history shows the confidence given');

  await page.getByRole('button', { name: 'Change', exact: true }).first().click();
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
    // The theme follows the media query on its change event, a task later.
    await page.waitForFunction((scheme) => document.documentElement.dataset.theme === scheme, colorScheme);
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
    const browser = await sharedChromium(t);
    const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { periods: [PERIOD], goals: GOALS } }), width, height: 900 });
    t.after(walk.close);
    const { page } = walk;

    await page.goto(`${served.origin}/goals`);
    // The goals' band (E-1, UC-GOL-01 check 10): its figures, the first one the headline.
    await page.locator('.band .figure').first().waitFor({ timeout: 10_000 });
    assert.deepEqual((await page.locator('.band .figure').allTextContents()).map((each) => each.trim()), ['2 goals', '1 needs a look', 'until Dec 31, 2026']);
    const results = page.getByRole('list', { name: 'Key results of Customers renew because the product helps them every day' });
    await results.getByText('82 % of 90 %').waitFor({ timeout: 5_000 });
    // Progress and confidence stand apart (UC-GOL-02 check 7), and the state is a word with its glyph.
    assert.deepEqual((await page.locator('[data-goal-confidence]').allTextContents()).map((each) => each.trim()), ['Confidence 6 of 10']);
    assert.equal(await page.locator('.goal .lozenge').filter({ hasText: 'At risk' }).locator('svg').count(), 1, 'the state has no glyph');
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
  const browser = await sharedChromium(t);
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

// user-experience.md §5.6 and §5.10: a widget's groups are named, its periods are the reader's
// dates, a bar chart has its axes and a line of one week is drawn as a dot.
test('chromium: a widget names its groups and periods, and every chart draws something', async (t) => {
  const browser = await sharedChromium(t);
  const widget = (n, title, chart, query) => ({
    id: `01a0e2e0-0000-7000-8000-00000000a31${n}`, title, chart, source: 'QUERY', view_id: null, query, goal_ids: [], compare: null, cumulative: false, x: (n - 1) * 6, y: 0, w: 6, h: 4,
  });
  const byType = widget(1, 'Open by type', 'BAR', { aggregate: { function: 'COUNT', group_by: 'entry_type' } });
  const perWeek = widget(2, 'Done per week', 'LINE', { aggregate: { function: 'COUNT', bucket: { field: 'completed_at', period: 'week' } } });
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a402', name: 'Weekly', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], version: 1, widgets: [byType, perWeek] };
  const type = (key, name) => ({ key, name, level: 'TASK', icon: null, color_token: 'blue', hub_id: null });
  // The types' names are Workflows' to answer, read only while it is on.
  const walk = await productWalk(browser, {
    modules: ['GOALS', 'WORKFLOWS'],
    answer: goalsBackend({ seed: {
      dashboards: [board],
      types: [type('bug', 'Bug'), type('change_request', 'Change request')],
      figures: {
        [byType.id]: [{ group: 'bug', period: null, count: 3, value: null }, { group: 'change_request', period: null, count: 5, value: null }],
        [perWeek.id]: [{ group: null, period: '2026-11-02', count: 8, value: null }],
      },
    } }),
  });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  // Bars lie down with the value at the bar (UC-GOL-04 check 19): the names stay legible, words not keys.
  const bars = page.locator('[data-chart="BAR"]');
  await bars.getByText('Change request', { exact: true }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await bars.locator('.name').allTextContents(), ['Bug', 'Change request']);
  assert.deepEqual(await bars.locator('.figure').allTextContents(), ['3', '5'], 'each bar carries its value');
  assert.equal(await page.getByText('change_request').count(), 0, 'no type key reaches the reader');
  // Every chart has its table as a toggle in the widget's menu (§8).
  assert.equal(await page.getByRole('table').count(), 0, 'the tables wait for their toggle');
  for (const title of ['Open by type', 'Done per week']) {
    await page.getByRole('button', { name: `More for ${title}` }).click();
    await page.getByRole('menuitem', { name: 'As table' }).click();
  }
  const types = page.getByRole('table', { name: 'Open by type' });
  await types.getByText('Bug', { exact: true }).waitFor({ timeout: 5_000 });
  await page.getByRole('table', { name: 'Done per week' }).getByText('Week of Nov 2, 2026', { exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByText('2026-11-02').count(), 0, 'no ISO day reaches the reader');
  await page.getByRole('button', { name: 'More for Done per week' }).click();
  await page.getByRole('menuitem', { name: 'As chart' }).click();
  const dot = await page.locator('[data-chart="LINE"] path.dots').boundingBox();
  assert.ok(dot && dot.width > 0 && dot.height > 0, `a line of one week draws its dot: ${JSON.stringify(dot)}`);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// UC-GOL-04 check 8: a widget grouped by a person names the person, beside a widget whose groups
// are no accounts - the server refuses a batch read naming one, as it does here.
test('chromium: a widget grouped by person names the people beside one grouped by state', async (t) => {
  const browser = await sharedChromium(t);
  const widget = (n, title, chart, group) => ({
    id: `01a0e2e0-0000-7000-8000-00000000a71${n}`, title, chart, source: 'QUERY', view_id: null, query: { aggregate: { function: 'COUNT', group_by: group } },
    goal_ids: [], compare: null, cumulative: false, x: (n - 1) * 6, y: 0, w: 6, h: 4,
  });
  const byPerson = widget(1, 'Open per person', 'TABLE', 'assignee_id');
  const byState = widget(2, 'Open and done', 'PIE', 'is_completed');
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a720', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], version: 1, widgets: [byPerson, byState] };
  const walk = await productWalk(browser, {
    modules: ['GOALS'],
    answer: goalsBackend({ seed: {
      dashboards: [board],
      figures: {
        [byPerson.id]: [{ group: ACCOUNT.id, period: null, count: 2, value: null }, { group: OTHER.id, period: null, count: 1, value: null }],
        [byState.id]: [{ group: 'false', period: null, count: 3, value: null }, { group: 'true', period: null, count: 1, value: null }],
      },
    } }),
  });
  t.after(walk.close);
  const { page } = walk;
  const asked = [];
  await page.route('**/api/v1/accounts:resolve', (route) => {
    const ids = route.request().postDataJSON()?.ids ?? [];
    asked.push(...ids);
    if (ids.some((id) => !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u.test(id))) return route.fulfill({ status: 400, json: { code: 'malformed_request', status: 400 } });
    return route.fulfill({ json: { data: [ACCOUNT, OTHER].filter((each) => ids.includes(each.id)) } });
  });

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  const table = page.getByRole('table', { name: 'Open per person' });
  await table.getByText(OTHER.display_name, { exact: true }).waitFor({ timeout: 10_000 });
  await table.getByText(ACCOUNT.display_name, { exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await page.getByText('Somebody in this workspace').count(), 0, 'every person is named');
  assert.deepEqual(asked.filter((id) => id === 'true' || id === 'false'), [], 'only people are asked for');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// UC-GOL-04 check 8: a widget grouped by status names each status as its workflow names it.
test('chromium: a widget grouped by status names the statuses as the workflows do', async (t) => {
  const browser = await sharedChromium(t);
  const byStatus = {
    id: '01a0e2e0-0000-7000-8000-00000000a731', title: 'By status', chart: 'BAR', source: 'QUERY', view_id: null, query: { aggregate: { function: 'COUNT', group_by: 'status' } },
    goal_ids: [], compare: null, cumulative: false, x: 0, y: 0, w: 6, h: 4,
  };
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a732', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], version: 1, widgets: [byStatus] };
  const shape = {
    statuses: [
      { key: 'new', name: 'New', category: 'TODO', color_token: 'slate' },
      { key: 'approved', name: 'Approved', category: 'IN_PROGRESS', color_token: 'blue' },
      { key: 'known_error', name: 'Known error', category: 'IN_PROGRESS', color_token: 'red' },
    ],
    initial: 'new', steps: [],
  };
  const goals = goalsBackend({ seed: {
    dashboards: [board],
    figures: { [byStatus.id]: [{ group: 'approved', period: null, count: 1, value: null }, { group: 'known_error', period: null, count: 2, value: null }, { group: 'in_progress', period: null, count: 4, value: null }] },
  } });
  const walk = await productWalk(browser, {
    modules: ['GOALS', 'WORKFLOWS'],
    answer: (method, path, query, body) => (path === '/workflows' && method === 'GET'
      ? { json: { data: [{ key: 'problems', system: false, name: 'Problems', published: shape, draft: null, published_at: '2026-10-30T09:00:00Z', version: 1 }], scheme: [] } }
      : goals(method, path, query, body)),
  });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  const bars = page.locator('[data-chart="BAR"]');
  await bars.getByText('Known error', { exact: true }).waitFor({ timeout: 10_000 });
  assert.deepEqual((await bars.locator('.name').allTextContents()).sort(), ['Approved', 'In progress', 'Known error']);
  assert.equal(await page.getByText('known_error').count(), 0, 'no status key reaches the reader');
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// A table longer than its widget scrolls inside the widget, so its head sticks to the widget's own
// top - not as far down as the app bar is tall, over the first row.
test('chromium: a table widget longer than its box starts with its head at the box\'s top and its first row in view', async (t) => {
  const browser = await sharedChromium(t);
  const long = {
    id: '01a0e2e0-0000-7000-8000-00000000a741', title: 'Open per group', chart: 'TABLE', source: 'QUERY', view_id: null, query: { aggregate: { function: 'COUNT', group_by: 'priority' } },
    goal_ids: [], compare: null, cumulative: false, x: 0, y: 0, w: 6, h: 3,
  };
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a742', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], version: 1, widgets: [long] };
  const groups = Array.from({ length: 14 }, (_, i) => ({ group: `group-${String(i + 1).padStart(2, '0')}`, period: null, count: 20 - i, value: null }));
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { dashboards: [board], figures: { [long.id]: groups } } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(`${served.origin}/dashboards/${board.id}`);
  const table = page.getByRole('table', { name: 'Open per group' });
  await table.locator('tbody tr').first().waitFor({ timeout: 10_000 });
  const measured = await page.locator('[data-chart="TABLE"]').evaluate((chart) => {
    const box = chart.closest('.content');
    const top = box.getBoundingClientRect();
    const head = chart.querySelector('thead th').getBoundingClientRect();
    const first = chart.querySelector('tbody tr').getBoundingClientRect();
    return { scrolls: box.scrollHeight > box.clientHeight, boxTop: top.top, headTop: head.top, headBottom: head.bottom, firstTop: first.top, firstBottom: first.bottom, boxBottom: top.bottom };
  });
  assert.ok(measured.scrolls, `the table is longer than its widget: ${JSON.stringify(measured)}`);
  assert.ok(Math.abs(measured.headTop - measured.boxTop) <= 2, `the head touches the box's top: ${JSON.stringify(measured)}`);
  assert.ok(measured.firstTop >= measured.headBottom - 1 && measured.firstBottom <= measured.boxBottom, `the first row is in view under the head: ${JSON.stringify(measured)}`);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

test('chromium: 375 px - a portfolio names its projects\' days in words and stacks them; an empty one says what fills it', async (t) => {
  const browser = await sharedChromium(t);
  const folio = (n, items) => ({ id: `01a0e2e0-0000-7000-8000-00000000a50${n}`, parent_id: null, name: `Programme ${n}`, description: '', owner_id: ACCOUNT.id, status: 'NONE', items, progress: null, version: 1 });
  const full = folio(1, [{ container_id: COLLECTION.id, kind: 'COLLECTION', name: 'Kitchen', archived: false, start_on: '2027-01-04', end_on: '2027-03-26', status: 'AT_RISK', entries: 12, done: 5, progress: 0.42 }]);
  const empty = folio(2, []);
  const update = { id: '01a0e2e0-0000-7000-8000-00000000a601', portfolio_id: full.id, author_id: ACCOUNT.id, status: 'AT_RISK', summary: 'Kitchen slips a week.', created_at: '2026-11-02T09:00:00Z' };
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { portfolios: [full, empty], updates: { [full.id]: [update] } } }), width: 375, height: 812 });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/portfolios/${full.id}`);
  await page.getByText('Jan 4, 2027', { exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByText('2027-01-04').count(), 0, 'no ISO day reaches the reader');
  // UC-GOL-03 check 8: each project's dates, health and progress, and the portfolio's updates.
  await page.getByText('Mar 26, 2027', { exact: true }).waitFor({ timeout: 5_000 });
  // On a phone the row's progress is the words beside its state; its bar stays in the row for wider screens.
  const kitchen = page.locator('progress[aria-label*="Kitchen"]');
  assert.equal(await kitchen.evaluate((each) => each.value), 42, 'the project\'s progress');
  await page.getByText('42 %', { exact: true }).first().waitFor({ timeout: 5_000 });
  const updates = page.getByRole('table', { name: 'Status updates' });
  await updates.getByText('Kitchen slips a week.').waitFor({ timeout: 5_000 });
  assert.ok(await page.getByText('At risk').count() >= 2, 'the project\'s health and the update\'s');
  // On a phone the head keeps its next step and its ⋯ - a portfolio has no status sheet to hold them.
  assert.ok(await page.getByRole('button', { name: 'Add project' }).first().isVisible(), 'the next step is gone on a phone');
  assert.ok(await page.getByRole('button', { name: 'More' }).first().isVisible(), 'the ⋯ is gone on a phone');
  // A Dialog M is full screen on a phone, its primary in the bar (page-system §5.1).
  await page.getByRole('button', { name: 'Add project' }).first().click();
  const adding = page.getByRole('dialog', { name: 'Add project' });
  assert.ok(await adding.getByRole('button', { name: 'Save' }).isVisible(), 'the primary is not in the bar on a phone');
  await adding.getByRole('button', { name: 'Close' }).first().click();
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
  const browser = await sharedChromium(t);
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

// The head's place line is said whole per case, in German too: a hub's name cannot be declined
// into "von {place}", so the workspace has its own sentence.
// UC-WRK-43 check 6 (Q-2): the dashboard's line names its keys in the reader's language, though
// its chips were cut before the German words arrived.
test('chromium: in German the dashboard\'s line names its keys in German', async (t) => {
  const browser = await sharedChromium(t);
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a742', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], filter: 'is: open due: <today', version: 1, widgets: [] };
  const backend = goalsBackend({ seed: { dashboards: [board] } });
  const walk = await productWalk(browser, {
    modules: ['GOALS'],
    answer: (method, path, ...rest) => (path === '/accounts/me' && method === 'GET' ? { json: { ...ACCOUNT, locale: 'de' } } : backend(method, path, ...rest)),
  });
  t.after(walk.close);
  const { page } = walk;
  // The German words arrive late, as on a slow line: the chips are cut in the source language first.
  await page.route(/\/assets\/de-[\w-]+\.js$/u, async (route) => { await new Promise((done) => setTimeout(done, 1_500)); await route.continue(); });

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  const bar = page.locator('[data-dashboard-filter]');
  await bar.getByText('Für alle Widgets').waitFor({ timeout: 10_000 });
  await bar.getByText('Fällig', { exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await bar.getByText('Due', { exact: true }).count(), 0, 'no key in the source language');
  assert.deepEqual(walk.errors(), []);
});

test('chromium: in German a goal\'s head names its place in a sentence that reads', async (t) => {
  const browser = await sharedChromium(t);
  const workspaceGoal = seeded(1, 'Kundinnen lösen ihr Anliegen selbst', { status: 'ON_TRACK' });
  const hubGoal = seeded(2, 'Das Haus ist bezogen', { hub_id: HUB.id, status: 'AT_RISK' });
  const backend = goalsBackend({ seed: { periods: [PERIOD], goals: [workspaceGoal, hubGoal] } });
  const walk = await productWalk(browser, {
    modules: ['GOALS'],
    answer: (method, path, ...rest) => (path === '/accounts/me' && method === 'GET' ? { json: { ...ACCOUNT, locale: 'de' } } : backend(method, path, ...rest)),
  });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/goals/${workspaceGoal.id}`);
  const head = page.locator('header.object-head');
  await head.getByText('Ein Ziel des Arbeitsbereichs').waitFor({ timeout: 10_000 });
  assert.equal(await head.getByText(/von Der/).count(), 0, 'the workspace is declined as a name');

  await page.goto(`${served.origin}/goals/${hubGoal.id}`);
  await head.getByText(`Ein Ziel von „${HUB.name}“`).waitFor({ timeout: 10_000 });
  assert.deepEqual(walk.errors(), []);
});

/** The head's status is the status button (§6.9, H-2): the head's height, its glyph and its ⌄. */
async function headStatusIsTheButtonShape(page, word) {
  const head = page.locator('header.object-head');
  const status = head.locator('.action-bar button.status').first();
  await status.waitFor({ timeout: 10_000 });
  assert.equal((await status.innerText()).trim(), word);
  assert.equal(await status.getAttribute('aria-haspopup'), 'menu', `${word} opens no menu`);
  assert.equal(await status.locator('.icon').count(), 2, `${word} carries no glyph beside its ⌄`);
  const [own, step] = [await status.boundingBox(), await head.locator('.action-bar .step').first().boundingBox()];
  assert.ok(own.height >= step.height - 1, `${word} is ${own.height} px high beside a ${step.height} px step`);
}

test('chromium: a goal\'s and a portfolio\'s head draw their status in the status button\'s shape', async (t) => {
  const browser = await sharedChromium(t);
  const goal = seeded(1, 'Customers renew', { status: 'AT_RISK' });
  const folio = { id: '01a0e2e0-0000-7000-8000-00000000a509', parent_id: null, name: 'Programme', description: '', owner_id: ACCOUNT.id, status: 'ON_TRACK', items: [], progress: null, version: 1 };
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { periods: [PERIOD], goals: [goal], portfolios: [folio] } }) });
  t.after(walk.close);
  const { page } = walk;
  await page.goto(`${served.origin}/goals/${goal.id}`);
  await headStatusIsTheButtonShape(page, 'At risk');
  await page.goto(`${served.origin}/portfolios/${folio.id}`);
  await headStatusIsTheButtonShape(page, 'On track');
  assert.deepEqual(walk.errors(), []);
});

// A project's row act says what it does for that project, and at 1280 nothing of the row is cut
// off - the act and its ⋯ included (finding of the family 3.1 review).
test('chromium: 1280 px - a portfolio\'s project rows change the project, and nothing of a row is clipped', async (t) => {
  const browser = await sharedChromium(t);
  const project = (name, status, progress) => ({ container_id: COLLECTION.id, kind: 'COLLECTION', name, archived: false, start_on: '2026-08-15', end_on: '2027-01-12', status, entries: 44, done: 9, progress });
  const folio = {
    id: '01a0e2e0-0000-7000-8000-00000000a50a', parent_id: null, name: 'Strategic projects 2026', description: '', owner_id: ACCOUNT.id, status: 'AT_RISK', progress: null, version: 1,
    items: [
      project('Product development', 'ON_TRACK', 0.2),
      { ...project('Neubau Logistikzentrum Hamburg-Billbrook', 'OFF_TRACK', 0.18), container_id: '01a0e2e0-0000-7000-8000-00000000c0f1' },
      { ...project('IT-Service-Desk', 'ON_TRACK', 0.13), container_id: '01a0e2e0-0000-7000-8000-00000000c0f2' },
      { ...project('Marketing & Kommunikation', 'AT_RISK', 0.12), container_id: '01a0e2e0-0000-7000-8000-00000000c0f3' },
    ],
  };
  const backend = goalsBackend({ seed: { portfolios: [folio] } });
  // In German, whose words run longest.
  const walk = await productWalk(browser, {
    modules: ['GOALS'], width: 1280, height: 900,
    answer: (method, path, ...rest) => (path === '/accounts/me' && method === 'GET' ? { json: { ...ACCOUNT, locale: 'de' } } : backend(method, path, ...rest)),
  });
  t.after(walk.close);
  const { page } = walk;
  await page.goto(`${served.origin}/portfolios/${folio.id}`);
  const verb = page.getByRole('button', { name: 'Product development ändern' });
  await verb.waitFor({ timeout: 10_000 });
  assert.equal((await verb.innerText()).trim(), 'Ändern');
  assert.equal(await page.getByRole('button', { name: 'Portfolio ändern' }).count(), 0, 'a project\'s row offers to change the portfolio');
  const clipped = await page.evaluate(() => {
    const out = [];
    for (const button of document.querySelectorAll('main table button')) {
      const box = button.getBoundingClientRect();
      if (box.width === 0) continue;
      for (let node = button.parentElement; node && node !== document.body; node = node.parentElement) {
        const style = getComputedStyle(node);
        if (style.overflowX === 'visible') continue;
        const frame = node.getBoundingClientRect();
        if (box.right > frame.right + 1 || box.left < frame.left - 1) out.push(button.getAttribute('aria-label') ?? button.textContent?.trim());
        break;
      }
    }
    return out;
  });
  assert.deepEqual(clipped, [], 'a row\'s act is clipped');
  // The table fits its frame: nothing of it scrolls sideways, the page neither.
  const table = await page.locator('main table').evaluate((node) => {
    const frame = node.parentElement.getBoundingClientRect();
    return { table: Math.round(node.getBoundingClientRect().width), frame: Math.round(frame.width), menus: [...node.querySelectorAll('button')].filter((button) => button.getBoundingClientRect().right > frame.right + 1).length };
  });
  assert.ok(table.table <= table.frame + 1 && table.menus === 0, `the projects table runs past its frame: ${JSON.stringify(table)}`);
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(wide <= 0, `the page scrolls sideways: ${wide}`);
});

// UC-GOL-04 check 20: the layout is edited by handle and corner, with the keyboard as well (WCAG
// 2.5.7), undone step by step, and the library offers the nine kinds previewed with one's own data.
test('chromium: a dashboard is laid out by its handles and the keyboard, undone, and grows from a library of nine kinds', async (t) => {
  const browser = await sharedChromium(t);
  const widget = (n, title, x) => ({
    id: `01a0e2e0-0000-7000-8000-00000000a61${n}`, title, chart: 'NUMBER', source: 'QUERY', view_id: null, query: { aggregate: { function: 'COUNT' } }, goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], x, y: 0, w: 3, h: 2,
  });
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a602', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], filter: '', version: 1, widgets: [widget(1, 'Open', 0), widget(2, 'Late', 3)] };
  const puts = [];
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { dashboards: [board], puts, preview: true, figures: { [board.widgets[0].id]: [{ group: null, period: null, count: 38, value: null }] } } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  await page.getByText('38', { exact: true }).waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'Edit' }).click();
  const band = page.locator('[data-edit-band]');
  await band.waitFor({ timeout: 5_000 });
  const handle = page.getByRole('button', { name: 'Move Open', exact: true });
  await handle.focus();
  await page.keyboard.press('ArrowDown');
  await page.getByText('Open: column 1, row 2, 3 wide, 2 high').waitFor({ state: 'attached', timeout: 5_000 });
  await page.keyboard.press('Shift+ArrowRight');
  await page.getByText('Open: column 1, row 2, 4 wide, 2 high').waitFor({ state: 'attached', timeout: 5_000 });
  await band.getByRole('button', { name: 'Undo' }).click();
  // The pointer drags by whole cells too.
  const box = await page.getByRole('button', { name: 'Resize Late', exact: true }).boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 400, box.y + box.height / 2, { steps: 5 });
  await page.mouse.up();
  await page.getByText(/^Late: column 4, row 1, [5-9] wide/).waitFor({ state: 'attached', timeout: 5_000 });

  await page.getByRole('button', { name: 'Add widget' }).click();
  const library = page.locator('[data-widget-library]');
  await library.waitFor({ timeout: 5_000 });
  assert.equal(await library.locator('.card').count(), 9, 'the library offers the nine kinds');
  await library.locator('[data-kind="BAR"] [data-chart="BAR"]').getByText('4', { exact: true }).waitFor({ timeout: 5_000 });
  assert.ok(walk.written.some((each) => each.path === '/items:aggregate' && each.body?.scope?.workspace === true), 'the previews count where the dashboard lives');
  await library.getByRole('button', { name: 'Lists' }).click();
  assert.equal(await library.locator('.card').count(), 2, 'a kind filter narrows the cards');
  // The editor's three tabs (UC-GOL-04 check 21): a text is written under Display, a target set under Look.
  await library.getByRole('button', { name: 'Text' }).click();
  await library.locator('[data-kind="TEXT"]').click();
  // The editor stands beside the grid, the others dimmed (scene wedit).
  const editor = page.locator('[data-widget-editor]');
  await editor.getByRole('tab', { name: 'Display' }).click();
  await editor.getByLabel('Text', { exact: true }).fill('This week **SA-7** first.');
  await editor.getByRole('button', { name: 'Apply' }).click();
  await editor.waitFor({ state: 'detached', timeout: 5_000 });
  await page.getByRole('button', { name: 'Change Open' }).click();
  assert.equal(await page.locator('article.widget.dimmed').count(), 2, 'the widgets not edited are dimmed');
  await editor.getByRole('tab', { name: 'Look' }).click();
  await editor.getByLabel('Target').fill('30');
  await editor.getByLabel('Warning from').fill('40');
  await editor.getByRole('button', { name: 'Apply' }).click();
  await editor.waitFor({ state: 'detached', timeout: 5_000 });

  await band.getByRole('button', { name: 'Done' }).click();
  await band.waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(puts.length, 1);
  const open = puts[0].widgets.find((each) => each.title === 'Open');
  assert.deepEqual([open.x, open.y, open.w, open.h], [0, 1, 3, 2], 'undo took the resize back, the move stayed');
  assert.ok(puts[0].widgets.find((each) => each.title === 'Late').w > 3, 'the corner widened the widget');
  assert.equal(puts[0].widgets.find((each) => each.chart === 'TEXT')?.text, 'This week **SA-7** first.');
  assert.deepEqual([open.target, open.warn_at], [30, 40], 'the figure carries its target and warning line');
  // The dashboard is read again after the save, so the figure draws its target and warning line.
  await page.getByText('Target ≤ 30 · warning from 40').waitFor({ timeout: 5_000 });
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// UC-GOL-04 check 22: *Als Liste öffnen* opens the counted collection with the widget's line - the
// dashboard's filter and its own - as a changed, unsaved view, and the line never enters the address.
test('chromium: a widget opens its collection\'s list with its line, which stays out of the address', async (t) => {
  const browser = await sharedChromium(t);
  const widget = {
    id: '01a0e2e0-0000-7000-8000-00000000a631', title: 'New this month', chart: 'NUMBER', source: 'QUERY', view_id: null,
    query: { scope: { container_id: COLLECTION.id }, filter: { field: 'created_at', op: 'GTE', value: '@today-P6W' }, aggregate: { function: 'COUNT' } },
    goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], x: 0, y: 0, w: 4, h: 2,
  };
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a632', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], filter: 'is: open', version: 1, widgets: [widget] };
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { dashboards: [board], figures: { [widget.id]: [{ group: null, period: null, count: 12, value: null }] } } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  await page.getByText('12', { exact: true }).waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'More for New this month' }).click();
  await page.getByRole('menuitem', { name: 'Open as list' }).click();
  await page.waitForURL(`**/collections/${COLLECTION.id}`, { timeout: 5_000 });
  assert.ok(!page.url().includes('open') && !page.url().includes('created'), 'the line stays out of the address');
  await page.getByRole('button', { name: 'View · 1 changed' }).waitFor({ timeout: 5_000 });
  const asked = walk.written.filter((each) => each.path === '/items:query').map((each) => JSON.stringify(each.body));
  assert.ok(asked.some((body) => body.includes('is_completed') && body.includes('@today-P6W')), 'the list asks for the widget\'s line');
  assert.deepEqual(walk.errors(), []);
});

// UC-GOL-04 check 21: the editor shows the dashboard's conditions as inherited chips, each released
// by a press, and names its groupings by the words a person reads, never a field's key.
test('chromium: the widget editor releases an inherited filter and names its fields in words', async (t) => {
  const browser = await sharedChromium(t);
  const widget = {
    id: '01a0e2e0-0000-7000-8000-00000000a641', title: 'By status', chart: 'BAR', source: 'QUERY', view_id: null,
    query: { scope: { workspace: true }, aggregate: { function: 'COUNT', group_by: 'status' } },
    goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], x: 0, y: 0, w: 6, h: 2,
  };
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a642', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], filter: 'is: open', version: 1, widgets: [widget] };
  const puts = [];
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { dashboards: [board], puts, figures: { [widget.id]: [{ group: 'todo', period: null, count: 3, value: null }] } } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  await page.getByRole('button', { name: 'Edit' }).click();
  await page.getByRole('button', { name: 'Change By status' }).click();
  const editor = page.locator('[data-widget-editor]');
  const chip = editor.locator('[data-inherited] button', { hasText: 'is: open' });
  assert.equal(await chip.getAttribute('aria-pressed'), 'true', 'the dashboard\'s condition is inherited');
  await chip.click();
  assert.equal(await chip.getAttribute('aria-pressed'), 'false');
  const grouping = await editor.getByLabel('Per', { exact: true }).locator('option').allTextContents();
  assert.ok(grouping.includes('Column') && !grouping.some((each) => /_|bucket/u.test(each)), `the groupings are words: ${grouping.join(', ')}`);
  await editor.getByRole('button', { name: 'Apply' }).click();
  await page.locator('[data-edit-band]').getByRole('button', { name: 'Done' }).click();
  await page.locator('[data-edit-band]').waitFor({ state: 'detached', timeout: 5_000 });
  assert.deepEqual(puts[0].widgets[0].released, ['is_completed']);
  assert.deepEqual(walk.errors(), []);
});

test('chromium 390 phone: the widget editor stands above the grid, the phone\'s width', async (t) => {
  const browser = await sharedChromium(t);
  const widget = {
    id: '01a0e2e0-0000-7000-8000-00000000a661', title: 'By status', chart: 'BAR', source: 'QUERY', view_id: null,
    query: { scope: { workspace: true }, aggregate: { function: 'COUNT', group_by: 'status' } },
    goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], x: 0, y: 0, w: 6, h: 2,
  };
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a662', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], filter: '', version: 1, widgets: [widget] };
  const walk = await productWalk(browser, { modules: ['GOALS'], width: 390, height: 844, isMobile: true, answer: goalsBackend({ seed: { dashboards: [board], puts: [], figures: { [widget.id]: [{ group: 'todo', period: null, count: 3, value: null }] } } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  await page.locator('main').getByRole('button', { name: 'More', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Edit' }).click();
  await page.getByRole('button', { name: 'Change By status' }).click();
  const editor = page.locator('[data-widget-editor]');
  await editor.waitFor({ timeout: 5_000 });
  await page.waitForTimeout(300);
  // A phone's browser widens its layout viewport to content that does not fit.
  assert.equal(await page.evaluate(() => window.innerWidth), 390, 'the editor widened the phone\'s page');
  const grid = await page.locator('.grid').boundingBox();
  const box = await editor.boundingBox();
  assert.ok(box && grid && box.y < grid.y, 'the editor stands above the grid');
  assert.deepEqual(walk.errors(), []);
});

// UC-GOL-04 check 23: the widget being edited stands large and live over the dimmed grid - its
// chart and line computed by the server as they are typed - and nothing is saved before Apply.
test('chromium: the widget editor draws the edited widget live, and its line is the chips of every list', async (t) => {
  const browser = await sharedChromium(t);
  const widget = {
    id: '01a0e2e0-0000-7000-8000-00000000a651', title: 'By status', chart: 'BAR', source: 'QUERY', view_id: null,
    query: { scope: { workspace: true }, aggregate: { function: 'COUNT' } },
    goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], x: 0, y: 0, w: 6, h: 2,
  };
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a652', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], filter: 'is: open', version: 1, widgets: [widget] };
  const puts = [];
  const drafts = [];
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { dashboards: [board], puts, drafts, figures: { [widget.id]: [{ group: null, period: null, count: 3, value: null }] } } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  await page.getByRole('button', { name: 'Edit' }).click();
  await page.getByRole('button', { name: 'Change By status' }).click();
  const preview = page.locator('[data-widget-preview]');
  await preview.getByText('Preview · live').waitFor({ timeout: 5_000 });
  await preview.getByText('5', { exact: true }).first().waitFor({ timeout: 5_000 });
  assert.equal(drafts[0]?.filter, 'is: open', 'the draft is computed under the dashboard\'s filter');

  const editor = page.locator('[data-widget-editor]');
  await editor.getByRole('tab', { name: 'Look' }).click();
  await editor.getByLabel('Drawn as').selectOption('NUMBER');
  await preview.locator('[data-chart="NUMBER"]').waitFor({ timeout: 5_000 });
  await editor.getByRole('tab', { name: 'Data' }).click();
  const line = editor.locator('[data-widget-line]').first().getByRole('combobox', { name: 'What is seen' });
  await line.click();
  await page.keyboard.type('title: milk ');
  await line.press('Enter');
  await preview.getByText('9', { exact: true }).first().waitFor({ timeout: 5_000 });
  assert.ok(drafts.some((each) => JSON.stringify(each.widgets[0].query?.filter ?? {}).includes('milk')), 'the line went into the draft');
  assert.equal(puts.length, 0, 'nothing is saved while editing');

  await editor.getByRole('button', { name: 'Apply' }).click();
  await page.locator('[data-edit-band]').getByRole('button', { name: 'Done' }).click();
  await page.locator('[data-edit-band]').waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(puts[0].widgets[0].chart, 'NUMBER');
  assert.match(JSON.stringify(puts[0].widgets[0].query.filter), /milk/u);
  assert.deepEqual(walk.errors(), []);
  assert.deepEqual(walk.unstubbed(), []);
});

// UC-GOL-04 check 22: a widget counting the whole workspace opens the search with its line, handed
// over in memory as the bar hands its words - never in the address.
test('chromium: a widget counting the workspace opens the search with its line, which stays out of the address', async (t) => {
  const browser = await sharedChromium(t);
  const widget = {
    id: '01a0e2e0-0000-7000-8000-00000000a661', title: 'Everything open', chart: 'NUMBER', source: 'QUERY', view_id: null,
    query: { scope: { workspace: true }, filter: { field: 'title', op: 'CONTAINS', value: 'milk' }, aggregate: { function: 'COUNT' } },
    goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], x: 0, y: 0, w: 4, h: 2,
  };
  const board = { id: '01a0e2e0-0000-7000-8000-00000000a662', name: 'Desk', hub_id: null, owner_id: ACCOUNT.id, sharing: 'PRIVATE', group_ids: [], filter: 'is: open', version: 1, widgets: [widget] };
  const walk = await productWalk(browser, { modules: ['GOALS'], answer: goalsBackend({ seed: { dashboards: [board], figures: { [widget.id]: [{ group: null, period: null, count: 12, value: null }] } } }) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/dashboards/${board.id}`);
  await page.getByText('12', { exact: true }).waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'More for Everything open' }).click();
  await page.getByRole('menuitem', { name: 'Open as list' }).click();
  await page.waitForURL(/\/search/u, { timeout: 5_000 });
  assert.ok(!page.url().includes('milk'), 'the line stays out of the address');
  // The line arrives whole as chips - the dashboard's condition and the widget's own.
  await page.getByRole('button', { name: 'Remove is: open' }).waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: 'Remove title: milk' }).waitFor({ timeout: 5_000 });
  for (let waited = 0; !walk.written.some((each) => each.path === '/search' && JSON.stringify(each.body).includes('milk')) && waited < 50; waited++) await page.waitForTimeout(100);
  assert.ok(walk.written.some((each) => each.path === '/search' && JSON.stringify(each.body).includes('milk')), 'the search asks for the widget\'s line');
  assert.deepEqual(walk.errors(), []);
});

// A dashboard shared with groups (design-system.md §4): the workspace's six groups are one field
// named "Groups", described by what is wrong while none is chosen. Opened by the keyboard, Space
// ticks, Escape closes the list and leaves the dialog open with focus on the field, the error goes,
// and the dashboard is created for the groups chosen. In English and in German.
const SIX_GROUPS = ['Board', 'Design', 'Finance', 'Platform', 'Sales', 'Support'].map((name, at) => ({
  id: `01a0e2e0-0000-7000-8000-00000000b0${String(at).padStart(2, '0')}`, name, description: null, members: [], version: 1,
}));
for (const locale of ['en', 'de']) {
  test(`chromium: a dashboard's six groups are one field, chosen by the keyboard (${locale})`, async (t) => {
    const browser = await sharedChromium(t);
    const backend = goalsBackend();
    const answer = (method, path, query, body) => {
      if (path === '/accounts/me' && method === 'GET') return { json: { ...ACCOUNT, locale } };
      if (path === '/groups' && method === 'GET') return { json: { data: SIX_GROUPS, page: { next_cursor: null, has_more: false } } };
      return backend(method, path, query, body);
    };
    const walk = await productWalk(browser, { modules: ['GOALS'], answer });
    t.after(walk.close);
    const { page } = walk;
    const words = locale === 'de'
      ? { tab: 'Dashboards', new: 'Neues Dashboard', name: 'Name', who: 'Wer es sieht', groups: 'Gruppen', needed: 'Wähle mindestens eine Gruppe.', create: 'Anlegen' }
      : { tab: 'Dashboards', new: 'New dashboard', name: 'Name', who: 'Who sees it', groups: 'Groups', needed: 'Choose at least one group.', create: 'Create' };

    await page.goto(`${served.origin}/goals?tab=dashboards`);
    if (locale === 'de') await page.waitForFunction(() => document.documentElement.lang === 'de', null, { timeout: 15_000 });
    await page.getByRole('button', { name: words.new }).first().click({ timeout: 15_000 });
    const dialog = page.getByRole('dialog', { name: words.new });
    await dialog.getByLabel(words.name).fill('Leadership');
    await dialog.getByLabel(words.who).selectOption('GROUPS');

    const field = dialog.getByRole('button', { name: new RegExp(`^${words.groups}`, 'u') });
    await field.waitFor({ timeout: 5_000 }).catch(() => {});
    assert.equal(await field.count(), 1, 'six groups are not one field');
    assert.equal(await field.getAttribute('aria-haspopup'), 'listbox');
    assert.equal(await describedBy(field), words.needed, 'the field is not described by what is wrong');

    await field.focus();
    await page.keyboard.press('Enter');
    const list = page.getByRole('listbox', { name: words.groups });
    await list.waitFor({ timeout: 5_000 });
    assert.equal(await focused(page), 'option "Board"', 'Enter did not land on the first group');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Space');
    await page.keyboard.press('Escape');
    await list.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await dialog.isVisible(), true, 'Escape closed the dialog, not only the list');
    assert.match(await focused(page), new RegExp(`^button "${words.groups} `, 'u'), 'closing the list left focus elsewhere');
    assert.equal(await describedBy(field), '', 'the error stays after a group is chosen');
    assert.deepEqual(await pick(page, field, ['Support']), ['Design', 'Support']);
    assert.deepEqual(await chipsOf(field), ['Design', 'Support']);

    await dialog.getByRole('button', { name: words.create }).click();
    await dialog.waitFor({ state: 'detached', timeout: 5_000 });
    await page.waitForURL(/\/dashboards\/[0-9a-f-]+$/u, { timeout: 5_000 });
    const made = walk.written.find((each) => each.method === 'POST' && each.path === '/dashboards');
    assert.equal(made?.body.sharing, 'GROUPS');
    assert.deepEqual(made?.body.group_ids, [SIX_GROUPS[1].id, SIX_GROUPS[5].id]);
    await page.getByRole('heading', { name: 'Leadership' }).first().waitFor({ timeout: 5_000 });
  });
}
