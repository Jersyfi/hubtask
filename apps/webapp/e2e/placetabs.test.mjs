// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A hub's tab strip (user-experience.md §2.4): its views in one fixed order, each only where its
// module is on at the hub; the place's menu holds no product; a tab's screen draws the same strip
// with the tab current and its own views as a second strip. Walked at desk and phone width, light
// and dark.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PERMISSIONS, fallback } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Tab Walker', email: 'tabs@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e0-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'Product development and delivery for the whole company', order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

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
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [] } });
    // The two time screens a hub's *Time* tab holds, empty.
    if (path.endsWith(`/api/v1/plans/${HUB.id}/actuals`)) return route.fulfill({ json: { plan_id: HUB.id, data: [], estimate_hours: 0, minutes: 0 } });
    if (path.endsWith(`/api/v1/plans/${HUB.id}/billing-rates`)) return route.fulfill({ json: { data: [] } });
    if (path.endsWith(`/api/v1/plans/${HUB.id}/budget`)) {
      return route.fulfill({ json: { plan_id: HUB.id, budgeted: false, budget_minutes: null, budget_amount_minor: null, currency: null, version: 0, minutes: 0, amount_minor: 0, unpriced_minutes: 0, over_time: false, over_money: false, weeks: [] } });
    }
    if (path.endsWith(`/api/v1/plans/${HUB.id}/workload`)) return route.fulfill({ json: { plan_id: HUB.id, aggregated: false, cells: [], team: [], unscheduled_minutes: 0 } });
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


const PLACE_TABS = (page) => page.getByRole('main').getByRole('navigation', { name: HUB.name, exact: true });

for (const [width, scheme] of [[1280, 'light'], [1280, 'dark'], [375, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: a hub's views are one strip, never head buttons`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    // The workspace has Whiteboards on; the hub narrows it away (UC-INS-21 check 3).
    const { page, failures, close } = await open(browser, {
      width, scheme, path: `/hubs/${HUB.id}`,
      modulesOn: ['GOALS', 'TIME', 'WHITEBOARD', 'KNOWLEDGE', 'PLANNING'],
      hubModules: ['GOALS', 'TIME', 'KNOWLEDGE', 'PLANNING'],
    });
    t.after(close);
    const strip = PLACE_TABS(page);
    await strip.waitFor({ timeout: 10_000 });
    const tabs = (await strip.getByRole('link').allTextContents()).map((each) => each.trim());
    assert.deepEqual(tabs, ['Overview', 'Plan', 'Pages', 'Time', 'Goals'], JSON.stringify(tabs));
    assert.equal(await strip.getByRole('link', { name: 'Overview' }).getAttribute('aria-current'), 'page');
    await noSidewaysScroll(page, `hub ${width} ${scheme}`);
    // No product is a head button (the place's menu is pinned by placeTabs.test.ts).
    for (const product of ['Plan', 'Pages', 'Goals', 'Dashboards', 'Workload', 'Releases', 'Estimates and actuals']) {
      assert.equal(await page.getByRole('main').getByRole('button', { name: product, exact: true }).count(), 0, `${product} is a head button`);
    }

    await strip.getByRole('link', { name: 'Time' }).click();
    await page.waitForFunction((id) => location.pathname === `/hubs/${id}/actuals`, HUB.id, { timeout: 5_000 });
    await PLACE_TABS(page).getByRole('link', { name: 'Time' }).waitFor({ timeout: 10_000 });
    assert.equal(await PLACE_TABS(page).getByRole('link', { name: 'Time' }).getAttribute('aria-current'), 'page', 'the tab is not current on its screen');
    const views = page.getByRole('main').getByRole('navigation', { name: 'Time', exact: true });
    assert.equal(await views.getByRole('link', { name: 'Estimates and actuals' }).getAttribute('aria-current'), 'page');
    await views.getByRole('link', { name: 'Workload' }).click();
    await page.waitForFunction((id) => location.pathname === `/hubs/${id}/workload`, HUB.id, { timeout: 5_000 });
    await noSidewaysScroll(page, `workload ${width} ${scheme}`);
    assert.deepEqual(failures, []);
  });
}

// §2.4, §2.5: the tree mirrors an open hub's views as quieter rows after its collections, the one
// the reader is on current; the hub's long name is one line with the whole name as its tooltip.
test('chromium 1280 px: the tree mirrors the hub\'s views, and its name is one line', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, { width: 1280, path: `/hubs/${HUB.id}/workload`, modulesOn: ['TIME', 'KNOWLEDGE'] });
  t.after(close);
  const tree = page.getByRole('navigation', { name: 'Workspace', exact: true });
  const time = tree.locator(`[data-node="${HUB.id}/tab/time"]`);
  await time.waitFor({ timeout: 10_000 });
  assert.equal(await time.getAttribute('aria-current'), 'page', 'the hub\'s Time row is not current on its screen');
  const views = (await tree.locator(`[data-node^="${HUB.id}/tab/"]`).allTextContents()).map((each) => each.trim());
  assert.deepEqual(views, ['Pages', 'Time'], JSON.stringify(views));
  const name = tree.getByRole('treeitem', { name: HUB.name }).locator('.label');
  assert.equal(await name.getAttribute('title'), HUB.name);
  const box = await name.evaluate((element) => ({ lines: Math.round(element.getBoundingClientRect().height / parseFloat(getComputedStyle(element).lineHeight)), cut: element.scrollWidth > element.clientWidth }));
  assert.equal(box.lines, 1, 'the hub\'s name breaks onto a second line');
  await tree.locator(`[data-node="${HUB.id}/tab/pages"]`).click();
  await page.waitForFunction((id) => location.pathname === `/hubs/${id}/pages`, HUB.id, { timeout: 5_000 });
  assert.deepEqual(failures, []);
});
