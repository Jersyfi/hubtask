// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The products band and a product's home (user-experience.md §2.1, §2.2, §2.6): one row per
// product switched on, in one order, under *Products*; none where none is on; the home lists the
// hubs where the product is on and says what to do where there are none; on a phone *More* opens
// the drawer whose first band is the products. Walked at desk and phone width, light and dark.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PERMISSIONS, fallback } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Band Walker', email: 'band@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e0-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'Product development and delivery for the whole company', order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };
const board = (n, title, at) => ({ id: `01a0e2e0-0000-7000-8000-0000000001${n}`, hub_id: HUB.id, title, element_count: 3, trashed_at: null, created_by: ACCOUNT.id, created_at: at, updated_by: ACCOUNT.id, updated_at: at, version: 1 });
const agent = (n, name, status, owner) => ({ id: `01a0e2e0-0000-7000-8000-0000000002${n}`, account_id: null, name, purpose: 'Sorts new requests into the right queue', kind: 'TICKET_TRIAGE', owner_id: owner, status,
  risk_class: 'MINIMAL', scopes: [], model_id: null, policy: { reversible: 'APPROVAL', irreversible: 'APPROVAL' }, budget: { tokens_per_day: 200000, actions_per_hour: 120, concurrent_runs: 2 },
  min_accuracy: 0, needs_approval: false, approved_by: null, approved_at: null, suspended_by: null, suspended_at: null, created_at: '2026-11-01T08:00:00Z', version: 1 });
