// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The frame on the shell wave (F9-06, ADR-0061 decision 1): one list of destinations, drawn three
// ways, and the proof that the same destinations are reachable on each. At 375 px the primary
// group is the bottom bar, the tree is behind ☰ and the account group behind "You"; at 600 px
// the drawer holds both groups and the avatar is in the bar; at 905 and 1280 px the navigation is
// pinned and folds to a rail. On every width: the search exists once and the bar has no field for
// it, `[data-tour="hubs"]` is on something the tour can point at, and nothing scrolls sideways.
//
// Chromium only: what differs by width is layout, and the layout has no engine-specific part;
// the engines job loads the bundle in all three.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Shell Walker', email: 'shell@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e0-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'House', order_key: 'a0', version: 1 };
const COLLECTION = { id: '01a0e2e0-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Kitchen', order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

/** The API at the network edge: an account, a hub with one collection, and empty pages for the rest. */
async function stub(route) {
  const url = new URL(route.request().url());
  const path = url.pathname;
  if (path.endsWith('/api/v1/stream')) return route.abort();
  if (path.endsWith('/api/v1/sync:snapshot')) {
    const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
  }
  if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
  if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (path.endsWith(`/api/v1/containers/${COLLECTION.id}`)) return route.fulfill({ json: COLLECTION });
  // The collection's lists that are arrays rather than pages.
  if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
  return route.fulfill({ json: PAGE });
}

const served = await serve(DIST);
test.after(() => served.close());

/** The destinations every width has to offer, by the name a reader sees. */
const PRIMARY = ['Workspace', 'Search', 'Jumble'];
const ACCOUNT_GROUP = ['Your settings', 'This installation', 'Administration', 'Take the tour again', 'Sign out'];

async function open(browser, width) {
  const context = await browser.newContext({ viewport: { width, height: 800 } });
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/`);
  await page.getByRole('button', { name: ACCOUNT.display_name }).or(page.getByRole('link', { name: 'You' })).first().waitFor({ timeout: 15_000 });
  return { page, failures, close: () => context.close() };
}

/** What is common to every width: no second search, the tour's target, no sideways scroll. */
async function common(page, width) {
  assert.equal(await page.locator('header input, header [type=search]').count(), 0, `${width}: the bar carries a search field`);
  assert.equal(await page.locator('[data-tour="hubs"]').count(), 1, `${width}: the tour's hubs step has ${await page.locator('[data-tour="hubs"]').count()} targets`);
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, `${width}: the page scrolls sideways (${scroll.width} of ${scroll.viewport})`);
  assert.equal(await page.getByRole('link', { name: 'Skip to the content' }).count(), 1, `${width}: no skip link`);
  assert.equal(await page.getByRole('link', { name: 'Accessibility statement, on hubtask.eu' }).count(), 1, `${width}: no footer`);
  assert.equal(await page.getByRole('status').filter({ hasText: /Connected|Reconnecting|Offline|synced|copy/ }).count() > 0, true, `${width}: no sync line`);
}

