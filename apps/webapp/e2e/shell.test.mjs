// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The frame on the shell wave (F9-06, ADR-0061 decision 1): one list of destinations, drawn three
// ways, and the proof that the same destinations are reachable on each. At 375 px the primary
// group is the bottom bar, the tree is behind ☰ and the account group behind "You"; at 600 px
// the drawer holds both groups and the avatar is in the bar; at 905 and 1280 px the navigation is
// pinned and folds to a rail. On every width the search exists exactly once (ADR-0063 decision 4):
// the bar's field from `medium` up, where the tree therefore has no Search row, and the bottom
// bar's destination on `compact`, where the bar has no room for a field. Also on every width:
// `[data-tour="hubs"]` is on something the tour can point at, and nothing scrolls sideways.
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
  // The manifest, for the one thing the frame reads out of it here: the product's version, which
  // the account menu's foot carries so that somebody reporting a problem can quote it.
  if (path.endsWith('/api/v1/meta/capabilities')) {
    return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], supported_locales: [{ locale: 'en', direction: 'ltr' }] } });
  }
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
const PRIMARY = ['Overview', 'Search', 'Jumble'];
/** The same list where the bar carries the entry to search: the row for it would be the second. */
const PLACES = ['Overview', 'Jumble'];
// The account group, in its order. "This installation" is at the foot as "About Hubtask" with the
// version beside it (ADR-0063 decision 6) — the same route under a name somebody would look for.
const ACCOUNT_GROUP = ['Your settings', 'Workspace administration', 'Take the tour again', 'Sign out', 'About Hubtask · 0.9.0'];

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

