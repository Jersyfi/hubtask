// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The navigation concept's screens (step 5): the hub's overview without tabs, each hub tool's page
// with its one view bar, the queue page, a collection's one view bar, a page without a tree of its
// own, *All contents*, ＋ under a page, and the former hub tabs' addresses (UC-WRK-36 checks 9, 10
// and 12, UC-SVC-03 check 10, UC-DOC-01 check 6, UC-CNV-10 check 6). Walked at desk width;
// `SHOTS=<dir>` also writes the screenshots the mockup is compared with.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chromium } from 'playwright';

import { ALL, DESK, IT, PD, PD_PAGES, WEB, id, open, served, side, texts } from './kompassstub.mjs';

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

test('UC-WRK-36 check 9: a collection draws one view bar - board, sprint, list, timeline, time - and its old plan is the timeline', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await walk(browser, `/collections/${WEB.id}`);
  t.after(close);
  const bar = main(page).getByRole('navigation', { name: WEB.name });
  await bar.getByRole('link', { name: 'Sprint' }).waitFor({ timeout: 10_000 });
  assert.deepEqual(await texts(bar.getByRole('link')), ['Board', 'Sprint', 'List', 'Timeline', 'Time']);
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