const AGENTS = [agent('01', 'Old sorter', 'RETIRED', ACCOUNT.id), agent('02', 'Request sorter', 'ACTIVE', ACCOUNT.id), agent('03', 'Duplicate finder', 'SUSPENDED', '01a0e2e0-0000-7000-8000-0000000009ff')];
const HELD = { id: '01a0e2e0-0000-7000-8000-000000000211', agent_id: AGENTS[1].id, use_case: 'UpdateItem', class: 'REVERSIBLE', input: {}, status: 'PENDING', decided_by: null, decided_at: null, code: '', created_at: '2026-11-03T08:00:00Z' };
const job = (n, status, at) => ({ id: `01a0e2e0-0000-7000-8000-00000000022${n}`, agent_id: AGENTS[1].id, target_type: 'ENTRY', target_id: '01a0e2e0-0000-7000-8000-000000000299', status, code: '', proposals: 1, tokens: 120, started_by: null, started_at: at, ended_at: null });
const JOBS = [job(1, 'FAILED', '2026-11-02T08:00:00Z'), job(2, 'DONE', '2026-11-03T08:00:00Z')];
const pageOf = (n, title, at, more = {}) => ({ id: `01a0e2e0-0000-7000-8000-0000000003${n}`, hub_id: HUB.id, parent_id: null, title, icon: null, position: n, template: false, content_language: null, owner_id: null, verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null, trashed_at: null, created_by: ACCOUNT.id, created_at: at, updated_by: ACCOUNT.id, updated_at: at, version: 1, ...more });
const PAGES = [
  pageOf('01', 'Onboarding guide', '2026-10-02T09:00:00Z'),
  pageOf('02', 'How we release, step by step, for every team that ships to customers', '2026-10-31T09:00:00Z'),
  pageOf('03', 'Meeting notes template', '2026-11-01T09:00:00Z', { template: true }),
  pageOf('04', 'Support rota', '2026-09-01T09:00:00Z', { owner_id: ACCOUNT.id, due_for_review: true }),
];
const HEALTH = { total: 42, completeness: 0.9, correctness: 0.8, stale: 2, orphans: 1, duplicates: 0, without_owner: 3, review_overdue: 4, classes: [{ class: 'laptop', total: 12, completeness: 1, stale: 0, orphans: 0, duplicates: 0 }, { class: 'server', total: 30, completeness: 0.9, stale: 2, orphans: 1, duplicates: 0 }], problems: [{ ci_id: '01a0e2e0-0000-7000-8000-000000000401', class: 'server', kinds: ['STALE'] }, { ci_id: '01a0e2e0-0000-7000-8000-000000000402', class: 'server', kinds: ['NO_OWNER'] }], stale_relations: 0, services: [], sources: [] };
const DESK = { id: '01a0e2e0-0000-7000-8000-000000000501', type: 'COLLECTION', parent_id: HUB.id, name: 'IT help desk for every office and every remote worker', handle: 'it', key_prefix: 'INC', order_key: 'a0', version: 1, structure: { settings: { entry_types: { TASK: ['incident'] } } } };
const SCRUM = { id: '01a0e2e0-0000-7000-8000-000000000502', type: 'COLLECTION', parent_id: HUB.id, name: 'Sprint work', order_key: 'a1', version: 1, structure: { settings: { entry_types: { TASK: ['story'] } } } };
const TYPES = [
  { key: 'incident', level: 'TASK', name: 'Incident', system: false, position: 1, archived: false, form: { channels: [{ channel: 'PORTAL' }] } },
  { key: 'story', level: 'TASK', name: 'Story', system: false, position: 2, archived: false },
];
const ticket = (n, title) => ({ id: `01a0e2e0-0000-7000-8000-0000000006${n}`, type: 'TASK', entry_type: 'incident', collection_id: DESK.id, title, number: Number(n), created_by: ACCOUNT.id, custom_fields: { priority: `P${Number(n) + 1}` }, completion: { mode: 'BINARY', is_completed: false }, version: 1 });
/** Each ticket's goals: the first running with an hour left, the second breached, the third paused. */
const goalOf = (standing, due, breached) => ({ goal: 'RESOLUTION', kind: 'SLA', policy_key: 'standard', assignee_group: null, standing, target_seconds: 7200, armed_at: new Date(Date.now() - 3_600_000).toISOString(), due_at: due, at_risk_at: null, met_at: null, breached_at: breached });
const GOALS = {
  '01': [goalOf('RUNNING', new Date(Date.now() + 3_600_000 + 20_000).toISOString(), null)],
  '02': [goalOf('BREACHED', null, new Date(Date.now() - 18 * 60_000).toISOString())],
  '03': [goalOf('PAUSED', null, null)],
};
/** Each queue's answer, by its narrowing: what the server would hold for this reader. */
const QUEUES = {
  mine: [ticket('01', 'The printer on the third floor prints every page twice and nobody knows why')],
  unassigned: [ticket('02', 'VPN drops at noon'), ticket('03', 'New laptop for Ana')],
  breaching: [],
  open: [ticket('01', 'The printer on the third floor prints every page twice and nobody knows why'), ticket('02', 'VPN drops at noon'), ticket('03', 'New laptop for Ana')],
};
function queueOf(body) {
  const nodes = body?.filter?.nodes ?? [];
  const narrowing = nodes[2];
  if (!narrowing) return 'open';
  if (narrowing.field === 'sla_standing') return 'breaching';
  return narrowing.op === 'IS_NULL' ? 'unassigned' : 'mine';
}
const queries = [];
const releaseOf = (n, name, on, state = 'PLANNED') => ({ id: `01a0e2e0-0000-7000-8000-0000000007${n}`, hub_id: HUB.id, name, description: '', release_on: on, state, released_at: null, notes_page_id: null, version: 1 });
const RELEASES = [releaseOf('01', 'Winter release with the new planning board and the reworked reports', '2026-12-10'), releaseOf('02', 'Autumn fixes', '2026-11-20'), releaseOf('03', 'Summer', '2026-07-01', 'RELEASED')];
const PORTFOLIOS = [{ id: '01a0e2e0-0000-7000-8000-000000000811', parent_id: null, name: 'Wachstum im Norden', description: '', owner_id: null, status: 'ON_TRACK', items: [], progress: 0, version: 1 }];
const DASHBOARDS = [{ id: '01a0e2e0-0000-7000-8000-000000000821', hub_id: null, owner_id: ACCOUNT.id, name: 'Betrieb am Morgen', sharing: 'PRIVATE', widgets: [], version: 1 }];
const SPRINTS = [{ id: '01a0e2e0-0000-7000-8000-000000000831', collection_id: SCRUM.id, name: 'Sprint Kranich', goal: '', start_on: '2026-11-02', end_on: '2026-11-13', state: 'ACTIVE', unit: 'POINTS', committed_value: 0, committed_count: 0, completed_value: 0, completed_count: 0, started_at: null, closed_at: null, version: 1 }];
const GOAL_ROWS = [{ id: '01a0e2e0-0000-7000-8000-000000000801', period_id: 'p1', hub_id: null, parent_id: null, title: 'Mehr Kundinnen gewinnen', description: '', owner_id: null, status: 'ON_TRACK', confidence: null, grade: null, key_results: [], entries: [], child_count: 0, progress: 0, version: 1 }];
const CIS = [{ id: '01a0e2e0-0000-7000-8000-000000000901', hub_id: HUB.id, class: 'firewall', name: 'FortiGate 60F', stage: 'LIVE', status: 'OK', attributes: {}, provenance: {}, protection: null, quantity: 1, created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-01T00:00:00Z', version: 1 }];
const BOARDS = [board('01', 'Retro of the autumn release with every team that took part in it', '2026-10-30T09:00:00Z'), board('02', 'Kick-off', '2026-10-01T09:00:00Z')];

/** The API at the network edge, with the workspace's modules and the hub's own. */
function stubFor({ modulesOn, hubModules }) {
  return async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    if (path.endsWith('/api/v1/stream')) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
    if (path.endsWith('/api/v1/sync:snapshot')) {
      const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
    }
    if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
    if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: PERMISSIONS.OWNER, modules_on: modulesOn } });
    }
    if (path.endsWith(`/api/v1/containers/${HUB.id}/modules`)) return route.fulfill({ json: { modules_on: hubModules ?? modulesOn } });
    if (path.endsWith('/api/v1/agents')) return route.fulfill({ json: { data: AGENTS } });
    if (path.endsWith('/api/v1/agent-requests')) return route.fulfill({ json: { data: [HELD] } });
    if (path.endsWith(`/api/v1/agents/${AGENTS[1].id}/jobs`)) return route.fulfill({ json: { data: JOBS } });
    if (/\/api\/v1\/agents\/[^/]+\/jobs$/.test(path)) return route.fulfill({ json: { data: [] } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: BOARDS } });
    if (path.endsWith('/api/v1/goals')) return route.fulfill({ json: { data: GOAL_ROWS } });
    if (path.endsWith('/api/v1/portfolios')) return route.fulfill({ json: { data: PORTFOLIOS } });
    if (path.endsWith('/api/v1/dashboards')) return route.fulfill({ json: { data: DASHBOARDS } });
    if (path.endsWith(`/api/v1/collections/${SCRUM.id}/sprints`)) return route.fulfill({ json: { data: SPRINTS, runs_sprints: true } });
    if (/\/api\/v1\/collections\/[^/]+\/sprints$/.test(path)) return route.fulfill({ json: { data: [], runs_sprints: false } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/cis`)) { ciLimits.push(url.searchParams.get('limit')); return route.fulfill({ json: { data: CIS, next_after: null } }); }
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/pages`) && !url.searchParams.has('templates')) return route.fulfill({ json: { data: PAGES } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/cmdb-health`)) return route.fulfill({ json: HEALTH });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/releases`)) return route.fulfill({ json: { data: RELEASES } });
    if (path.endsWith('/api/v1/pages:search')) return route.fulfill({ json: { data: [{ id: PAGES[0].id, hub_id: HUB.id, parent_id: null, title: 'Onboarding guide', icon: null, excerpt: 'Your first «release» week', rank: 1, updated_at: PAGES[0].updated_at }] } });
    if (path.endsWith('/api/v1/search')) return route.fulfill({ json: { data: [], page: { next_cursor: null, has_more: false } } });
    if (path.endsWith('/api/v1/entry-types')) return route.fulfill({ json: { data: TYPES } });
    const sla = /\/api\/v1\/items\/01a0e2e0-0000-7000-8000-0000000006(\d\d)\/sla$/.exec(path);
    if (sla) return route.fulfill({ json: { data: GOALS[sla[1]] ?? [] } });
    if (path.endsWith('/api/v1/accounts:resolve')) return route.fulfill({ json: { data: [{ id: ACCOUNT.id, display_name: ACCOUNT.display_name }] } });
    if (path.endsWith('/api/v1/sla-policies')) return route.fulfill({ json: { data: [] } });
    if (path.endsWith('/api/v1/items:query')) {
      const body = route.request().postDataJSON();
      queries.push(body);
      const rows = QUEUES[queueOf(body)];
      return route.fulfill({ json: { data: rows, groups: [], page: { next_cursor: null, has_more: false }, total: rows.length } });
    }
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('parent_id') === HUB.id) return route.fulfill({ json: { ...PAGE, data: [DESK, SCRUM] } });
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [] } });
    if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
    return fallback(route, route.request(), path);
  };
}