/** What is common to every width: one search, the tour's target, no sideways scroll. */
async function common(page, width) {
  // One entry to search, and which one depends on the width. Counted here rather than asserted
  // per test, because "exactly one" is the rule both halves of decision 4 exist to keep.
  const inBar = await page.locator('header form[role="search"] input').count();
  const inNav = await page.getByRole('treeitem', { name: 'Search' }).count() + await page.getByRole('link', { name: 'Search', exact: true }).count();
  assert.equal(inBar, width < 600 ? 0 : 1, `${width}: the bar has ${inBar} search fields`);
  assert.equal(inBar + inNav, 1, `${width}: ${inBar + inNav} entries to search`);
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
  assert.equal(await bar.getByRole('link', { name: 'Overview' }).getAttribute('aria-current'), 'page');

  // ☰ is the tour's target and opens the tree alone: the hub, the trash last, and none of the
  // three destinations the bar below already has.
  const toggle = page.getByRole('button', { name: 'Open the navigation' });
  assert.equal(await toggle.getAttribute('data-tour'), 'hubs');
  await toggle.click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  const rows = await drawer.getByRole('treeitem').allTextContents();
  assert.deepEqual(rows.map((row) => row.trim()), ['House', 'Archive', 'Trash'], `the drawer holds ${JSON.stringify(rows)}`);
  assert.equal(await drawer.getByRole('button', { name: 'Create hub' }).count(), 1, 'the drawer has no way to create a hub');
  // A navigation closes it, and the collection inside the hub is reachable through it.
  await drawer.getByRole('treeitem', { name: 'House' }).click();
  await drawer.getByRole('treeitem', { name: 'Kitchen' }).waitFor({ timeout: 5_000 });
  await drawer.getByRole('treeitem', { name: 'Kitchen' }).click();
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`);
  assert.equal(await bar.getByRole('link', { name: 'Overview' }).getAttribute('aria-current'), 'page', 'a collection is inside the workspace');

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
  assert.deepEqual(rows, [...PLACES, 'House', 'Archive', 'Trash'], `the drawer holds ${JSON.stringify(rows)}`);
  // Search is the bar's field here, and it leads to the same destination the row used to.
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
  await page.locator('header form[role="search"] input').fill('milk');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => location.pathname === '/search', null, { timeout: 5_000 });
  assert.equal(new URL(page.url()).search, '', 'the words reached the address bar');

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
    // Two trees in one column, and one list behind them: the places and the hubs, then the
    // `keeping` band pinned to the foot (ADR-0063 decision 1). The rows are the same rows.
    const tree = page.getByRole('navigation', { name: 'Workspace' });
    const keeping = page.getByRole('navigation', { name: 'What is kept' });
    const rows = (await tree.getByRole('treeitem').allTextContents()).map((row) => row.trim());
    assert.deepEqual(rows, [...PLACES, 'House'], `${width}: the tree holds ${JSON.stringify(rows)}`);
    assert.deepEqual((await keeping.getByRole('treeitem').allTextContents()).map((row) => row.trim()), ['Archive', 'Trash'], `${width}: the keeping band`);
    // And it is at the foot: below every row of the tree above it, and at the bottom of the column.
    const foot = await page.evaluate(() => {
      const aside = document.querySelector('aside.sidenav');
      const band = aside?.querySelector('.keeping')?.getBoundingClientRect();
      const last = [...(aside?.querySelectorAll('nav[aria-label="Workspace"] [role="treeitem"]') ?? [])].at(-1)?.getBoundingClientRect();
      return band && last ? { below: Math.round(band.top - last.bottom), toBottom: Math.round(aside.getBoundingClientRect().bottom - band.bottom) } : null;
    });
    assert.ok(foot !== null && foot.below > 100, `${width}: the keeping band is ${JSON.stringify(foot)} from the hubs`);
    assert.ok(foot.toBottom < 40, `${width}: the keeping band is ${foot.toBottom}px above the column's bottom`);
    assert.equal(await page.locator('aside[data-tour="hubs"]').count(), 1, `${width}: the tour's target is not the pinned navigation`);
    assert.equal(await tree.getByRole('treeitem', { name: 'Overview' }).getAttribute('aria-current'), 'page');

    // The rail: the same tree, folded to its marks, and back. The width is the token's.
    const aside = page.locator('aside.sidenav');
    const pinned = await aside.evaluate((el) => el.getBoundingClientRect().width);
    await page.getByRole('button', { name: 'Collapse the navigation' }).click();
    const rail = await aside.evaluate((el) => el.getBoundingClientRect().width);
    assert.ok(rail < pinned / 2, `${width}: the rail is ${rail} px against ${pinned} px pinned`);
    assert.equal(await tree.getByRole('treeitem').count(), rows.length, `${width}: the rail lost rows`);
    assert.equal(await keeping.getByRole('treeitem').count(), 2, `${width}: the rail lost the keeping band`);
    // And every mark is **drawn**, inside the column. Folding used to leave the twist in front of
    // it and clip the half that stuck out, so the rail was a column of slivers (issue 915).
    const marks = await aside.evaluate((el) => {
      const column = el.getBoundingClientRect();
      return [...el.querySelectorAll('[role="treeitem"] svg')].map((mark) => {
        const box = mark.getBoundingClientRect();
        return { inside: box.left >= column.left && box.right <= column.right, width: Math.round(box.width) };
      });
    });
    const railRows = await aside.getByRole('treeitem').count();
    assert.equal(marks.length, railRows, `${width}: the rail drew ${marks.length} marks for ${railRows} rows`);
    assert.deepEqual(marks.filter((mark) => !mark.inside || mark.width === 0), [], `${width}: a mark of the rail is clipped or missing`);
    // The label is announced although it is not drawn, so the rail is navigable by name.
    assert.equal(await tree.getByRole('treeitem', { name: 'Jumble' }).count(), 1, `${width}: the rail's rows lost their names`);

    // A branch pressed in the rail opens its subtree beside the column, so nothing is unreachable
    // while the navigation is folded (ADR-0063 decision 2). Escape closes it and focus comes back.
    const hubMark = aside.locator(`[data-node="${HUB.id}"]`);
    await hubMark.click();
    const flyout = page.locator('.flyout');
    await flyout.waitFor({ timeout: 5_000 });
    // Opening it is also what asks the server for the level, so the collections arrive after the
    // panel does — a flyout that only set its own state would open beside a hub nobody had read.
    await flyout.getByRole('treeitem', { name: COLLECTION.name }).waitFor({ timeout: 5_000 })
      .catch(() => assert.fail(`${width}: the flyout does not hold the hub's collections`));
    await page.keyboard.press('Escape');
    await flyout.waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('data-node')), HUB.id, `${width}: focus did not come back to the mark`);
    await page.getByRole('button', { name: 'Expand the navigation' }).click();
    assert.equal(await aside.evaluate((el) => el.getBoundingClientRect().width), pinned);

    // The connection is one mark in the bar, and no line of the page (ADR-0063 decision 5). At its
    // quietest it says nothing until it is pressed; what the line used to print is behind it.
    const mark = page.getByRole('banner', { name: 'Application bar' }).locator('.trigger');
    assert.equal(await mark.count(), 1, `${width}: the connection is not in the bar`);
    assert.equal(await mark.getAttribute('aria-label'), 'Connected');
    assert.ok(await mark.evaluate((el) => el.hasAttribute('data-quiet')), `${width}: the quiet case is not quiet`);
    assert.equal(await page.locator('main').getByText(/Connected|Reconnecting/).count(), 0, `${width}: a line of the page still says it`);
    await mark.click();
    const surface = page.getByRole('dialog', { name: 'The copy and the server' });
    await surface.waitFor({ timeout: 5_000 });
    assert.equal(await surface.getByText('Connected').count(), 1, `${width}: what it opened does not say the state`);
    await page.keyboard.press('Escape');
    await surface.waitFor({ state: 'hidden', timeout: 5_000 }).catch(() => {});

    // Every destination, through the tree and the menu.
    await tree.getByRole('treeitem', { name: 'Jumble' }).click();
    assert.equal(new URL(page.url()).pathname, '/jumble');
    await keeping.getByRole('treeitem', { name: 'Trash' }).click();
    assert.equal(new URL(page.url()).pathname, '/trash');
    // The trigger is the avatar, named by the person; the name is beside it only from `large`,
    // and the address is inside rather than in the frame (ADR-0063 decision 6).
    const trigger = page.getByRole('button', { name: ACCOUNT.display_name });
    // The avatar's initials are always drawn; the name in words only from `large`.
    assert.equal(
      (await trigger.textContent()).includes(ACCOUNT.display_name),
      width >= 1240,
      `${width}: the trigger draws "${(await trigger.textContent()).trim()}"`,
    );
    assert.equal(await page.getByRole('banner', { name: 'Application bar' }).getByText(ACCOUNT.email).count(), 0, `${width}: the bar carries an address`);
    await trigger.click();
    const menu = page.getByRole('menu', { name: 'You' });
    await menu.waitFor({ timeout: 5_000 });
    assert.deepEqual((await menu.getByRole('menuitem').allTextContents()).map((item) => item.trim()), ACCOUNT_GROUP);
    // Whose menu it is, said where it is opened and nowhere else.
    const whose = page.locator('.surface').filter({ has: page.getByRole('menu', { name: 'You' }) });
    assert.equal(await whose.getByText(ACCOUNT.email).count(), 1, `${width}: the menu does not say whose it is`);
    await menu.getByRole('menuitem', { name: /About Hubtask/ }).click();
    assert.equal(new URL(page.url()).pathname, '/installation');
    assert.deepEqual(failures, []);
  });
}
