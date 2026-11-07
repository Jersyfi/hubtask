// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The navigation concept's sidebar (UC-WRK-36 checks 1-8 and 17, UC-WRK-37 check 5): the fixed top
// and foot, the workspace level, a hub's level with its tools, queues, collections and one content
// tree, the quick switch, the width, the rail and its peek. Walked at desk width; `SHOTS=<dir>`
// also writes the screenshots the mockup is compared with.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PERMISSIONS, fallback } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const SHOTS = process.env.SHOTS;

const id = (n) => `01a0e2e0-0000-7000-8000-${String(n).padStart(12, '0')}`;
const ACCOUNT = { id: id(1), kind: 'USER', display_name: 'Mara Lind', email: 'mara@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z', products_seen: ['PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'] };
const hub = (n, name, color) => ({ id: id(n), type: 'HUB', parent_id: null, name, color_token: color, order_key: `a${n}`, version: 1 });
const IT = hub(100, 'IT operations', 'teal');
const PD = hub(101, 'Product development', 'orange');
const HOME = hub(102, 'Household', 'violet');
const OTHERS = ['Marketing', 'People', 'Finance', 'Facilities', 'Sales', 'Purchasing', 'Legal', 'Research', 'Support', 'Design', 'Data'].map((name, i) => hub(110 + i, name, 'blue'));
const HUBS = [PD, IT, HOME, ...OTHERS];
const collection = (n, parent, name, extra = {}) => ({ id: id(n), type: 'COLLECTION', parent_id: parent.id, name, order_key: `a${n}`, version: 1, ...extra });
const DESK = collection(200, IT, 'Service desk', { structure: { settings: { entry_types: { TASK: ['incident'] } } }, figure: { kind: 'DUE', due: 1 } });
const CHANGES = collection(201, IT, 'Changes', { figure: { kind: 'OPEN', open: 4 } });
const WEB = collection(210, PD, 'Web app', { figure: { kind: 'OPEN', open: 18 } });
const MOBILE = collection(212, PD, 'Mobile', { figure: { kind: 'OPEN', open: 12 } });
const DESIGN = collection(213, PD, 'Design', { figure: { kind: 'OPEN', open: 14 } });
const BUGS = collection(211, PD, 'Bugs', { figure: { kind: 'DUE', due: 3 } });
const TODO = collection(220, HOME, 'To do');
const COLLECTIONS = [DESK, CHANGES, WEB, MOBILE, DESIGN, BUGS, TODO];
/** A day counted from today, as a calendar date. */
const day = (offset) => new Date(Date.now() + offset * 86_400_000).toISOString().slice(0, 10);
const sprint = (n, at, name, endIn) => ({ id: id(n), collection_id: at.id, name, goal: '', start_on: day(endIn - 13), end_on: day(endIn), state: 'ACTIVE', unit: 'POINTS', committed_value: 0, committed_count: 0, completed_value: 0, completed_count: 0, started_at: null, closed_at: null, version: 1 });
const SPRINTS = { [WEB.id]: [sprint(600, WEB, 'Sprint 42', 5)], [MOBILE.id]: [sprint(601, MOBILE, 'Sprint 9', 8)] };
// Product development's handbook: pages nested two deep, boards among them and one not yet filed.
const pdPage = (n, title, parent = null) => ({ id: id(n), hub_id: PD.id, parent_id: parent, title, order_key: `a${n}`, template: false, version: 1 });
const PD_PAGES = [
  pdPage(310, 'Product handbook'), pdPage(311, 'Vision and principles', id(310)), pdPage(312, 'How we plan', id(310)),
  pdPage(313, 'Release notes'), pdPage(314, 'Release 3.1', id(313)), pdPage(315, 'Release 3.2', id(313)),
  pdPage(316, 'Architecture decisions'), pdPage(317, 'Onboarding'), pdPage(318, 'Meeting notes'),
];
const PD_BOARDS = [
  { id: id(410), hub_id: PD.id, title: 'Roadmap sketch', parent_page_id: id(312), order_key: 'a1', version: 1 },
  { id: id(411), hub_id: PD.id, title: 'Retro board', parent_page_id: null, order_key: null, version: 1 },
];
const GOALS = ['Ship the new editor', 'Halve the bug backlog', 'Faster first load', 'Mobile parity', 'Accessible everywhere']
  .map((title, i) => ({ id: id(700 + i), hub_id: PD.id, period_id: id(799), parent_id: null, title, status: i === 2 ? 'AT_RISK' : 'ON_TRACK' }));