const ciLimits = [];
const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { width, scheme = 'light', path = '/', reducedMotion = 'no-preference', ...modules }) {
  const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme, reducedMotion });
  await context.route('**/api/v1/**', stubFor(modules));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}${path}`);
  await page.getByRole('button', { name: ACCOUNT.display_name }).waitFor({ timeout: 15_000 });
  return { page, failures, close: () => context.close() };
}

async function noSidewaysScroll(page, label) {
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, `${label}: the page scrolls sideways (${scroll.width} of ${scroll.viewport})`);
}

for (const scheme of ['light', 'dark']) {
  test(`chromium 1280 px, ${scheme}: the products band, in one order, and a product's home with its hubs`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width: 1280, scheme, modulesOn: ['TIME', 'SERVICE_DESK', 'KNOWLEDGE'] });
    t.after(close);
    const tree = page.getByRole('navigation', { name: 'Workspace', exact: true });
    await tree.getByRole('treeitem', { name: HUB.name }).waitFor({ timeout: 10_000 });
    const rows = (await tree.getByRole('treeitem').allTextContents()).map((row) => row.trim());
    // The places (the approvals where the desk is on), then the products in Module.Keys() order
    // whatever order the manifest answers, then the hubs.
    // Today's row counts what waits for the reader's yes beside its name.
    assert.deepEqual(rows, ['Today  1 waiting', 'Search', 'Inbox', 'Waiting for my yes', 'Pages', 'Service desk', 'Time', HUB.name], JSON.stringify(rows));
    assert.equal(await page.locator('aside.sidenav').getByText('Products', { exact: true }).count(), 1, 'the band has no caption');
    // The account menu carries none of them.
    await page.getByRole('button', { name: ACCOUNT.display_name }).click();
    const items = (await page.getByRole('menu').getByRole('menuitem').allTextContents()).map((item) => item.trim());
    for (const product of ['Pages', 'Service desk', 'Time', 'My notes', 'Waiting for my yes', 'Administration']) assert.ok(!items.includes(product), `${product} is behind the avatar`);
    await page.keyboard.press('Escape');

    await tree.getByRole('treeitem', { name: 'Pages' }).click();
    await page.waitForFunction(() => location.pathname === '/pages', null, { timeout: 5_000 });
    assert.equal((await page.getByRole('heading', { level: 1 }).textContent()).trim(), 'Pages');
    assert.equal(await tree.getByRole('treeitem', { name: 'Pages' }).getAttribute('aria-current'), 'page', 'the row is not current on its home');
    const tabs = page.getByRole('main').getByRole('navigation', { name: 'Pages' });
    assert.equal(await tabs.getByRole('link', { name: 'Spaces' }).getAttribute('aria-current'), 'page');
    // The hub, one line with the whole name as its tooltip, opening the hub's pages.
    const hub = page.getByRole('main').getByRole('link', { name: HUB.name });
    await hub.waitFor({ timeout: 5_000 });
    assert.equal(await hub.getAttribute('href'), `/hubs/${HUB.id}/pages`);
    await noSidewaysScroll(page, `1280 ${scheme}`);
    // *My notes* is the home's second tab, at its own address, and the row stays current.
    await tabs.getByRole('link', { name: 'My notes' }).click();
    await page.waitForFunction(() => location.pathname === '/notes', null, { timeout: 5_000 });
    assert.equal(await tree.getByRole('treeitem', { name: 'Pages' }).getAttribute('aria-current'), 'page');
    assert.deepEqual(failures, []);
  });
}