test('chromium: 375 px — the bottom bar, the tree behind ☰, the account group behind "You"', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 375);
  t.after(close);
  await common(page, 375);

  // Spacious for a thumb, and the bar the thumb reaches.
  assert.equal(await page.locator('.frame[data-density="spacious"]').count(), 1, 'the frame is not spacious below medium');
  const bar = page.getByRole('navigation', { name: 'Sections' });
  for (const name of [...PRIMARY, 'You']) assert.equal(await bar.getByRole('link', { name }).count(), 1, `${name} is not in the bottom bar`);
  assert.equal(await bar.getByRole('link', { name: 'Workspace' }).getAttribute('aria-current'), 'page');

  // ☰ is the tour's target and opens the tree alone: the hub, the trash last, and none of the
  // three destinations the bar below already has.
  const toggle = page.getByRole('button', { name: 'Open the navigation' });
  assert.equal(await toggle.getAttribute('data-tour'), 'hubs');
  await toggle.click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  const rows = await drawer.getByRole('treeitem').allTextContents();
  assert.deepEqual(rows.map((row) => row.trim()), ['House', 'Trash'], `the drawer holds ${JSON.stringify(rows)}`);
  assert.equal(await drawer.getByRole('button', { name: 'Create hub' }).count(), 1, 'the drawer has no way to create a hub');
  // A navigation closes it, and the collection inside the hub is reachable through it.
  await drawer.getByRole('treeitem', { name: 'House' }).click();
  await drawer.getByRole('treeitem', { name: 'Kitchen' }).waitFor({ timeout: 5_000 });
  await drawer.getByRole('treeitem', { name: 'Kitchen' }).click();
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`);
  assert.equal(await bar.getByRole('link', { name: 'Workspace' }).getAttribute('aria-current'), 'page', 'a collection is inside the workspace');

  // "You" opens the account group as a sheet: the name at its head, the five rows, sign out last.
  await bar.getByRole('link', { name: 'You' }).click();
  const sheet = page.locator('dialog[open]');
  await sheet.waitFor({ timeout: 5_000 });
  assert.equal(await sheet.getByText(ACCOUNT.display_name).count(), 1, 'the sheet does not name the person');
  const items = await sheet.getByRole('button').allTextContents();
  const named = items.map((item) => item.trim()).filter((item) => ACCOUNT_GROUP.includes(item));
  assert.deepEqual(named, ACCOUNT_GROUP, `the sheet holds ${JSON.stringify(items)}`);
  await sheet.getByRole('button', { name: 'Your settings' }).click();
  await sheet.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, '/profile');
  assert.equal(await bar.getByRole('link', { name: 'You' }).getAttribute('aria-current'), 'page');

  // The trash, through the tree, from anywhere.
  await toggle.click();
  await page.locator('dialog[open]').getByRole('treeitem', { name: 'Trash' }).click();
  assert.equal(new URL(page.url()).pathname, '/trash');

  // The bar gives way while a field has focus - the keyboard has that part of the screen.
  await page.goto(`${served.origin}/search`);
  const field = page.locator('main input, main textarea').first();
  await field.waitFor({ timeout: 10_000 });
  await field.focus();
  await page.waitForFunction(() => {
    const nav = document.querySelector('nav[aria-label="Sections"]');
    return nav && getComputedStyle(nav).opacity === '0';
  }, null, { timeout: 5_000 }).catch(() => assert.fail('the bottom bar did not give way to the keyboard'));

  assert.deepEqual(failures, []);
});

test('chromium: 600 px — the drawer holds both groups, the avatar is in the bar, no bottom bar', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 600);
  t.after(close);
  await common(page, 600);

  assert.equal(await page.getByRole('navigation', { name: 'Sections' }).count(), 0, 'a bottom bar on a tablet');
  assert.equal(await page.locator('.frame[data-density="spacious"]').count(), 0, 'spacious with a mouse on a tablet');
  await page.getByRole('button', { name: 'Open the navigation' }).click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  const rows = (await drawer.getByRole('treeitem').allTextContents()).map((row) => row.trim());
  assert.deepEqual(rows, [...PRIMARY, 'House', 'Trash'], `the drawer holds ${JSON.stringify(rows)}`);
  await drawer.getByRole('treeitem', { name: 'Search' }).click();
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, '/search');

  await page.getByRole('button', { name: ACCOUNT.display_name }).click();
  const menu = page.getByRole('menu', { name: 'You' });
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual((await menu.getByRole('menuitem').allTextContents()).map((item) => item.trim()), ACCOUNT_GROUP);
  await page.keyboard.press('Escape');
  assert.deepEqual(failures, []);
});

for (const width of [905, 1280]) {
  test(`chromium: ${width} px — the navigation pinned, folding to a rail, the avatar in the bar`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, close } = await open(browser, width);
    t.after(close);
    await common(page, width);

    assert.equal(await page.getByRole('navigation', { name: 'Sections' }).count(), 0, `${width}: a bottom bar on a desk`);
    assert.equal(await page.getByRole('button', { name: 'Open the navigation' }).count(), 0, `${width}: a drawer trigger on a desk`);
    const tree = page.getByRole('navigation', { name: 'Workspace' });
    const rows = (await tree.getByRole('treeitem').allTextContents()).map((row) => row.trim());
    assert.deepEqual(rows, [...PRIMARY, 'House', 'Trash'], `${width}: the tree holds ${JSON.stringify(rows)}`);
    assert.equal(await page.locator('aside[data-tour="hubs"]').count(), 1, `${width}: the tour's target is not the pinned navigation`);
    assert.equal(await tree.getByRole('treeitem', { name: 'Workspace' }).getAttribute('aria-current'), 'page');

    // The rail: the same tree, folded to its marks, and back. The width is the token's.
    const aside = page.locator('aside.sidenav');
    const pinned = await aside.evaluate((el) => el.getBoundingClientRect().width);
    await page.getByRole('button', { name: 'Collapse the navigation' }).click();
    const rail = await aside.evaluate((el) => el.getBoundingClientRect().width);
    assert.ok(rail < pinned / 2, `${width}: the rail is ${rail} px against ${pinned} px pinned`);
    assert.equal(await tree.getByRole('treeitem').count(), rows.length, `${width}: the rail lost rows`);
    await page.getByRole('button', { name: 'Expand the navigation' }).click();
    assert.equal(await aside.evaluate((el) => el.getBoundingClientRect().width), pinned);

    // Every destination, through the tree and the menu.
    await tree.getByRole('treeitem', { name: 'Jumble' }).click();
    assert.equal(new URL(page.url()).pathname, '/jumble');
    await tree.getByRole('treeitem', { name: 'Trash' }).click();
    assert.equal(new URL(page.url()).pathname, '/trash');
    await page.getByRole('button', { name: ACCOUNT.display_name }).click();
    const menu = page.getByRole('menu', { name: 'You' });
    await menu.waitFor({ timeout: 5_000 });
    assert.deepEqual((await menu.getByRole('menuitem').allTextContents()).map((item) => item.trim()), ACCOUNT_GROUP);
    await menu.getByRole('menuitem', { name: 'This installation' }).click();
    assert.equal(new URL(page.url()).pathname, '/installation');
    assert.deepEqual(failures, []);
  });
}