const RELEASES = [{ id: id(710), hub_id: PD.id, name: 'Release 3.2', state: 'PLANNED', release_on: day(14), version: 1 }];
/** What is on the reader today: five entries due by tonight, and one later. */
const MINE = [0, 0, 0, -1, -2, 6].map((offset, i) => ({ id: id(800 + i), title: `Entry ${i + 1}`, collection_id: WEB.id, due_at: `${day(offset)}T00:00:00Z`, due_date_only: true, completion: { is_completed: false } }));
const ARRIVALS = [0, 1, 2].map((i) => ({ id: id(900 + i), channel: 'EMAIL', attachments: [], status: 'NEW', received_at: '2026-11-07T08:00:00Z' }));
const APPROVALS = [0, 1].map((i) => ({ id: id(950 + i), state: 'PENDING' }));
const KB = { id: id(300), hub_id: IT.id, parent_id: null, title: 'Knowledge base', order_key: 'a1', template: false, version: 1 };
const VPN = { id: id(301), hub_id: IT.id, parent_id: KB.id, title: 'Set up the VPN', order_key: 'a1', template: false, version: 1 };
const RUNBOOKS = { id: id(302), hub_id: IT.id, parent_id: null, title: 'Runbooks', order_key: 'a2', template: false, version: 1 };
const TEMPLATE = { id: id(303), hub_id: IT.id, parent_id: null, title: 'Incident review template', order_key: 'a3', template: true, version: 1 };
const MAP = { id: id(400), hub_id: IT.id, title: 'Network map', parent_page_id: null, order_key: 'a5', version: 1 };
const QUEUES = [
  { view_id: id(500), key: 'about_to_breach', name: 'About to breach', count: 1, at_risk: 1 },
  { view_id: id(501), key: 'mine', name: 'Mine', count: 3, at_risk: 0 },
  { view_id: id(502), key: 'unassigned', name: 'Unassigned', count: 2, at_risk: 0 },
];
const PAGE = { items: [], page: { next_cursor: null, has_more: false } };