test('chromium 1280 px: a product no hub uses says so and offers a hub; with nothing on there is no band', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const narrowed = await open(browser, { width: 1280, path: '/whiteboards', modulesOn: ['WHITEBOARD'], hubModules: [] });
  t.after(narrowed.close);
  await narrowed.page.getByText('No hub uses Whiteboards yet').waitFor({ timeout: 10_000 });
  assert.equal(await narrowed.page.getByRole('main').getByRole('button', { name: 'Create hub' }).count(), 1, 'no way forward from the empty home');

  const none = await open(browser, { width: 1280, path: '/whiteboards', modulesOn: [] });
  t.after(none.close);
  const tree = none.page.getByRole('navigation', { name: 'Workspace', exact: true });
  await tree.getByRole('treeitem', { name: HUB.name }).waitFor({ timeout: 10_000 });
  assert.equal(await none.page.locator('aside.sidenav').getByText('Products', { exact: true }).count(), 0, 'a household with nothing on sees the caption');
  // Reached by a link while off: the neutral screen with its way forward (§2.8).
  await none.page.getByRole('button', { name: 'Switch Whiteboards on' }).waitFor({ timeout: 5_000 });
  assert.equal(await none.page.getByText('Products', { exact: true }).count() > 0, true, 'the trail is not the products');
  assert.deepEqual([...narrowed.failures, ...none.failures], []);
});

