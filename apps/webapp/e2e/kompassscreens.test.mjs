// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The navigation concept's screens (step 5): the hub's overview without tabs, each hub tool's page
// with its one view bar, the queue page, a collection's one view bar, a page without a tree of its
// own, *All contents*, ＋ under a page, and the former hub tabs' addresses (UC-WRK-36 checks 9, 10
// and 12, UC-SVC-03 checks 10 and 11, UC-DOC-01 check 6, UC-CNV-10 check 6), the queue's *Filter* and
// *Save as queue*, and the *Desk* kind of a new collection. Walked at desk width;
// `SHOTS=<dir>` also writes the screenshots the mockup is compared with.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, DESK, IT, PD, PD_PAGES, QUEUES, WEB, id, open, served, side, texts } from './kompassstub.mjs';
import { MANIFEST, PERMISSIONS, PRESETS, ROLES, kindChoice, newInHead } from './fixture.mjs';

const SHOTS = process.env.SHOTS;
test.after(() => served.close());

const ON = {
  [PD.id]: ['WORKFLOWS', 'PLANNING', 'TIME', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
  [IT.id]: ['WORKFLOWS', 'SERVICE_DESK', 'ASSETS', 'GOALS', 'KNOWLEDGE', 'WHITEBOARD'],
};
const REQUEST = { id: id(1200), title: 'Printer on the 2nd floor does not print', collection_id: DESK.id, number: 41, created_by: id(1), assignee_id: null, status: null, completion: { is_completed: false } };
/** The queue's rows, and the page the walk opens. */
function extra(route, request, url) {
  const path = url.pathname;
  if (path.endsWith('/api/v1/items:query') && (request.postDataJSON()?.filter?.nodes ?? []).some((node) => node.field === 'sla_standing')) {
    return route.fulfill({ json: { data: [REQUEST], total: 1, page: { next_cursor: null, has_more: false } } }).then(() => true);
  }
  if (/\/api\/v1\/pages\/[^/]+\/labels$/.test(path)) return route.fulfill({ json: { labels: [] } }).then(() => true);
  if (/\/api\/v1\/pages\/[^/]+\/[a-z-]+$/.test(path)) return route.fulfill({ json: { data: [], page: { next_cursor: null, has_more: false } } }).then(() => true);
  const page = /\/api\/v1\/pages\/([^/]+)$/.exec(path);
  if (page && request.method() === 'GET') {
    const found = PD_PAGES.find((each) => each.id === page[1]);
    if (!found) return false;
    const ancestors = found.parent_id ? [PD_PAGES.find((each) => each.id === found.parent_id)].map((each) => ({ id: each.id, title: each.title })) : [];
    return route.fulfill({ json: { ...found, body: 'Welcome to the team.', writable: false, labels: [], shared: false, drafts: false, unpublished: false, draft: null, ancestors, updated_at: '2026-11-06T10:00:00Z' } }).then(() => true);
  }
  return false;
}
const walk = (browser, path, options = {}) => open(browser, { path, modulesOn: ALL, hubModules: ON, extra, owner: true, height: 1000, ...options });
const main = (page) => page.getByRole('main');
const at = (page, path) => page.waitForFunction((want) => location.pathname + location.search === want, path, { timeout: 10_000 });

test('UC-WRK-36 check 10: the hub\'s overview has no tabs - its tools\' lines and the collection cards', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await walk(browser, `/hubs/${PD.id}`);
  t.after(close);
  const lines = main(page).getByRole('list', { name: 'What the products say here' });
  await lines.locator('[data-product-line="goals"]').waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('navigation', { name: PD.name }).count(), 0, 'the overview draws a view bar');
  assert.equal(await main(page).getByRole('tab').count(), 0, 'the overview draws tabs');
  assert.equal(await lines.locator('[data-product-line="plan"] a').getAttribute('href'), `/hubs/${PD.id}/plan`);
  assert.equal(await lines.locator('[data-product-line="goals"] a').getAttribute('href'), `/hubs/${PD.id}/goals`);
  await lines.locator('[data-product-line="contents"]').waitFor();
  assert.equal(await lines.locator('[data-product-line="contents"] a').getAttribute('href'), `/hubs/${PD.id}/contents`);
  await main(page).locator(`[data-card="${WEB.id}"]`).waitFor();
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-hub.png` });
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 9: each hub tool draws one view bar of its own tabs, never a second strip', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await walk(browser, `/hubs/${PD.id}/plan`);
  t.after(close);
  const bar = main(page).getByRole('navigation', { name: PD.name });
  await bar.getByRole('link', { name: 'Schedule' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(bar.getByRole('link')), ['Schedule', 'Releases', 'Sprints']);
  assert.equal(await bar.getByRole('link', { name: 'Schedule' }).getAttribute('aria-current'), 'page');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-plan.png` });
  await bar.getByRole('link', { name: 'Sprints' }).click();
  await at(page, `/hubs/${PD.id}/plan/sprints`);
  await main(page).locator('[data-running-sprint]').first().waitFor({ timeout: 10_000 });
  assert.equal(await main(page).locator('[data-running-sprint]').count(), 2, 'the hub\'s two running sprints');
  assert.equal(await side(page).getByRole('link', { name: 'Plan' }).getAttribute('aria-current'), 'page');
  // Goals: the hub's goals and its dashboards.
  await side(page).getByRole('link', { name: 'Goals' }).click();
  await at(page, `/hubs/${PD.id}/goals`);
  const goals = main(page).getByRole('navigation', { name: PD.name });
  await goals.getByRole('link', { name: 'Dashboards' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(goals.getByRole('link')), ['Goals', 'Dashboards']);
  // Inventory: its sections in one bar, *Manage* a menu at its end.
  await page.goto(`${served.origin}/hubs/${IT.id}/inventory`);
  const sections = main(page).getByRole('navigation', { name: 'Parts of the inventory' });
  await sections.getByRole('link', { name: 'Inventory' }).waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('tablist').count(), 0, 'the inventory draws a strip of tabs');
  const manage = main(page).locator('[data-assets-manage]');
  if (await manage.count()) {
    await manage.click();
    await page.getByRole('menuitem', { name: 'Runs' }).click();
    await at(page, `/hubs/${IT.id}/inventory/runs`);
  }
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-inv.png` });
  assert.deepEqual(failures, []);
});

test('UC-SVC-03 check 10: a queue in the sidebar opens its table over the hub\'s desks; the desk\'s old address takes the first', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await walk(browser, `/hubs/${IT.id}/desk`);
  t.after(close);
  // The former tab's address takes the hub's first queue (§8).
  await at(page, `/hubs/${IT.id}/desk?queue=${id(500)}`);
  await main(page).getByRole('heading', { name: 'About to breach' }).waitFor({ timeout: 10_000 });
  const table = main(page).getByRole('table', { name: /About to breach/ });
  await table.getByRole('link', { name: REQUEST.title }).waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('radiogroup').count(), 0, 'the queue page draws the four queues\' choice');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-queue.png` });
  const level = side(page).getByRole('region', { name: IT.name });
  assert.equal(await level.getByRole('link', { name: /About to breach/ }).getAttribute('aria-current'), 'page');
  await level.getByRole('link', { name: /^Mine/ }).click();
  await at(page, `/hubs/${IT.id}/desk?queue=${id(501)}`);
  await main(page).getByRole('heading', { name: 'Mine' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 9: a collection draws one view bar - board, the running sprint, backlog, list, timeline, time - and its old plan is the timeline', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await walk(browser, `/collections/${WEB.id}`);
  t.after(close);
  const bar = main(page).getByRole('navigation', { name: WEB.name });
  // The sprint view names the running sprint (the mockup's *Sprint 42*), the backlog a view of its own.
  await bar.getByRole('link', { name: 'Sprint 42' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(bar.getByRole('link')), ['Board', 'Sprint 42', 'Backlog', 'List', 'Timeline', 'Time']);
  assert.equal(await main(page).getByRole('toolbar', { name: /entries/i }).count(), 0, 'a second strip under the bar');
  await bar.getByRole('link', { name: 'Board' }).click();
  await at(page, `/collections/${WEB.id}?layout=board`);
  await page.waitForFunction(() => [...document.querySelectorAll('main nav a[aria-current="page"]')].some((each) => each.textContent?.trim() === 'Board'), undefined, { timeout: 5_000 });
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-coll.png` });
  // The collection's former *Plan* and *Sprints* open its *Timeline* and *Sprint* views (§8).
  await page.goto(`${served.origin}/collections/${WEB.id}/plan`);
  await at(page, `/collections/${WEB.id}/timeline`);
  await main(page).getByRole('navigation', { name: WEB.name }).getByRole('link', { name: 'Timeline', exact: true }).waitFor({ timeout: 10_000 });
  assert.equal(await main(page).getByRole('navigation', { name: WEB.name }).getByRole('link', { name: 'Timeline', exact: true }).getAttribute('aria-current'), 'page');
  await page.goto(`${served.origin}/collections/${WEB.id}/sprints`);
  await at(page, `/collections/${WEB.id}/sprint`);
  await main(page).getByRole('navigation', { name: WEB.name }).getByRole('link', { name: 'Backlog', exact: true }).click();
  await at(page, `/collections/${WEB.id}/backlog`);
  await main(page).locator('[data-sprint-board][data-backlog]').waitFor({ timeout: 10_000 });
  assert.equal(await main(page).locator('[data-sprint-board] [data-drop="sprint"]').count(), 0, 'the backlog draws the sprint\'s lanes');
  assert.equal(await main(page).getByRole('navigation', { name: WEB.name }).getByRole('link', { name: 'Backlog', exact: true }).getAttribute('aria-current'), 'page');
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 12: the former hub tabs\' addresses open the places that took their role', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await walk(browser, `/hubs/${PD.id}/actuals`);
  t.after(close);
  await at(page, `/hubs/${PD.id}/time`);
  for (const [from, to] of [
    [`/hubs/${PD.id}/workload`, `/hubs/${PD.id}/time/workload`],
    [`/hubs/${PD.id}/releases`, `/hubs/${PD.id}/plan/releases`],
    [`/hubs/${IT.id}/assets`, `/hubs/${IT.id}/inventory`],
    [`/hubs/${IT.id}/assets/health`, `/hubs/${IT.id}/inventory/health`],
    [`/goals?hub=${PD.id}`, `/hubs/${PD.id}/goals`],
    [`/hubs/${PD.id}/whiteboards`, `/hubs/${PD.id}/contents?kind=whiteboards`],
    [`/hubs/${PD.id}/pages`, `/pages/${PD_PAGES[0].id}`],
  ]) {
    await page.goto(`${served.origin}${from}`);
    await at(page, to);
  }
  assert.deepEqual(failures, []);
});

test('UC-DOC-01 check 6, UC-CNV-10 check 6: a page has its trail and no tree of its own; ＋ on a page offers a whiteboard under it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const onboarding = PD_PAGES.find((each) => each.title === 'How we plan');
  const { page, failures, close } = await walk(browser, `/pages/${onboarding.id}`);
  t.after(close);
  await main(page).getByRole('heading', { name: 'How we plan' }).waitFor({ timeout: 10_000 });
  const trail = main(page).getByRole('navigation', { name: 'Where this page is' });
  assert.deepEqual(await texts(trail.getByRole('link')), [PD.name, 'Product handbook']);
  assert.equal(await main(page).getByRole('tree').count(), 0, 'the page draws a tree of its own');
  const contents = side(page).getByRole('tree', { name: `Contents of ${PD.name}` });
  assert.equal(await contents.getByRole('treeitem', { name: 'How we plan' }).getAttribute('aria-current'), 'page');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-wiki.png` });
  // ＋ on the page's row: *Page · From a template… · Whiteboard*, the board made under it.
  const row = contents.getByRole('treeitem', { name: 'How we plan' });
  await row.hover();
  await side(page).getByRole('button', { name: 'Create under “How we plan”' }).click();
  assert.deepEqual(await texts(page.getByRole('menuitem')), ['Page', 'From a template…', 'Whiteboard']);
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-create.png` });
  await page.getByRole('menuitem', { name: 'Whiteboard' }).click();
  await page.getByRole('dialog', { name: 'Create under “How we plan”' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('UC-SVC-03 check 11: the queue\'s Filter narrows it, and Save as queue keeps the narrowing as a queue of the hub', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const asked = [];
  const saved = [];
  const SAVED = { id: id(520), key: 'printers', name: 'Printers', scope_type: 'HUB', scope_id: IT.id, owner_id: id(1), layout: 'LIST_COLLAPSED', grouping: {}, visible_fields: [], sharing: 'SCOPE', queue: true, queue_position: 4, version: 1 };
  const filtering = async (route, request, url) => {
    const path = url.pathname;
    // The fields a condition can name, as the installation answers them.
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], view_layouts: ['LIST_COLLAPSED'], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: PERMISSIONS.OWNER, modules_on: ALL, roles: ROLES, query_fields: MANIFEST.query_fields } }).then(() => true);
    }
    if (path.endsWith('/api/v1/items:query')) asked.push(request.postDataJSON());
    if (path.endsWith('/api/v1/views') && request.method() === 'POST') {
      saved.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { ...SAVED, query: request.postDataJSON().query } }).then(() => true);
    }
    if (path.endsWith(`/api/v1/hubs/${IT.id}/queues`) && saved.length > 0) {
      return route.fulfill({ json: { data: [...QUEUES, { view_id: SAVED.id, key: SAVED.key, name: SAVED.name, count: 1, at_risk: 0 }] } }).then(() => true);
    }
    return extra(route, request, url);
  };
  const { page, failures, close } = await walk(browser, `/hubs/${IT.id}/desk?queue=${id(500)}`, { extra: filtering });
  t.after(close);
  await main(page).getByRole('heading', { name: 'About to breach' }).waitFor({ timeout: 10_000 });
  const head = main(page).locator('header.head');
  assert.equal(await head.locator('[data-tour="queue-save"]').count(), 0, 'Save as queue before the filter differs from the queue');
  await head.locator('[data-tour="queue-filter"]').click();
  const line = main(page).locator('[data-queue-filter]').getByRole('textbox', { name: 'As a line' });
  await line.fill('title = "Printer"');
  await line.press('Enter');
  // The queue is asked again with the condition joined to its own.
  await head.locator('[data-tour="queue-save"]').waitFor({ timeout: 10_000 });
  const narrowed = asked.filter((body) => JSON.stringify(body.filter ?? {}).includes('"title"'));
  assert.ok(narrowed.length > 0, 'the queue was not asked with the filter');
  assert.ok(JSON.stringify(narrowed.at(-1).filter).includes('sla_standing'), 'the filter replaced the queue\'s own conditions');
  await head.locator('[data-tour="queue-save"]').click();
  const dialog = page.getByRole('dialog', { name: 'Save as queue' });
  await dialog.getByRole('textbox', { name: 'Name of the queue' }).fill('Printers');
  await dialog.locator('[data-save-queue]').click();
  await at(page, `/hubs/${IT.id}/desk?queue=${SAVED.id}`);
  assert.equal(saved.length, 1);
  assert.equal(saved[0].scope_type, 'HUB');
  assert.equal(saved[0].scope_id, IT.id);
  assert.equal(saved[0].queue, true);
  assert.equal(saved[0].name, 'Printers');
  assert.equal(saved[0].query.scope_container_id, IT.id);
  assert.ok(JSON.stringify(saved[0].query.filter).includes('"title"'), 'the saved queue lost the filter');
  // The new queue stands in the sidebar under *Queues*.
  await side(page).getByRole('region', { name: IT.name }).getByRole('link', { name: /^Printers/ }).waitFor({ timeout: 10_000 });
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 18: a new collection offers the Desk kind where the hub has a portal type, and the desk offers it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const MADE = { id: id(230), type: 'COLLECTION', parent_id: PD.id, name: 'Help', order_key: 'a9', version: 1 };
  const making = async (route, request, url) => {
    const path = url.pathname;
    if (path.endsWith('/api/v1/presets')) return route.fulfill({ json: PRESETS }).then(() => true);
    if (path.endsWith('/api/v1/containers') && request.method() === 'POST') {
      written.push({ path: '/containers', body: request.postDataJSON() });
      return route.fulfill({ status: 201, json: MADE }).then(() => true);
    }
    if (path.endsWith(`/api/v1/containers/${MADE.id}/structure`) && request.method() === 'PATCH') {
      written.push({ path: '/structure', body: request.postDataJSON(), ifMatch: request.headers()['if-match'] });
      return route.fulfill({ json: { ...MADE, version: 2 } }).then(() => true);
    }
    return extra(route, request, url);
  };
  const { page, failures, close } = await walk(browser, `/hubs/${PD.id}`, { extra: making });
  t.after(close);
  await newInHead(page).first().click({ timeout: 15_000 });
  await page.getByRole('menuitem', { name: 'Collection', exact: true }).click();
  const dialog = page.locator('dialog[open]');
  await kindChoice(dialog, 'Desk').waitFor({ timeout: 10_000 });
  const card = dialog.locator('[data-collection-kind="desk"]');
  assert.ok((await card.textContent()).includes('Requests through the portal'), 'the desk card says what it is for');
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/live-newcoll.png` });
  await dialog.getByRole('textbox', { name: 'Name' }).fill('Help');
  await kindChoice(dialog, 'Desk').click();
  await dialog.locator('[data-create-container]').click();
  await page.waitForFunction(() => document.querySelector('dialog[open]') === null, null, { timeout: 10_000 });
  const created = written.find((each) => each.path === '/containers');
  assert.equal(created?.body.preset, 'TASKS', 'a desk starts from Tasks');
  const shaped = written.find((each) => each.path === '/structure');
  assert.deepEqual(shaped?.body, { settings: { entry_types: { TASK: ['incident'] } } }, 'the desk offers the hub\'s portal type by name');
  assert.deepEqual(failures, []);
});

