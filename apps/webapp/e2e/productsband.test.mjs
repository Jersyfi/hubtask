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
