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
const BUGS = collection(211, PD, 'Bugs', { figure: { kind: 'DUE', due: 3 } });
const TODO = collection(220, HOME, 'To do');
const COLLECTIONS = [DESK, CHANGES, WEB, BUGS, TODO];
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
    if (/\/api\/v1\/collections\/[^/]+\/sprints$/.test(path)) return route.fulfill({ json: { data: [], runs_sprints: path.includes(WEB.id) } });
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: HUBS } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: COLLECTIONS.filter((each) => each.parent_id === url.searchParams.get('parent_id')) } });
    if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
    return fallback(route, request, path);
  };
  return { handler, written };
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { width = 1440, path = '/', storage = {}, ...options }) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
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
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-today.png` });
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
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-it.png` });
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
  if (SHOTS) await side(page).screenshot({ path: `${SHOTS}/live-hub-pd.png` });
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