test('UC-WRK-36 check 18: without a portal type in the hub the Desk kind says what it needs and cannot be chosen', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const noPortal = async (route, request, url) => {
    const path = url.pathname;
    if (path.endsWith('/api/v1/presets')) return route.fulfill({ json: PRESETS }).then(() => true);
    if (path.endsWith('/api/v1/entry-types')) return route.fulfill({ json: { data: [{ key: 'story', name: 'Story', level: 'TASK', hub_id: null }], page: { next_cursor: null, has_more: false } } }).then(() => true);
    return extra(route, request, url);
  };
  const { page, failures, close } = await walk(browser, `/hubs/${PD.id}`, { extra: noPortal });
  t.after(close);
  await newInHead(page).first().click({ timeout: 15_000 });
  await page.getByRole('menuitem', { name: 'Collection', exact: true }).click();
  const dialog = page.locator('dialog[open]');
  await kindChoice(dialog, 'Desk').waitFor({ timeout: 10_000 });
  assert.ok((await dialog.locator('[data-collection-kind="desk"]').textContent()).includes('Needs a portal type in the hub'));
  assert.equal(await kindChoice(dialog, 'Desk').getAttribute('aria-disabled'), 'true', 'a desk without a portal type can be chosen');
  assert.deepEqual(failures, []);
});

