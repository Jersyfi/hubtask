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
const agent = (n, name, status, owner) => ({ id: `01a0e2e0-0000-7000-8000-0000000002${n}`, account_id: null, name, purpose: 'Sorts new requests into the right queue', kind: 'TICKET_TRIAGE', owner_id: owner, status });
const AGENTS = [agent('01', 'Old sorter', 'RETIRED', ACCOUNT.id), agent('02', 'Request sorter', 'ACTIVE', ACCOUNT.id), agent('03', 'Duplicate finder', 'SUSPENDED', '01a0e2e0-0000-7000-8000-0000000009ff')];
const pageOf = (n, title, at, more = {}) => ({ id: `01a0e2e0-0000-7000-8000-0000000003${n}`, hub_id: HUB.id, parent_id: null, title, icon: null, position: n, template: false, content_language: null, owner_id: null, verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null, trashed_at: null, created_by: ACCOUNT.id, created_at: at, updated_by: ACCOUNT.id, updated_at: at, version: 1, ...more });
const PAGES = [
  pageOf('01', 'Onboarding guide', '2026-10-02T09:00:00Z'),
  pageOf('02', 'How we release, step by step, for every team that ships to customers', '2026-10-31T09:00:00Z'),
  pageOf('03', 'Meeting notes template', '2026-11-01T09:00:00Z', { template: true }),
  pageOf('04', 'Support rota', '2026-09-01T09:00:00Z', { owner_id: ACCOUNT.id, due_for_review: true }),
];
const HEALTH = { total: 42, completeness: 0.9, correctness: 0.8, stale: 2, orphans: 1, duplicates: 0, without_owner: 3, review_overdue: 4, classes: [], problems: [{ ci_id: '01a0e2e0-0000-7000-8000-000000000401', class: 'server', kinds: ['STALE'] }, { ci_id: '01a0e2e0-0000-7000-8000-000000000402', class: 'server', kinds: ['NO_OWNER'] }], stale_relations: 0, services: [], sources: [] };
const DESK = { id: '01a0e2e0-0000-7000-8000-000000000501', type: 'COLLECTION', parent_id: HUB.id, name: 'IT help desk for every office and every remote worker', order_key: 'a0', version: 1, structure: { settings: { entry_types: { TASK: ['incident'] } } } };
const SCRUM = { id: '01a0e2e0-0000-7000-8000-000000000502', type: 'COLLECTION', parent_id: HUB.id, name: 'Sprint work', order_key: 'a1', version: 1, structure: { settings: { entry_types: { TASK: ['story'] } } } };
const TYPES = [
  { key: 'incident', level: 'TASK', name: 'Incident', system: false, position: 1, archived: false, form: { channels: [{ channel: 'PORTAL' }] } },
  { key: 'story', level: 'TASK', name: 'Story', system: false, position: 2, archived: false },
];
const ticket = (n, title) => ({ id: `01a0e2e0-0000-7000-8000-0000000006${n}`, type: 'TASK', entry_type: 'incident', collection_id: DESK.id, title, completion: { mode: 'BINARY', is_completed: false }, version: 1 });
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
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/whiteboards`)) return route.fulfill({ json: { data: BOARDS } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/pages`) && !url.searchParams.has('templates')) return route.fulfill({ json: { data: PAGES } });
    if (path.endsWith(`/api/v1/hubs/${HUB.id}/cmdb-health`)) return route.fulfill({ json: HEALTH });
    if (path.endsWith('/api/v1/entry-types')) return route.fulfill({ json: { data: TYPES } });
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

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { width, scheme = 'light', path = '/', ...modules }) {
  const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme });
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
    assert.deepEqual(rows, ['Today', 'Search', 'Inbox', 'Waiting for my yes', 'Pages', 'Service desk', 'Time', HUB.name], JSON.stringify(rows));
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
  await none.page.getByRole('button', { name: 'Switch Whiteboard on' }).waitFor({ timeout: 5_000 });
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
  await page.getByRole('button', { name: 'Switch Whiteboard on' }).waitFor({ timeout: 5_000 });
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

// §2.2: the agents' home lists the agents working here, live first, a retired one not at all.
test('chromium 1280 px: the agents\' home lists the agents working here', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: '/agents', modulesOn: ['AGENTS'] });
  t.after(close);
  const working = page.getByRole('region', { name: 'Working here' });
  await working.waitFor({ timeout: 10_000 });
  const names = await working.locator('[title]').evaluateAll((each) => each.map((node) => node.getAttribute('title')));
  assert.deepEqual(names, ['Request sorter', 'Duplicate finder']);
  assert.equal(await working.getByRole('link', { name: 'Request sorter' }).getAttribute('href'), '/profile/agents');
  assert.match((await working.textContent()) ?? '', /Live[\s\S]*Stopped/u);
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
    assert.deepEqual(figures, ['Configuration items=42', 'Found wanting=2', 'Review overdue=4']);
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
    const { page, failures, close } = await open(browser, { width, scheme, path: '/desk', modulesOn: ['SERVICE_DESK'] });
    t.after(close);
    const queues = page.getByRole('tablist', { name: 'Queues' });
    await queues.getByRole('tab', { name: 'Mine (1)' }).waitFor({ timeout: 10_000 });
    assert.deepEqual(await queues.getByRole('tab').allTextContents(), ['Mine (1)', 'Unassigned (2)', 'About to breach (0)', 'All open (3)']);
    // Only the desk is asked, by what breaks first, counted.
    assert.ok(queries.length > 0 && queries.every((body) => JSON.stringify(body.filter.nodes[0]) === JSON.stringify({ op: 'IN', field: 'collection_id', value: [DESK.id] }) && body.sort[0].field === 'sla_due' && body.count === 'exact'), JSON.stringify(queries));
    await queues.getByRole('tab', { name: 'Unassigned (2)' }).click();
    const panel = page.getByRole('tabpanel');
    await panel.getByRole('link', { name: 'VPN drops at noon' }).waitFor({ timeout: 5_000 });
    await queues.getByRole('tab', { name: 'About to breach (0)' }).click();
    await panel.getByText('Nothing waits in this queue').waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `desk home ${width} ${scheme}`);

    await page.goto(`${served.origin}/hubs/${HUB.id}/desk`);
    const strip = page.getByRole('navigation', { name: HUB.name });
    await strip.getByRole('link', { name: 'Desk' }).waitFor({ timeout: 10_000 });
    assert.equal(await strip.getByRole('link', { name: 'Desk' }).getAttribute('aria-current'), 'page');
    const desks = page.getByRole('region', { name: 'Desks' });
    await desks.waitFor({ timeout: 10_000 });
    assert.deepEqual(await desks.locator('.name').allTextContents(), [DESK.name]);
    await page.getByRole('tab', { name: 'All open (3)' }).waitFor({ timeout: 10_000 });
    await noSidewaysScroll(page, `hub desk ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}