function stubFor({ modulesOn, hubModules = {}, stars = [], prefs = {}, administration = true }) {
  const written = { preferences: [], stars: [] };
  const handler = async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (path.endsWith('/api/v1/stream')) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
    if (path.endsWith('/api/v1/sync:snapshot')) {
      const records = [...HUBS, ...COLLECTIONS].map((each) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: each.id, container_id: each.parent_id, payload: each }));
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
    }
    if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
    if (path.endsWith(`/api/v1/accounts/${ACCOUNT.id}/preferences`) && request.method() === 'PATCH') {
      const body = request.postDataJSON();
      written.preferences.push(body);
      Object.assign(prefs, body);
      return route.fulfill({ json: { ...ACCOUNT, ...prefs } });
    }
    if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: { ...ACCOUNT, ...prefs } });
    if (path.endsWith(`/api/v1/accounts/${ACCOUNT.id}/stars`)) {
      if (request.method() === 'PUT') {
        const body = request.postDataJSON();
        written.stars.push(body);
        stars.splice(0, stars.length, ...body.stars);
      }
      return route.fulfill({ json: { stars, version: written.stars.length } });
    }
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: administration ? PERMISSIONS.OWNER : ['READ', 'WRITE_ITEMS'], modules_on: modulesOn } });
    }
    const modules = /\/api\/v1\/containers\/([^/]+)\/modules$/.exec(path);
    if (modules) {
      const at = COLLECTIONS.find((each) => each.id === modules[1])?.parent_id ?? modules[1];
      return route.fulfill({ json: { modules_on: hubModules[at] ?? modulesOn } });
    }
    if (path.endsWith('/api/v1/entry-types')) {
      return route.fulfill({ json: { data: [{ key: 'incident', name: 'Incident', level: 'TASK', hub_id: null, form: { channels: [{ channel: 'PORTAL' }] } }], page: PAGE.page } });
    }
    if (path.endsWith(`/api/v1/hubs/${IT.id}/queues`)) return route.fulfill({ json: { data: QUEUES } });
    if (/\/api\/v1\/hubs\/[^/]+\/queues$/.test(path)) return route.fulfill({ json: { data: [] } });
    if (path.endsWith(`/api/v1/hubs/${IT.id}/pages`) && !url.searchParams.has('templates')) return route.fulfill({ json: { data: [KB, VPN, RUNBOOKS, TEMPLATE], ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${IT.id}/whiteboards`)) return route.fulfill({ json: { data: [MAP], ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${PD.id}/pages`) && !url.searchParams.has('templates')) return route.fulfill({ json: { data: PD_PAGES, ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${PD.id}/whiteboards`)) return route.fulfill({ json: { data: PD_BOARDS, ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${PD.id}/releases`)) return route.fulfill({ json: { data: RELEASES, ...PAGE } });
    if (path.endsWith(`/api/v1/hubs/${IT.id}/cmdb-health`)) return route.fulfill({ json: { total: 120, problems: Array.from({ length: 6 }, (_, i) => ({ ci_id: id(980 + i), kind: 'STALE' })) } });
    if (path.endsWith('/api/v1/goals') && url.searchParams.get('hub_id') === PD.id) return route.fulfill({ json: { data: GOALS, ...PAGE } });
    if (path.endsWith('/api/v1/jumble/entries')) return route.fulfill({ json: { data: url.searchParams.get('status') === 'NEW' ? ARRIVALS : [], page: { next_cursor: null } } });
    if (path.endsWith('/api/v1/approvals')) return route.fulfill({ json: { data: APPROVALS, page: { next_cursor: null, has_more: false } } });
    if (path.endsWith('/api/v1/search') && (request.postDataJSON()?.filter?.nodes ?? []).some((node) => node.field === 'assignee_id')) {
      return route.fulfill({ json: { data: MINE, page: { next_cursor: null, has_more: false } } });
    }
    if (path.endsWith('/api/v1/items:query')) {
      const body = request.postDataJSON() ?? {};
      const nodes = body.filter?.nodes ?? [];
      if (nodes.some((node) => node.field === 'collection_id' && node.value?.includes?.(DESK.id))) return route.fulfill({ json: { data: [], total: 1, page: { next_cursor: null, has_more: false } } });
      return route.fulfill({ json: { data: [], total: 0, page: { next_cursor: null, has_more: false } } });
    }
    if (/\/api\/v1\/collections\/[^/]+\/sprints$/.test(path)) {
      const at = /collections\/([^/]+)\/sprints/.exec(path)[1];
      return route.fulfill({ json: { data: SPRINTS[at] ?? [], runs_sprints: at in SPRINTS } });
    }
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: HUBS } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: COLLECTIONS.filter((each) => each.parent_id === url.searchParams.get('parent_id')) } });
    if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
    return fallback(route, request, path);
  };
  return { handler, written };
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { width = 1440, height = 900, path = '/', storage = {}, ...options }) {
  const context = await browser.newContext({ viewport: { width, height } });
  const { handler, written } = stubFor(options);
  await context.route('**/api/v1/**', handler);
  await context.addInitScript((kept) => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    for (const [key, value] of Object.entries(kept)) localStorage.setItem(key, value);
  }, storage);
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}${path}`);
  await page.getByRole('button', { name: ACCOUNT.display_name }).waitFor({ timeout: 15_000 });
  return { page, failures, written, close: () => context.close() };
}

const ALL = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];
const side = (page) => page.locator('aside.sidenav');
const texts = async (locator) => (await locator.allInnerTexts()).map((each) => each.split('\n')[0].trim()).filter(Boolean);
/** The section heads of the middle, in order. */
const heads = (page) => texts(side(page).locator('section > :first-child'));

test('UC-WRK-36 check 1: the workspace level from the top, the foot pinned', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { modulesOn: ALL, hubModules: { [HOME.id]: [] }, stars: [{ kind: 'HUB', id: IT.id }] });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(side(page).getByRole('list', { name: 'For you' }).getByRole('link')), ['Today', 'Inbox', 'Waiting for your yes']);
  assert.deepEqual(await texts(side(page).getByRole('list', { name: 'What is kept' }).getByRole('link')), ['Administration', 'Archive', 'Trash']);
  await side(page).getByRole('link', { name: 'IT operations' }).first().waitFor({ timeout: 10_000 });
  const order = await heads(page);
  assert.deepEqual(order.map((each) => each.toLowerCase()), ['starred', 'products', 'hubs'], JSON.stringify(order));
  const productRows = await texts(side(page).locator('section').nth(1).getByRole('link'));
  assert.deepEqual(productRows, ['Plans', 'Pages', 'Whiteboards', 'Service desk', 'Assets', 'Time', 'Goals', 'Agents']);
  // 14 hubs: 12 shown, then *n more*.
  assert.equal(await side(page).getByRole('treeitem').count(), 12);
  await side(page).getByText('2 more hubs').waitFor();
  assert.equal(await side(page).getByRole('link', { name: 'Today' }).getAttribute('aria-current'), 'page');
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 1: no star, no Starred; no administration without the right; no product while none is on', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { modulesOn: [], administration: false });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).waitFor({ timeout: 10_000 });
  assert.deepEqual((await heads(page)).map((each) => each.toLowerCase()), ['hubs']);
  assert.deepEqual(await texts(side(page).getByRole('list', { name: 'What is kept' }).getByRole('link')), ['Archive', 'Trash']);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 checks 2-6: a hub\'s level - head, tools in order, queues, collections, one content tree', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  // Planning is off at the hub: no Plan; the desk collection brings Queues.
  const { page, failures, close } = await open(browser, {
    path: `/hubs/${IT.id}`,
    modulesOn: ALL,
    hubModules: { [IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'] },
    stars: [{ kind: 'VIEW', id: id(501) }],
  });
  t.after(close);
  const level = side(page).getByRole('region', { name: 'IT operations' });
  await level.getByRole('link', { name: 'Queues' }).waitFor({ timeout: 10_000 });
  await level.getByRole('button', { name: 'All hubs' }).waitFor();
  await level.getByRole('heading', { name: 'IT operations' }).waitFor();
  await level.getByRole('button', { name: 'Star IT operations' }).waitFor();
  await level.getByRole('button', { name: 'Switch hub' }).waitFor();
  const tools = await texts(level.locator('ul.tools > li > a'));
  // Check 3: the fixed order, Overview first; Mine is starred and leads the queues (check 4).
  assert.deepEqual(tools, ['Overview', 'Queues', 'Mine', 'About to breach', 'Unassigned', 'Inventory', 'Goals'], JSON.stringify(tools));
  assert.equal(await level.getByRole('link', { name: 'Overview' }).getAttribute('aria-current'), 'page');
  // Check 4: a breaching queue's count is on the danger surface and says so in words.
  const breach = level.locator('ul.tools > li', { hasText: 'About to breach' });
  assert.ok((await breach.innerText()).includes('1'));
  assert.ok(await breach.locator('[data-urgent], .urgent, [aria-label*="about to breach"], [title*="about to breach"]').count() >= 1, 'the urgent count has no words');
  // Parts in order: Collections, then Contents with its count.
  await level.getByRole('tree', { name: 'Contents of IT operations' }).waitFor();
  assert.deepEqual((await heads(page)).map((each) => each.toLowerCase().replace(/\s+/g, ' ')), ['collections', 'contents · 4']);
  // Check 6: the desk carries its mark.
  const desk = level.getByRole('treeitem', { name: /Service desk/ });
  assert.ok((await desk.innerHTML()).includes('Desk'), 'the desk mark is missing');
  // Check 5: pages and the board in one tree, the template left out, the board without a twist.
  const contents = level.getByRole('tree', { name: 'Contents of IT operations' });
  assert.deepEqual(await texts(contents.getByRole('treeitem')), ['Knowledge base', 'Runbooks', 'Network map']);
  assert.equal(await contents.getByRole('treeitem', { name: 'Network map' }).getAttribute('aria-expanded'), null);
  // Check 2: *‹ All hubs* returns to the workspace level; the page stays.
  await level.getByRole('button', { name: 'All hubs' }).click();
  await side(page).getByRole('tree', { name: 'Hubs' }).waitFor({ timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, `/hubs/${IT.id}`);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 2: a collection, a page and a tool show their hub\'s level; focus lands on All hubs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: '/', modulesOn: ALL, hubModules: { [PD.id]: ['PLANNING', 'TIME', 'GOALS'] } });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /Product development/ }).click();
  const level = side(page).getByRole('region', { name: 'Product development' });
  await level.getByRole('link', { name: 'Plan' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(level.locator('ul.tools > li > a')), ['Overview', 'Plan', 'Time', 'Goals']);
  await page.waitForFunction(() => document.activeElement?.textContent?.includes('All hubs'), undefined, { timeout: 5_000 });
  await level.getByRole('treeitem', { name: /Web app/ }).click();
  await page.waitForFunction((at) => location.pathname === `/collections/${at}`, WEB.id, { timeout: 5_000 });
  assert.equal(await level.getByRole('treeitem', { name: /Web app/ }).getAttribute('aria-current'), 'page');
  // The sprint mark after a collection that runs sprints (check 6).
  assert.ok((await level.getByRole('treeitem', { name: /Web app/ }).innerHTML()).includes('Runs sprints'));
  await level.getByRole('link', { name: 'Time' }).click();
  await page.waitForFunction((at) => location.pathname === `/hubs/${at}/actuals`, PD.id, { timeout: 5_000 });
  assert.equal(await side(page).getByRole('link', { name: 'Time' }).getAttribute('aria-current'), 'page');
  // No hub tool is also a tab on the screen (check 3): the screen's bar holds the tool's own views.
  const bar = page.getByRole('main').getByRole('navigation', { name: 'Product development' });
  await bar.waitFor({ timeout: 5_000 });
  assert.deepEqual(await texts(bar.getByRole('link')), ['Estimates and actuals', 'Workload']);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 7: the quick switch keeps the tool where the other hub has it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, {
    path: `/hubs/${PD.id}/plan`, modulesOn: ALL,
    hubModules: { [PD.id]: ['PLANNING'], [IT.id]: ['PLANNING'], [HOME.id]: [] },
    stars: [{ kind: 'HUB', id: IT.id }],
  });
  t.after(close);
  const level = side(page).getByRole('region', { name: 'Product development' });
  await level.getByRole('button', { name: 'Switch hub' }).click({ timeout: 10_000 });
  const flyout = page.getByRole('dialog', { name: 'Switch hub' });
  await flyout.getByRole('link', { name: 'IT operations' }).click();
  await page.waitForFunction((at) => location.pathname === `/hubs/${at}/plan`, IT.id, { timeout: 5_000 });
  await side(page).getByRole('region', { name: 'IT operations' }).getByRole('button', { name: 'Switch hub' }).click();
  await page.getByRole('dialog', { name: 'Switch hub' }).getByRole('searchbox').fill('house');
  await page.getByRole('dialog', { name: 'Switch hub' }).getByRole('link', { name: 'Household' }).click();
  await page.waitForFunction((at) => location.pathname === `/hubs/${at}`, HOME.id, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 17: a workspace with no module on shows no tool and no Contents in a hub (D1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { path: `/hubs/${HOME.id}`, modulesOn: [] });
  t.after(close);
  const level = side(page).getByRole('region', { name: 'Household' });
  await level.getByRole('treeitem', { name: /To do/ }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(level.locator('ul.tools > li > a')), ['Overview']);
  assert.deepEqual((await heads(page)).map((each) => each.toLowerCase()), ['collections']);
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-household.png` });
  assert.deepEqual(failures, []);
});