for (const scheme of ['light', 'dark']) {
  test(`chromium 375 px, ${scheme}: More opens the drawer with the products first`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width: 375, scheme, modulesOn: ['SERVICE_DESK', 'ASSETS'] });
    t.after(close);
    // Today with *More in Hubtask* fits the phone.
    await page.getByRole('region', { name: 'More in Hubtask' }).waitFor({ timeout: 10_000 });
    await noSidewaysScroll(page, `Today 375 ${scheme}`);
    const bar = page.getByRole('navigation', { name: 'Sections' });
    await bar.getByRole('button', { name: 'More' }).click();
    const drawer = page.locator('dialog[open]');
    await drawer.waitFor({ timeout: 5_000 });
    await drawer.getByRole('treeitem', { name: HUB.name }).waitFor({ timeout: 5_000 });
    const rows = (await drawer.getByRole('treeitem').allTextContents()).map((row) => row.trim());
    // What the bar does not carry: the approvals, then the products, the hub, the administration
    // and the keeping band.
    assert.deepEqual(rows, ['Waiting for my yes', 'Service desk', 'Assets', HUB.name, 'Administration', 'Archive', 'Trash'], JSON.stringify(rows));
    await drawer.getByRole('treeitem', { name: 'Assets' }).click();
    await page.waitForFunction(() => location.pathname === '/assets', null, { timeout: 5_000 });
    assert.equal(await bar.getByRole('button', { name: 'More' }).getAttribute('aria-current'), 'page', 'More is not current on a place only the drawer lists');
    await page.getByRole('main').getByRole('link', { name: HUB.name }).waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `375 ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// user-experience.md §3.3: *More in Hubtask* offers the products that are off to whoever may switch
// one on, each with what it does; *Try it* opens its home with the switch; it can be hidden for good.
test('chromium 1280 px: Today offers the products that are off, and hides them for good', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, modulesOn: ['KNOWLEDGE'] });
  t.after(close);
  const more = page.getByRole('region', { name: 'More in Hubtask' });
  await more.waitFor({ timeout: 10_000 });
  const names = (await more.locator('.offer-name').allTextContents()).map((name) => name.trim());
  assert.deepEqual(names, ['Plans', 'Whiteboards', 'Service desk', 'Assets', 'Time', 'Goals', 'Agents'], JSON.stringify(names));
  await more.getByRole('listitem').filter({ hasText: 'Whiteboards' }).getByRole('button', { name: 'Try it' }).click();
  await page.waitForFunction(() => location.pathname === '/whiteboards', null, { timeout: 5_000 });
  await page.getByRole('button', { name: 'Switch Whiteboards on' }).waitFor({ timeout: 5_000 });
  await page.goto(`${served.origin}/`);
  await more.getByRole('button', { name: 'Hide More in Hubtask for good' }).click();
  await more.waitFor({ state: 'detached', timeout: 5_000 });
  await page.reload();
  await page.getByRole('button', { name: ACCOUNT.display_name }).waitFor({ timeout: 15_000 });
  await page.waitForTimeout(500);
  assert.equal(await more.count(), 0, 'More in Hubtask came back after it was hidden');
  assert.deepEqual(failures, []);
});

// §2.2: the whiteboards' home lists the boards themselves, recent first, as cards.
for (const [width, scheme] of [[1280, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: the whiteboards' home shows the boards, the last changed first`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/whiteboards', modulesOn: ['WHITEBOARD'] });
    t.after(close);
    const recent = page.getByRole('region', { name: 'Recently changed boards' });
    await recent.waitFor({ timeout: 10_000 });
    const titles = await recent.getByRole('link').evaluateAll((links) => links.map((link) => link.querySelector('[title]')?.getAttribute('title')));
    assert.deepEqual(titles, [BOARDS[0].title, BOARDS[1].title]);
    assert.equal(await recent.getByRole('link').first().getAttribute('href'), `/whiteboards/${BOARDS[0].id}`);
    await noSidewaysScroll(page, `whiteboards ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// §7.8: the agents' home is their cards, live first and a retired one last, each counting the acts
// waiting on it; the runs are a tab, the newest first.
test('chromium 1280 px: the agents\' home shows the agents as cards, their held acts and runs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/agents', modulesOn: ['AGENTS'] });
  t.after(close);
  const cards = page.locator('[data-agent-card]');
  await cards.first().waitFor({ timeout: 10_000 });
  assert.deepEqual(await cards.locator('h2').allTextContents(), ['Request sorter', 'Duplicate finder', 'Old sorter']);
  assert.match((await cards.first().textContent()) ?? '', /Live/u);
  await cards.first().getByRole('button', { name: '1 waiting' }).click();
  await page.waitForURL(new RegExp(`/agents\\?tab=requests&agent=${AGENTS[1].id}$`));
  await page.getByText('Waiting on Request sorter').waitFor({ timeout: 10_000 });
  await page.getByRole('link', { name: 'Runs' }).click();
  const runs = page.getByRole('list', { name: 'Runs' });
  await runs.waitFor({ timeout: 10_000 });
  assert.match((await runs.textContent()) ?? '', /Done[\s\S]*Failed/u, 'the newest run first');
  assert.deepEqual(failures, []);
});

// §2.2: the pages' home lists the reader's pages to review and the recently changed ones, the
// space's content only; the assets' home opens with the inventory's health across hubs.
for (const [width, scheme] of [[1280, 'dark'], [375, 'light']]) {
  test(`chromium ${width} px, ${scheme}: the pages' home shows what waits for review and what changed`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/pages', modulesOn: ['KNOWLEDGE'] });
    t.after(close);
    const review = page.getByRole('region', { name: 'Waiting for your review' });
    await review.waitFor({ timeout: 10_000 });
    assert.deepEqual(await review.locator('.name').allTextContents(), ['Support rota']);
    const recent = page.getByRole('region', { name: 'Recently changed pages' });
    assert.deepEqual(await recent.locator('.name').allTextContents(), [PAGES[1].title, 'Onboarding guide', 'Support rota']);
    assert.equal(await recent.getByRole('link').first().getAttribute('href'), `/pages/${PAGES[1].id}`);
    await noSidewaysScroll(page, `pages ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });

  test(`chromium ${width} px, ${scheme}: the assets' home opens with the inventory's health`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/assets', modulesOn: ['ASSETS'] });
    t.after(close);
    const inventory = page.getByRole('region', { name: 'Inventory across your hubs' });
    await inventory.waitFor({ timeout: 10_000 });
    const figures = await inventory.locator('dl > div').evaluateAll((rows) => rows.map((row) => `${row.querySelector('dt')?.textContent}=${row.querySelector('dd')?.textContent}`));
    assert.deepEqual(figures, ['Configuration items=42', 'Need attention=2', 'Review overdue=4']);
    // The inventory itself across hubs, by class, the most held first, each saying where (§2.2).
    const byClass = page.getByRole('region', { name: 'By class' });
    await byClass.waitFor({ timeout: 5_000 });
    assert.deepEqual(await byClass.locator('.name').allTextContents(), ['Server · 30', 'Laptop · 12']);
    assert.equal(await byClass.locator('.purpose').first().innerText(), `30 in ${HUB.name}`);
    assert.equal(await byClass.getByRole('link').first().getAttribute('href'), `/hubs/${HUB.id}/assets`);
    // What makes one need attention is said beside the figure, not left to guess.
    await inventory.getByText('A configuration item needs attention when it is incomplete').waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `assets ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// §2.2, §2.4, §7.4: the desk's home and a hub's Desk tab draw the handler's queues across the
// hub's desk collections only, with their counts, what breaks first on top (UC-SVC-03).
for (const [width, scheme] of [[1280, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: the desk's home and the hub's Desk tab draw the queues`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    queries.length = 0;
    const { page, failures, close } = await open(browser, { width, scheme, path: '/desk', modulesOn: ['WORKFLOWS', 'SERVICE_DESK'] });
    t.after(close);
    const queues = page.getByRole('radiogroup', { name: 'Queues' });
    await queues.getByRole('radio', { name: 'Mine (1)' }).waitFor({ timeout: 10_000 });
    assert.deepEqual((await queues.getByRole('radio').allTextContents()).map((each) => each.trim()), ['Mine (1)', 'Unassigned (2)', 'About to breach (0)', 'All open (3)']);
    // Only the desk is asked, by what breaks first, counted.
    assert.ok(queries.length > 0 && queries.every((body) => JSON.stringify(body.filter.nodes[0]) === JSON.stringify({ op: 'IN', field: 'collection_id', value: [DESK.id] }) && body.sort[0].field === 'sla_due' && body.count === 'exact'), JSON.stringify(queries));
    // §7.4: the open requests as a table, the SLA said in words beside its bar, never by colour alone.
    const panel = page.getByRole('region', { name: 'All open', exact: true });
    await queues.getByRole('radio', { name: 'All open (3)' }).click();
    const table = panel.getByRole('table', { name: 'Requests in All open' });
    await table.locator('[data-sla="breached"]').waitFor({ timeout: 10_000 });
    const headings = await table.locator('thead th').allTextContents();
    assert.deepEqual(headings.map((each) => each.trim()), ['Key', 'Request', 'Requester', 'Status', 'Priority', 'SLA', 'Assignee']);
    const cells = await table.locator('[data-queue-row]').evaluateAll((rows) => rows.map((row) => [...row.querySelectorAll('td')].map((cell) => cell.textContent.trim().replace(/\s+/g, ' '))));
    assert.deepEqual(cells.map((row) => [row[0], row[2], row[4], row[5]]), [
      ['INC-1', ACCOUNT.display_name, 'P2', '1 h 0 min left'], ['INC-2', ACCOUNT.display_name, 'P3', 'Breached 18 min ago'], ['INC-3', ACCOUNT.display_name, 'P4', 'Paused'],
    ], JSON.stringify(cells));
    assert.equal(await page.locator('[data-desk-figure]').textContent(), '3 open · 2 unassigned');
    // All seven columns fit beside the tree: none hides behind the edge fade.
    const frame = await table.evaluate((node) => { const scroller = node.closest('.scroll'); return [scroller.scrollWidth, scroller.clientWidth]; });
    assert.ok(frame[0] <= frame[1] + 1, `the queue table scrolls sideways: ${frame}`);
    // One strip of tabs only: the queues are a segmented choice under the home's tabs.
    assert.equal(await page.getByRole('tablist').count(), 0, 'a second tab strip');
    await queues.getByRole('radio', { name: 'Unassigned (2)' }).click();
    await page.getByRole('region', { name: 'Unassigned', exact: true }).getByRole('link', { name: 'VPN drops at noon' }).waitFor({ timeout: 5_000 });
    await queues.getByRole('radio', { name: 'About to breach (0)' }).click();
    await page.getByRole('region', { name: 'About to breach', exact: true }).getByText('Nothing waits in this queue').waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `desk home ${width} ${scheme}`);

    await page.goto(`${served.origin}/hubs/${HUB.id}/desk`);
    const strip = page.getByRole('navigation', { name: HUB.name });
    await strip.getByRole('link', { name: 'Desk' }).waitFor({ timeout: 10_000 });
    assert.equal(await strip.getByRole('link', { name: 'Desk' }).getAttribute('aria-current'), 'page');
    const desks = page.getByRole('region', { name: 'Desks' });
    await desks.waitFor({ timeout: 10_000 });
    assert.deepEqual(await desks.locator('.name').allTextContents(), [DESK.name]);
    await page.getByRole('radio', { name: 'All open (3)' }).waitFor({ timeout: 10_000 });
    await noSidewaysScroll(page, `hub desk ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// A handler with nothing of their own opens on the first queue that holds a request, not on an
// empty *Mine* while requests wait unassigned.
test('chromium 1280 px: the desk opens on the first queue that holds a request', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const mine = QUEUES.mine;
  QUEUES.mine = [];
  t.after(() => { QUEUES.mine = mine; });
  const { page, failures, close } = await open(browser, { width: 1280, path: '/desk', modulesOn: ['WORKFLOWS', 'SERVICE_DESK'] });
  t.after(close);
  const queues = page.getByRole('radiogroup', { name: 'Queues' });
  await queues.getByRole('radio', { name: 'Unassigned (2)', checked: true }).waitFor({ timeout: 10_000 });
  await page.getByRole('region', { name: 'Unassigned', exact: true }).getByRole('link', { name: 'VPN drops at noon' }).waitFor({ timeout: 5_000 });
  await queues.getByRole('radio', { name: 'Mine (0)' }).click();
  await page.getByRole('region', { name: 'Mine', exact: true }).getByText('Nothing waits in this queue').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

// §2.2: the plans' home lists the releases still to go out, the soonest first.
test('chromium 375 px, dark: the plans\' home lists the releases to come', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 375, scheme: 'dark', path: '/plans', modulesOn: ['PLANNING'] });
  t.after(close);
  const upcoming = page.getByRole('region', { name: 'Releases to come' });
  await upcoming.waitFor({ timeout: 10_000 });
  assert.deepEqual(await upcoming.locator('.name').allTextContents(), ['Autumn fixes', RELEASES[0].name]);
  assert.equal(await upcoming.getByRole('link').first().getAttribute('href'), `/hubs/${HUB.id}/releases`);
  await noSidewaysScroll(page, 'plans 375 dark');
  assert.deepEqual(failures, []);
});

// §2.2, §7.9: the plans' home lists the sprints running in the hubs' collections, with where and
// how long, and a hub's row names its running sprint.
test('chromium 1280 px: the plans\' home lists the sprints running and says each hub\'s', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/plans', modulesOn: ['PLANNING'] });
  t.after(close);
  const running = page.getByRole('region', { name: 'Sprints running' });
  await running.waitFor({ timeout: 10_000 });
  assert.deepEqual(await running.locator('.name').allTextContents(), ['Sprint Kranich']);
  assert.match(await running.locator('.purpose').first().innerText(), new RegExp(`${HUB.name} › ${SCRUM.name} · (Its last day has passed|\\d+ days? left)`));
  assert.equal(await running.getByRole('link').first().getAttribute('href'), `/collections/${SCRUM.id}/sprints`);
  await page.waitForFunction((name) => [...document.querySelectorAll('main a')].some((a) => a.textContent?.includes(name) && a.textContent.includes('Sprint Kranich ·')), HUB.name, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

// §7.9: a hub's overview says one line per product on there - the running sprint and the next
// release, the pages changed this week, the inventory - each leading to where it is about.
test('chromium 1280 px: a hub\'s overview says one line per product on there', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: `/hubs/${HUB.id}`, modulesOn: ['PLANNING', 'KNOWLEDGE', 'ASSETS', 'WHITEBOARD'] });
  t.after(close);
  const lines = page.getByRole('list', { name: 'What the products say here' });
  await lines.locator('[data-product-line="assets"]').waitFor({ timeout: 10_000 });
  await lines.locator('[data-product-line="plan"]').waitFor({ timeout: 10_000 });
  assert.deepEqual(await lines.locator('[data-product-line]').evaluateAll((rows) => rows.map((row) => row.getAttribute('data-product-line'))), ['plan', 'pages', 'assets']);
  assert.match(await lines.locator('[data-product-line="plan"]').innerText(), /Plan\s+Sprint Kranich · (Its last day has passed|\d+ days? left) · Next: Autumn fixes on /);
  assert.equal(await lines.locator('[data-product-line="plan"] a').getAttribute('href'), `/collections/${SCRUM.id}/sprints`);
  // Every page of the fixture changed before this week began, the template included.
  assert.match(await lines.locator('[data-product-line="pages"]').innerText(), /Pages\s+No page changed this week/);
  assert.equal(await lines.locator('[data-product-line="pages"] a').getAttribute('href'), `/hubs/${HUB.id}/pages`);
  assert.match(await lines.locator('[data-product-line="assets"]').innerText(), /42 configuration items · 2 need attention/);
  await noSidewaysScroll(page, 'hub overview');
  assert.deepEqual(failures, []);
});

// §5.1/§5.2: the plans' head carries its primary create act, as every product home does: *Plan a
// sprint* leads to a collection's sprints, the other collections in its menu.
test('chromium 1280 px: the plans\' home leads its create act to a collection\'s sprints', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/plans', modulesOn: ['PLANNING'] });
  t.after(close);
  const plan = page.getByRole('button', { name: 'Plan a sprint' });
  await plan.waitFor({ timeout: 10_000 });
  await page.getByRole('button', { name: 'In another collection' }).click();
  await page.getByRole('menuitem', { name: `${HUB.name} › ${SCRUM.name}` }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await plan.click();
  await page.waitForURL(`**/collections/${DESK.id}/sprints?new=1`, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

// §5.3: one search for every kind: entries, pages, boards and places, grouped and filtered by kind;
// with no entry matching, the screen does not say nothing matches while other kinds do.
for (const [width, scheme] of [[1280, 'dark'], [375, 'light']]) {
  test(`chromium ${width} px, ${scheme}: the search finds pages, boards and places beside entries`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, { width, scheme, path: '/search', modulesOn: ['KNOWLEDGE', 'WHITEBOARD'] });
    t.after(close);
    await page.getByRole('searchbox', { name: 'Search everything you can see' }).fill('release');
    const pagesFound = page.getByRole('region', { name: 'Pages' });
    await pagesFound.waitFor({ timeout: 10_000 });
    assert.equal(await pagesFound.getByRole('link', { name: 'Onboarding guide' }).getAttribute('href'), `/pages/${PAGES[0].id}`);
    const boardsFound = page.getByRole('region', { name: 'Boards' });
    await boardsFound.waitFor({ timeout: 10_000 });
    assert.deepEqual(await boardsFound.locator('.name').allTextContents(), [BOARDS[0].title]);
    assert.equal(await page.getByText('Nothing matches those words.').count(), 0);
    const kinds = page.getByRole('group', { name: 'What to show' });
    assert.deepEqual(await kinds.getByRole('button').allTextContents(), ['All', 'Entries', 'Pages', 'Boards', 'Hubs and collections']);
    await kinds.getByRole('button', { name: 'Boards' }).click();
    await pagesFound.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await boardsFound.count(), 1);
    await kinds.getByRole('button', { name: 'Hubs and collections' }).click();
    await page.getByText('Nothing of this kind matches those words.').waitFor({ timeout: 5_000 });
    await page.getByRole('searchbox', { name: 'Search everything you can see' }).fill('product development');
    const places = page.getByRole('region', { name: 'Hubs and collections' });
    await places.waitFor({ timeout: 5_000 });
    assert.equal(await places.getByRole('link').first().getAttribute('href'), `/hubs/${HUB.id}`);
    await noSidewaysScroll(page, `search ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// Motion (user-experience.md §6): a home's rows arrive with the entrance role, staggered, by
// opacity and transform only - and nothing moves where the system or the product asks for less.
// Counted at `Element.animate`, because an arrival is over before a poll could see it.
test('chromium 1280 px: a home\'s rows arrive, and nothing moves under reduced motion', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const watch = () => {
    window.__arrivals = [];
    const animate = Element.prototype.animate;
    Element.prototype.animate = function (keyframes, options) {
      if (this.classList.contains('row')) window.__arrivals.push({ properties: Object.keys(keyframes[0] ?? {}).sort(), delay: options?.delay ?? 0, duration: options?.duration ?? 0 });
      return animate.call(this, keyframes, options);
    };
  };

  const first = await (async () => {
    const opened = await open(browser, { width: 1280, path: '/pages', modulesOn: ['KNOWLEDGE'] });
    t.after(opened.close);
    await opened.page.addInitScript(watch);
    await opened.page.reload();
    await opened.page.getByRole('main').getByRole('link', { name: HUB.name }).waitFor({ timeout: 10_000 });
    const seen = await opened.page.evaluate(() => window.__arrivals ?? []);
    assert.deepEqual(opened.failures, []);
    return seen;
  })();
  assert.ok(first.length >= 1, `no row arrived: ${JSON.stringify(first)}`);
  for (const arrival of first) {
    assert.deepEqual(arrival.properties, ['opacity', 'transform'], 'an arrival moves only opacity and transform (rule 6)');
    assert.ok(arrival.duration > 0, 'the entrance role has no duration');
  }
  assert.equal(first[0].delay, 0, 'the first row waits');

  for (const [label, options, choice] of [['the system', { reducedMotion: 'reduce' }, null], ['the product', {}, 'reduced']]) {
    const opened = await open(browser, { width: 1280, path: '/pages', modulesOn: ['KNOWLEDGE'], ...options });
    t.after(opened.close);
    await opened.page.addInitScript(watch);
    if (choice) await opened.page.addInitScript((value) => localStorage.setItem('hubtask.motion', value), choice);
    await opened.page.reload();
    await opened.page.getByRole('main').getByRole('link', { name: HUB.name }).waitFor({ timeout: 10_000 });
    assert.deepEqual(await opened.page.evaluate(() => window.__arrivals ?? []), [], `a row moved although ${label} asked for reduced motion`);
    assert.deepEqual(opened.failures, []);
  }
});

// §5.3: goals and configuration items are found by the search, and the palette finds boards and
// goals beside pages - "FortiGate" and a goal's title found nothing before.
test('chromium 1280 px: the search finds goals and configuration items, the palette boards and goals', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/search', modulesOn: ['WHITEBOARD', 'GOALS', 'ASSETS'] });
  t.after(close);
  const field = page.getByRole('searchbox', { name: 'Search everything you can see' });
  await field.fill('kundinnen');
  const goals = page.getByRole('region', { name: 'Goals' });
  await goals.waitFor({ timeout: 10_000 });
  assert.equal(await goals.getByRole('link').first().getAttribute('href'), `/goals/${GOAL_ROWS[0].id}`);
  await field.fill('fortigate');
  const assets = page.getByRole('region', { name: 'Assets' });
  await assets.waitFor({ timeout: 10_000 });
  assert.equal(await assets.getByRole('link').first().getAttribute('href'), `/cis/${CIS[0].id}`);

  const palette = page.getByRole('combobox', { name: 'Search everything' });
  await palette.click();
  await palette.fill('kick-off');
  await page.locator('#search-menu').getByRole('option', { name: /Kick-off/u }).waitFor({ timeout: 10_000 });
  await palette.fill('kundinnen');
  await page.locator('#search-menu').getByRole('option', { name: /Mehr Kundinnen gewinnen/u }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// §5.3: portfolios, dashboards, releases and sprints are found too, by the search and the palette,
// from the same sources; a hub's inventory is read in its largest page, not the first hundred.
test('chromium 1280 px: the search and the palette find portfolios, dashboards, releases and sprints', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  ciLimits.length = 0;
  const { page, failures, close } = await open(browser, { width: 1280, path: '/search', modulesOn: ['GOALS', 'ASSETS', 'PLANNING'] });
  t.after(close);
  const field = page.getByRole('searchbox', { name: 'Search everything you can see' });
  for (const [words, region, href] of [
    ['wachstum', 'Portfolios', `/portfolios/${PORTFOLIOS[0].id}`],
    ['morgen', 'Dashboards', `/dashboards/${DASHBOARDS[0].id}`],
    ['autumn', 'Releases', `/hubs/${HUB.id}/releases`],
    ['kranich', 'Sprints', `/collections/${SCRUM.id}/sprints`],
    ['fortigate', 'Assets', `/cis/${CIS[0].id}`],
  ]) {
    await field.fill(words);
    const found = page.getByRole('region', { name: region });
    await found.waitFor({ timeout: 10_000 });
    assert.equal(await found.getByRole('link').first().getAttribute('href'), href, region);
  }
  assert.ok(ciLimits.length > 0 && ciLimits.every((limit) => limit === '500'), `the inventory was read in pages of ${ciLimits.join(', ')}`);

  const palette = page.getByRole('combobox', { name: 'Search everything' });
  await palette.click();
  await palette.fill('wachstum');
  await page.locator('#search-menu').getByRole('option', { name: /Wachstum im Norden/u }).waitFor({ timeout: 10_000 });
  await palette.fill('kranich');
  await page.locator('#search-menu').getByRole('option', { name: /Sprint Kranich/u }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// §2.2: a home's hub rows say each hub's state, not its name alone - the plans' its next release,
// the assets' its inventory - with the hub's own figure where it counts one.
test('chromium 1280 px: the plans\' and the assets\' hub rows say each hub\'s state', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/plans', modulesOn: ['PLANNING', 'ASSETS'] });
  t.after(close);
  const hubs = page.getByRole('region', { name: 'In your hubs' });
  const row = hubs.locator('li').filter({ hasText: HUB.name });
  await row.locator('[data-hub-line]').waitFor({ timeout: 10_000 });
  assert.match(await row.locator('[data-hub-line]').innerText(), /Autumn fixes/u);

  await page.goto(`${served.origin}/assets`);
  const assets = page.getByRole('region', { name: 'In your hubs' }).locator('li').filter({ hasText: HUB.name });
  await assets.locator('[data-hub-line]').waitFor({ timeout: 10_000 });
  assert.match(await assets.locator('[data-hub-line]').innerText(), /42 configuration items · 2 need attention/u);
  assert.deepEqual(failures, []);
});