test('a new collection of the kind Scrum starts from Project and opens on its sprint view', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const written = [];
  const MADE = { id: id(231), type: 'COLLECTION', parent_id: PD.id, name: 'Platform', order_key: 'a9', version: 1 };
  const making = async (route, request, url) => {
    const path = url.pathname;
    if (path.endsWith('/api/v1/presets')) return route.fulfill({ json: PRESETS }).then(() => true);
    if (path.endsWith('/api/v1/containers') && request.method() === 'POST') {
      written.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: MADE }).then(() => true);
    }
    return extra(route, request, url);
  };
  const { page, failures, close } = await walk(browser, `/hubs/${PD.id}`, { extra: making });
  t.after(close);
  await newInHead(page).first().click({ timeout: 15_000 });
  await page.getByRole('menuitem', { name: 'Collection', exact: true }).click();
  const dialog = page.locator('dialog[open]');
  await dialog.getByRole('heading', { name: `New collection in ${PD.name}` }).waitFor({ timeout: 10_000 });
  await kindChoice(dialog, 'Scrum').waitFor({ timeout: 10_000 });
  await dialog.getByRole('textbox', { name: 'Name' }).fill('Platform');
  await kindChoice(dialog, 'Scrum').click();
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  await at(page, `/collections/${MADE.id}/sprint`);
  assert.equal(written[0]?.preset, 'PROJECT', 'Scrum starts from Project');
  assert.deepEqual(failures, []);
});