test('UC-WRK-37 check 5: ☆ in the hub\'s head stars it, and it stands under Starred', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { path: `/hubs/${PD.id}`, modulesOn: [] });
  t.after(close);
  const star = side(page).getByRole('button', { name: 'Star Product development' });
  await star.click({ timeout: 10_000 });
  await page.waitForFunction(() => true);
  await side(page).getByRole('button', { name: 'Remove the star from Product development' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(written.stars.at(-1), { stars: [{ kind: 'HUB', id: PD.id }] });
  await side(page).getByRole('button', { name: 'All hubs' }).click();
  await side(page).locator('section', { hasText: 'Starred' }).getByRole('link', { name: 'Product development' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 5: the contents mode is the person\'s, written to their preferences', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, written, close } = await open(browser, { path: `/hubs/${IT.id}`, modulesOn: ALL, hubModules: { [IT.id]: ['KNOWLEDGE', 'WHITEBOARD'] } });
  t.after(close);
  const contents = side(page).getByRole('tree', { name: 'Contents of IT operations' });
  await contents.waitFor({ timeout: 10_000 });
  await side(page).getByRole('button', { name: 'Contents', exact: true }).click();
  await page.getByRole('menuitem', { name: 'A–Z' }).click();
  await page.waitForFunction(() => true);
  await contents.getByRole('treeitem', { name: 'Set up the VPN' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(written.preferences.at(-1), { nav_contents_mode: 'NAME' });
  assert.deepEqual(await texts(contents.getByRole('treeitem')), ['Knowledge base', 'Network map', 'Runbooks', 'Set up the VPN']);
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 8: the width is dragged and kept, Ctrl+[ folds to the rail, hovering it peeks', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { modulesOn: ['PLANNING'], storage: { 'hubtask.nav.width': '320' } });
  t.after(close);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).waitFor({ timeout: 10_000 });
  assert.equal(Math.round((await side(page).boundingBox()).width), 320, 'the kept width is not used');
  const handle = side(page).getByRole('separator', { name: 'Width of the navigation' });
  await handle.dblclick();
  assert.equal(Math.round((await side(page).boundingBox()).width), 264);
  const box = await handle.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + 40);
  await page.mouse.down();
  await page.mouse.move(box.x + 600, box.y + 40, { steps: 5 });
  await page.mouse.up();
  assert.equal(Math.round((await side(page).boundingBox()).width), 400, 'the drag is not bounded at 400');
  assert.equal(await page.evaluate(() => localStorage.getItem('hubtask.nav.width')), '400');
  await page.locator('main').click({ position: { x: 600, y: 400 } });
  await page.keyboard.press('Control+BracketLeft');
  await page.waitForFunction(() => document.querySelector('aside.sidenav')?.hasAttribute('data-rail'), undefined, { timeout: 5_000 });
  assert.ok((await side(page).boundingBox()).width < 80);
  await page.mouse.move(20, 300);
  await page.waitForFunction(() => document.querySelector('aside.sidenav')?.hasAttribute('data-peek'), undefined, { timeout: 5_000 });
  await side(page).getByRole('tree', { name: 'Hubs' }).waitFor({ timeout: 5_000 });
  await page.mouse.move(900, 400);
  await page.waitForFunction(() => !document.querySelector('aside.sidenav')?.hasAttribute('data-peek'), undefined, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

/** The figure at a row's end: its last line, where the name is the first. */
const figureOf = async (row) => (await row.innerText()).split('\n').map((each) => each.trim()).filter(Boolean).slice(1).join(' ');

test('UC-WRK-36 checks 1-6: the rows carry their figures, the hub head its ⋯, the hub its contents', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const recent = JSON.stringify(Array.from({ length: 20 }, (_, i) => ({ kind: 'collection', id: id(1000 + i), title: `Place ${i + 1}`, at: Date.now() - i * 60_000, placeId: PD.id })));
  const { page, failures, close } = await open(browser, {
    height: 1300,
    path: `/hubs/${PD.id}`,
    modulesOn: ALL,
    hubModules: { [PD.id]: ['PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'], [IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'] },
    stars: [{ kind: 'COLLECTION', id: WEB.id }, { kind: 'HUB', id: IT.id }],
    storage: { [`hubtask.recent.${ACCOUNT.id}`]: recent },
  });
  t.after(close);
  const top = side(page).getByRole('list', { name: 'For you' });
  const level = side(page).getByRole('region', { name: 'Product development' });
  await level.getByRole('tree', { name: 'Contents of Product development' }).waitFor({ timeout: 10_000 });
  // For you: what is due by tonight, the undecided arrivals, what waits for the reader's yes (§2.2).
  await page.waitForFunction(() => /Today\s*5/.test(document.querySelector('aside.sidenav ul.foryou')?.textContent ?? ''), undefined, { timeout: 5_000 });
  assert.equal(await figureOf(top.getByRole('listitem').filter({ hasText: 'Today' })), '5');
  assert.equal(await figureOf(top.getByRole('listitem').filter({ hasText: 'Inbox' })), '3');
  assert.equal(await figureOf(top.getByRole('listitem').filter({ hasText: 'Waiting for your yes' })), '2');
  // The hub head: the name, ▾, ☆ and ⋯ - the ⋯ there for every reader, its link first.
  const more = level.getByRole('button', { name: 'Manage Product development' });
  await more.waitFor();
  // The tools' figures: the running sprint, the goals at risk (§2.4 item 2).
  const tool = (name) => level.locator('ul.tools > li', { hasText: name });
  await page.waitForFunction(() => document.querySelector('aside.sidenav ul.tools')?.textContent?.includes('at risk'), undefined, { timeout: 5_000 });
  assert.equal(await figureOf(tool('Plan')), 'Sprint 42 · 6 d');
  assert.equal(await figureOf(tool('Goals')), '1 at risk');
  // The collections' figures (§2.4 part 3): the sprint's days left, open, due.
  const row = (name) => level.getByRole('treeitem', { name: new RegExp(name) });
  await page.waitForFunction(() => document.querySelector('aside.sidenav [role="tree"]')?.textContent?.includes('9 d'), undefined, { timeout: 5_000 });
  assert.deepEqual(await Promise.all(['Web app', 'Mobile', 'Design', 'Bugs'].map((name) => figureOf(row(name)))), ['6 d', '9 d', '14 open', '3 due']);
  // Contents: the hub's pages and boards in one tree, with its count.
  assert.ok((await heads(page)).some((each) => /^contents · 11$/i.test(each.replace(/\s+/g, ' '))), JSON.stringify(await heads(page)));
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-pd.png` });
  await more.click();
  await page.getByRole('menuitem', { name: 'Copy link' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  // IT operations: a desk's requests about to breach, the inventory's attention.
  await page.goto(`${served.origin}/hubs/${IT.id}`);
  const it = side(page).getByRole('region', { name: 'IT operations' });
  await it.getByRole('link', { name: 'Queues' }).waitFor({ timeout: 10_000 });
  await page.waitForFunction(() => document.querySelector('aside.sidenav [role="tree"]')?.textContent?.includes('SLA'), undefined, { timeout: 5_000 });
  assert.equal(await figureOf(it.getByRole('treeitem', { name: /Service desk/ })), '1 SLA');
  assert.equal(await figureOf(it.locator('ul.tools > li', { hasText: 'Inventory' })), '6 !');
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-it.png` });

  // The workspace level: Recent opens the last twenty places (§2.3).
  await page.goto(`${served.origin}/`);
  await side(page).getByRole('treeitem', { name: /IT operations/ }).waitFor({ timeout: 10_000 });
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-today.png` });
  await side(page).getByRole('button', { name: 'Recent' }).click();
  assert.equal(await page.getByRole('dialog', { name: 'Recent' }).getByRole('link').count(), 20);
  assert.deepEqual(failures, []);
});
