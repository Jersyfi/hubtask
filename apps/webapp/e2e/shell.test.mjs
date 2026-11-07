// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The frame on the shell wave (ADR-0061 decision 1): one list of destinations, drawn three
// ways, and the proof that the same destinations are reachable on each. At 375 px the primary
// group is the bottom bar, the tree is behind ☰ and the account group behind "You"; at 600 px
// the drawer holds both groups and the avatar is in the bar; at 905 and 1280 px the navigation is
// pinned and folds to a rail. On every width there is a way to search, and from `medium` up there
// are two of them on purpose: the bar's field, which is where a search is typed, and the tree's
// Search row, which is the *place* - a reader who wants the filter, or the search they were sent,
// goes there with nothing typed. On `compact` the bar has no room for a field and the bottom bar
// carries Search as a destination. Also on every width:
// `[data-tour="hubs"]` is on something the tour can point at, and nothing scrolls sideways.
//
// Chromium only: what differs by width is layout, and the layout has no engine-specific part;
// the engines job loads the bundle in all three.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PERMISSIONS, fallback, unstubbedSoFar } from './fixture.mjs';
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
  // The stream, **accepted and empty**: the connection is what the mark in the bar reads,
  // so a walk that refused it would draw *Reconnecting…* on every screenshot of every
  // screen. It carries the server's own reconnect suggestion and no records; what a walk needs
  // from the stream is that it was opened.
  if (path.endsWith('/api/v1/stream')) {
    return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
  }
  if (path.endsWith('/api/v1/sync:snapshot')) {
    const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
  }
  if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
  // The manifest, for the one thing the frame reads out of it here: the product's version, which
  // the account menu's foot carries so that somebody reporting a problem can quote it.
  if (path.endsWith('/api/v1/meta/capabilities')) {
    return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: PERMISSIONS.OWNER } });
  }
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
  if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (path.endsWith(`/api/v1/containers/${COLLECTION.id}`)) return route.fulfill({ json: COLLECTION });
  // The collection's lists that are arrays rather than pages.
  if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
  // The frame's own reads, in the shapes the API answers them, and a record of anything this
  // walk never prepared: a guess nobody notices is what `fallback` exists to prevent.
  return fallback(route, route.request(), path);
}

const served = await serve(DIST);
test.after(() => served.close());

/** The one link out of the application, by the name a reader sees, wherever it is offered. */
const STATEMENT = 'Accessibility statement, on hubtask.eu';

/**
 * The destinations every width has to offer, by the name a reader sees.
 *
 * One list, drawn three ways - and the tree draws all three of them, whatever the bar carries: a
 * field is where a search is typed and Search is the place it is built, which is why they are no
 * longer counted as one thing.
 */
const PRIMARY = ['Today', 'Search', 'Inbox'];
/** The sidebar's fixed top: the places without Search, which is the bar's field (the navigation concept, §2.2). */
const FOR_YOU = ['Today', 'Inbox'];
// The account group, in its order. "This installation" is "About Hubtask" — the same route under
// a name somebody would look for (ADR-0063 decision 6) — and it carries no version: a build
// reference is not what a menu row is called, and the page behind it quotes the version whole
// (ADR-0065 decision 5). Signing out is last, because it is the last thing a reader does.
// Only what is about the person (user-experience.md §2.3); the administration is a row of the
// column's workspace band.
const ACCOUNT_GROUP = ['Your settings', 'Appearance', 'Help and shortcuts', 'About Hubtask', 'Sign out'];

async function open(browser, width, options = {}) {
  const context = await browser.newContext({ viewport: { width, height: 800 } });
  await context.route('**/api/v1/**', stub);
  if (options.signedIn !== false) {
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
  }
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/`);
  // Signed out there is no account group to wait for; the screen that asks for a credential is
  // what arrives, and its heading is what says it has.
  if (options.signedIn === false) await page.getByRole('heading', { level: 1 }).waitFor({ timeout: 15_000 });
  else await page.getByRole('button', { name: ACCOUNT.display_name }).or(page.getByRole('link', { name: 'You' })).first().waitFor({ timeout: 15_000 });
  // The context travels with the page: `setOffline` is a property of the context, and what a
  // device with no network draws is part of what the frame owes the reader.
  return { context, page, failures, close: () => context.close() };
}

/** What is common to every width: a way to search, the tour's target, no sideways scroll. */
async function common(page, width) {
  // A way to search on every width, and the field exactly where there is room for it. The tree's
  // row is a destination rather than a second field, so it is counted separately: what must never
  // happen is *two fields*, and what must always happen is at least one way through.
  // From `expanded` the field; on `medium` its look, opening the search over the page.
  const inBar = await page.locator('header form[role="search"] input').count() + await page.locator('header button.seek').count();
  const inNav = await page.getByRole('treeitem', { name: 'Search' }).count() + await page.getByRole('link', { name: 'Search', exact: true }).count();
  assert.equal(inBar, width < 600 ? 0 : 1, `${width}: the bar has ${inBar} search fields`);
  // On `compact` the bottom bar carries Search and it is the only way through, so it is counted
  // here. Above that the tree carries it - and at 600 px the tree is behind ☰, so counting it
  // here would count a closed drawer: that width's own test opens it and reads the rows.
  if (width < 600) assert.equal(inNav, 1, `${width}: ${inNav} Search destinations`);
  // Centred, measured rather than looked at. It is the kind of thing that is right in a
  // screenshot and wrong by six pixels for a year: an empty slot that still takes a gap, or a
  // control outside the group that is supposed to balance the other side. Two pixels of slack for
  // sub-pixel rounding, and nothing for anything else.
  // What is centred is the field, *Capture* standing past its end (design-system.md §11.2); on
  // `medium` the field's look shrinks with the bar instead and is not measured here.
  if (inBar === 1 && (await page.locator('header form[role="search"]').count()) === 1) {
    const centred = await page.evaluate(() => {
      const ways = document.querySelector('header form[role="search"]').closest('.ways').getBoundingClientRect();
      const row = document.querySelector('header form[role="search"]').closest('header').firstElementChild.getBoundingClientRect();
      return Math.round(ways.left + ways.width / 2 - (row.left + row.width / 2));
    });
    assert.equal(Math.abs(centred) <= 2, true, `${width}: the bar's field and Capture are ${centred} px off centre`);
  }
  // On a phone the hubs are the bottom bar's *Browse* (the navigation concept §9).
  const hubsTarget = width < 600 ? 'nav a[href="/browse"]' : '[data-tour="hubs"]';
  assert.equal(await page.locator(hubsTarget).count(), 1, `${width}: the tour's hubs step has ${await page.locator(hubsTarget).count()} targets`);
  // The tour's capture and palette steps (ux-inventory.md §E1): one capture control to point at on
  // every width - the bar's, or the bottom bar's ＋ on a phone - and the palette's field where the
  // bar draws it; a phone passes that step over.
  const captureTargets = await page.locator('[data-tour="capture"]:visible, nav button[data-action]:visible').count();
  assert.equal(captureTargets, 1, `${width}: the tour's capture step has ${captureTargets} targets`);
  assert.equal(await page.locator('[data-tour="palette"]:visible').count(), width < 600 ? 0 : 1, `${width}: the tour's palette step`);
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, `${width}: the page scrolls sideways (${scroll.width} of ${scroll.viewport})`);
  assert.equal(await page.getByRole('link', { name: 'Skip to the content' }).count(), 1, `${width}: no skip link`);
  // No footer, on any width and on every screen behind the sign-in: the way to the statement is
  // "About Hubtask" while there is a session, and a landmark carrying one external link is not
  // worth the band it takes off a board (`design-system.md` §10). The walk below proves the
  // way is there; this proves the band is not.
  assert.equal(await page.getByRole('link', { name: STATEMENT }).count(), 0, `${width}: a footer while signed in`);
  assert.equal(await page.getByRole('status').filter({ hasText: /Connected|Reconnecting|Offline|synced|copy/ }).count() > 0, true, `${width}: no sync line`);
}

test('chromium: 375 px — the bottom bar with Browse, the levels as a stack, the account behind the avatar', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 375);
  t.after(close);
  await common(page, 375);

  // Spacious for a thumb, and the bar the thumb reaches (the navigation concept §9).
  assert.equal(await page.locator('.frame[data-density="spacious"]').count(), 1, 'the frame is not spacious below medium');
  const bar = page.getByRole('navigation', { name: 'Sections' });
  for (const name of ['Today', 'Browse', 'Inbox', 'Search']) assert.equal(await bar.getByRole('link', { name }).count(), 1, `${name} is not in the bottom bar`);
  // The ＋ is an act: a button, never a link (UC-WRK-26 check 1).
  assert.equal(await bar.getByRole('button', { name: 'Capture' }).count(), 1, 'the bottom bar has no capture');
  assert.equal(await bar.getByRole('link', { name: 'Capture' }).count(), 0, 'capture is drawn as a link');
  assert.equal(await bar.getByRole('link', { name: 'Today' }).getAttribute('aria-current'), 'page');
  assert.equal(await page.getByRole('button', { name: 'Open the navigation' }).count(), 0, 'a drawer beside the stack');

  // Browse is the workspace level as a step: the hub, the administration and the keeping, and
  // none of the destinations the bar below already has.
  await bar.getByRole('link', { name: 'Browse' }).click();
  const browse = page.locator('[data-browse-step]');
  await browse.getByRole('link', { name: new RegExp(HUB.name) }).waitFor({ timeout: 5_000 });
  const rows = (await browse.locator('a').allInnerTexts()).map((row) => row.split('\n')[0].trim());
  assert.deepEqual(rows, ['House', 'Administration', 'Archive', 'Trash'], `Browse holds ${JSON.stringify(rows)}`);
  // Pressing the hub's row steps into the hub's level; its collection opens the collection.
  await browse.getByRole('link', { name: new RegExp(HUB.name) }).click();
  await page.waitForFunction((id) => location.pathname === `/browse/hubs/${id}`, HUB.id, { timeout: 5_000 });
  await browse.getByRole('link', { name: 'Kitchen' }).click();
  await page.waitForFunction((id) => location.pathname === `/collections/${id}`, COLLECTION.id, { timeout: 5_000 });
  assert.equal(await bar.getByRole('link', { name: 'Browse' }).getAttribute('aria-current'), 'page', 'a collection is not Browse\'s');
  // ‹ the level above is the hub's step.
  await page.locator('[data-phone-bar] [data-back]').click();
  await page.waitForFunction((id) => location.pathname === `/browse/hubs/${id}`, HUB.id, { timeout: 5_000 });

  // The avatar on a root opens the account group as a sheet: the name at its head, the rows,
  // sign out last.
  await bar.getByRole('link', { name: 'Today' }).click();
  await page.locator('[data-phone-bar]').getByRole('button', { name: ACCOUNT.display_name }).click();
  const sheet = page.locator('dialog[open]');
  await sheet.waitFor({ timeout: 5_000 });
  assert.equal(await sheet.getByText(ACCOUNT.display_name).count(), 1, 'the sheet does not name the person');
  const items = await sheet.locator('a, button').allTextContents();
  const named = items.map((item) => item.trim()).filter((item) => ACCOUNT_GROUP.includes(item));
  assert.deepEqual(named, ACCOUNT_GROUP, `the sheet holds ${JSON.stringify(items)}`);
  await sheet.getByRole('link', { name: 'Your settings' }).or(sheet.getByRole('button', { name: 'Your settings' })).first().click();
  await sheet.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, '/profile');
  assert.equal(await bar.locator('[aria-current="page"]').count(), 0, 'a place of the bar is current in Your settings');

  // Your settings is a section of its own (ADR-0065 decision 3), its list behind ☰ - and its first
  // row is the way out, which is what a section somebody cannot leave would be missing.
  const toggle = page.getByRole('button', { name: 'Open the navigation' });
  await toggle.click();
  const settings = page.locator('dialog[open]');
  await settings.waitFor({ timeout: 5_000 });
  assert.equal(await settings.getByRole('link', { name: 'Trash' }).count() + await settings.getByRole('treeitem', { name: 'Trash' }).count(), 0, 'the sidebar is drawn inside the section');
  await settings.getByRole('treeitem', { name: 'Signed in', exact: true }).click();
  await settings.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(new URL(page.url()).pathname, '/profile/sessions');
  await toggle.click();
  await page.locator('dialog[open]').getByRole('treeitem', { name: 'The workspace', exact: true }).click();
  await page.waitForFunction(() => location.pathname === '/', null, { timeout: 5_000 });

  // The trash, through Browse's foot: Browse returns to the hub's step where it was left, a
  // second tap to its root.
  await bar.getByRole('link', { name: 'Browse' }).click();
  await page.waitForFunction((id) => location.pathname === `/browse/hubs/${id}`, HUB.id, { timeout: 5_000 });
  await bar.getByRole('link', { name: 'Browse' }).click();
  await page.waitForFunction(() => location.pathname === '/browse', null, { timeout: 5_000 });
  await page.locator('[data-browse-step]').getByRole('link', { name: 'Trash' }).click();
  await page.waitForFunction(() => location.pathname === '/trash', null, { timeout: 5_000 });

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
  await drawer.getByRole('treeitem', { name: /House/ }).waitFor({ timeout: 5_000 });
  const rows = (await drawer.locator('a, [role="treeitem"]').allInnerTexts()).map((row) => row.split('\n')[0].trim());
  assert.deepEqual(rows, [...FOR_YOU, 'House', 'Administration', 'Archive', 'Trash'], `the drawer holds ${JSON.stringify(rows)}`);
  // Search is the bar's way into the search over the page here, and it leads to the same
  // destination as the row.
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });
  await page.locator('header button.seek').click();
  await page.getByRole('dialog').locator('form[role="search"] input').fill('milk');
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => location.pathname === '/search', null, { timeout: 5_000 });
  // The address carries a handle for the words, never the words: what is kept is kept
  // in this tab, and a link made of this address would carry the narrowing and nothing typed.
  assert.equal(page.url().includes('milk'), false, `the words reached the address bar: ${page.url()}`);
  assert.equal(new URL(page.url()).searchParams.get('q'), null, 'the term is in the address as `q`');

  await page.getByRole('button', { name: ACCOUNT.display_name }).click();
  const menu = page.getByRole('menu', { name: 'You' });
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual((await menu.getByRole('menuitem').allTextContents()).map((item) => item.trim()), ACCOUNT_GROUP);
  await page.keyboard.press('Escape');
  assert.deepEqual(failures, []);
});

test('chromium: 600 px — the drawer closed and opened again at once stays open', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, 600);
  t.after(close);

  const toggle = page.getByRole('button', { name: 'Open the navigation' });
  // Held before it opens: behind a modal the opener is inert, and no role query reaches it.
  const opener = await toggle.elementHandle();
  await toggle.focus();
  await page.keyboard.press('Enter');
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  // Escape and the opener within one task, so the native close event of the first closing is
  // certainly still queued when the drawer opens again: it must not close the drawer it finds open.
  // Svelte applies a change in a microtask, which still runs within the task.
  const closed = await opener.evaluate(async (element) => {
    const open = document.querySelector('dialog[open]');
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    for (let i = 0; i < 50 && open?.open; i += 1) await Promise.resolve();
    const wasClosed = open !== null && !open.open;
    element.click();
    return wasClosed;
  });
  assert.ok(closed, 'Escape did not close the drawer within the task');
  await page.evaluate(() => new Promise((resolve) => setTimeout(resolve, 200)));
  assert.ok(await drawer.isVisible(), 'the reopened drawer closed itself');
});

for (const width of [905, 1280]) {
  test(`chromium: ${width} px — the navigation pinned, folding to a rail, the avatar alone in the bar`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { context, page, failures, close } = await open(browser, width);
    t.after(close);
    await common(page, width);

    assert.equal(await page.getByRole('navigation', { name: 'Sections' }).count(), 0, `${width}: a bottom bar on a desk`);
    assert.equal(await page.getByRole('button', { name: 'Open the navigation' }).count(), 0, `${width}: a drawer trigger on a desk`);
    // The sidebar's three regions (the navigation concept, §2.1): *For you* at the top, the
    // workspace level in the middle, the foot pinned below (ADR-0063 decision 1).
    const aside = page.locator('aside.sidenav');
    const top = aside.getByRole('list', { name: 'For you' });
    const tree = aside.getByRole('tree', { name: 'Hubs' });
    const keeping = aside.getByRole('list', { name: 'What is kept' });
    await tree.getByRole('treeitem', { name: /House/ }).waitFor({ timeout: 10_000 });
    const first = (texts) => texts.map((row) => row.split('\n')[0].trim());
    assert.deepEqual(first(await top.getByRole('link').allInnerTexts()), FOR_YOU, `${width}: For you`);
    assert.deepEqual(first(await tree.getByRole('treeitem').allInnerTexts()), ['House'], `${width}: the hubs`);
    assert.deepEqual(first(await keeping.getByRole('link').allInnerTexts()), ['Administration', 'Archive', 'Trash'], `${width}: the foot`);
    // And it is at the foot: below every row above it, and at the bottom of the column.
    const foot = await page.evaluate(() => {
      const column = document.querySelector('aside.sidenav');
      const band = column?.querySelector('ul.foot')?.getBoundingClientRect();
      const last = [...(column?.querySelectorAll('[role="treeitem"]') ?? [])].at(-1)?.getBoundingClientRect();
      return band && last ? { below: Math.round(band.top - last.bottom), toBottom: Math.round(column.getBoundingClientRect().bottom - band.bottom) } : null;
    });
    assert.ok(foot !== null && foot.below > 100, `${width}: the foot is ${JSON.stringify(foot)} from the hubs`);
    assert.ok(foot.toBottom < 40, `${width}: the foot is ${foot.toBottom}px above the column's bottom`);
    assert.equal(await page.locator('aside[data-tour="hubs"]').count(), 1, `${width}: the tour's target is not the pinned navigation`);
    assert.equal(await top.getByRole('link', { name: 'Today' }).getAttribute('aria-current'), 'page');

    // The rail: the same places, folded to their marks, and back. The width is the token's.
    const pinned = await aside.evaluate((el) => el.getBoundingClientRect().width);
    await page.getByRole('button', { name: 'Collapse the navigation' }).click();
    const rail = await aside.evaluate((el) => el.getBoundingClientRect().width);
    assert.ok(rail < pinned / 2, `${width}: the rail is ${rail} px against ${pinned} px pinned`);
    assert.equal(await top.getByRole('link').count(), FOR_YOU.length, `${width}: the rail lost For you`);
    assert.equal(await keeping.getByRole('link').count(), 3, `${width}: the rail lost the foot`);
    await aside.getByRole('link', { name: HUB.name, exact: true }).waitFor({ timeout: 5_000 });
    // And every mark is **drawn**, inside the column: a mark clipped by the folded column makes
    // the rail a column of slivers.
    const marks = await aside.evaluate((el) => {
      const column = el.getBoundingClientRect();
      return [...el.querySelectorAll('li.row')].map((row) => {
        const box = row.querySelector('.mark svg')?.getBoundingClientRect();
        return box ? { inside: box.left >= column.left && box.right <= column.right, width: Math.round(box.width) } : { inside: false, width: 0 };
      });
    });
    assert.ok(marks.length >= FOR_YOU.length + 1 + 3, `${width}: the rail drew ${marks.length} rows`);
    assert.deepEqual(marks.filter((mark) => !mark.inside || mark.width === 0), [], `${width}: a mark of the rail is clipped or missing`);
    // The label is announced although it is not drawn, so the rail is navigable by name.
    assert.equal(await top.getByRole('link', { name: 'Inbox' }).count(), 1, `${width}: the rail's rows lost their names`);

    // A hub pressed in the rail goes to the hub, and the rail then holds that hub's level - its
    // mark and its tools - so nothing is unreachable while the navigation is folded.
    await aside.getByRole('link', { name: HUB.name, exact: true }).click();
    await page.waitForFunction((id) => location.pathname === `/hubs/${id}`, HUB.id, { timeout: 5_000 })
      .catch(() => assert.fail(`${width}: a hub pressed in the rail did not open the hub`));
    // Away from the rail, so its peek (a hover of 300 ms) does not unfold it.
    await page.mouse.move(width - 40, 400);
    await page.waitForFunction(() => !document.querySelector('aside.sidenav')?.hasAttribute('data-peek'), undefined, { timeout: 5_000 });
    assert.equal(await aside.getByRole('link', { name: HUB.name, exact: true }).getAttribute('aria-current'), 'page', `${width}: the rail does not hold the hub's level`);
    await page.getByRole('button', { name: 'Expand the navigation' }).click();
    assert.equal(await aside.evaluate((el) => el.getBoundingClientRect().width), pinned);
    await aside.getByRole('region', { name: HUB.name }).getByRole('treeitem', { name: COLLECTION.name }).waitFor({ timeout: 5_000 })
      .catch(() => assert.fail(`${width}: the hub's level does not hold its collections`));
    await aside.getByRole('button', { name: 'All hubs' }).click();
    await tree.waitFor({ timeout: 5_000 });

    // What the application says about itself is a mark too (ADR-0065 decision 4), and not a
    // banner above the head of every page: the stage is behind it, and the page starts at its own
    // heading.
    const notice = page.getByRole('banner', { name: 'Application bar' }).getByRole('button', { name: 'Hubtask is a preview' });
    assert.equal(await notice.count(), 1, `${width}: the stage is not in the bar`);
    assert.equal(await page.locator('main').getByText('Hubtask is a preview').count(), 0, `${width}: a banner still says it on the page`);
    await notice.click();
    const said = page.getByRole('dialog', { name: 'Hubtask is a preview' });
    await said.waitFor({ timeout: 5_000 });
    assert.equal(await said.getByText(/What you see is meant to stay/).count(), 1, `${width}: the mark opened nothing`);
    await page.keyboard.press('Escape');
    await said.waitFor({ state: 'hidden', timeout: 5_000 }).catch(() => {});

    // The connection is one mark in the bar, and no line of the page (ADR-0063 decision 5). At its
    // quietest it says nothing until it is pressed; what a line would print is behind it.
    const mark = page.getByRole('banner', { name: 'Application bar' }).locator('.trigger');
    assert.equal(await mark.count(), 1, `${width}: the connection is not in the bar`);
    assert.equal(await mark.getAttribute('aria-label'), 'Connected');
    assert.ok(await mark.evaluate((el) => el.hasAttribute('data-quiet')), `${width}: the quiet case is not quiet`);
    assert.equal(await page.locator('main').getByText(/Connected|Reconnecting/).count(), 0, `${width}: a line of the page still says it`);
    await mark.click();
    const surface = page.getByRole('dialog', { name: 'The copy and the server' });
    await surface.waitFor({ timeout: 5_000 });
    assert.equal(await surface.getByText('All synchronised').count(), 1, `${width}: what it opened does not say the state`);
    await page.keyboard.press('Escape');
    await surface.waitFor({ state: 'hidden', timeout: 5_000 }).catch(() => {});

    // The three controls at the end of the bar are drawn as one kind of thing: the
    // two glyph controls are `IconButton`'s square with its radius, and the avatar is the same
    // square drawn round, because what is inside it is round. Three shapes for one row of
    // controls is the defect this guards.
    const shapes = await page.evaluate(() => {
      const read = (selector) => {
        const el = document.querySelector(`header ${selector}`);
        if (!el) return null;
        const box = el.getBoundingClientRect();
        return { w: Math.round(box.width), h: Math.round(box.height), radius: getComputedStyle(el).borderRadius };
      };
      return { notice: read('.notice-mark button'), sync: read('.trigger'), avatar: read('.account'), toggle: read('[data-toggle]') };
    });
    assert.deepEqual(shapes.notice, shapes.sync, `${width}: the two glyph controls are drawn differently`);
    // One size and one hover shape for every control of the bar (§13, G2): the rail toggle and the
    // avatar are the marks' square, at every width.
    assert.deepEqual(shapes.avatar, shapes.sync, `${width}: the avatar is drawn as ${JSON.stringify(shapes.avatar)}`);
    assert.deepEqual(shapes.toggle, shapes.sync, `${width}: the rail toggle is drawn as ${JSON.stringify(shapes.toggle)}`);

    // A device with no network says so, rather than saying it is trying: `navigator.onLine` is
    // false and the mark is the struck cloud ADR-0063 decision 5 names. Trusted in one direction
    // only - coming back says "reconnecting" until the stream is accepted again, never
    // "connected" on the browser's word.
    await context.setOffline(true);
    await page
      .waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') === 'offline', null, { timeout: 5_000 })
      .catch(() => assert.fail(`${width}: a device with no network does not say it is offline`));
    assert.equal(await mark.getAttribute('aria-label'), 'Offline');
    await context.setOffline(false);
    await page
      .waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') !== 'offline', null, { timeout: 5_000 })
      .catch(() => assert.fail(`${width}: the mark stayed offline after the network came back`));

    // Every destination, through the sidebar and the menu.
    await top.getByRole('link', { name: 'Inbox' }).click();
    assert.equal(new URL(page.url()).pathname, '/jumble');
    await keeping.getByRole('link', { name: 'Trash' }).click();
    assert.equal(new URL(page.url()).pathname, '/trash');
    // The trigger is the avatar alone at every width, named by the person; the name and the
    // address are inside rather than in the frame (design-system.md §11.2).
    const trigger = page.getByRole('button', { name: ACCOUNT.display_name });
    assert.equal((await trigger.textContent()).includes(ACCOUNT.display_name), false, `${width}: the trigger draws "${(await trigger.textContent()).trim()}"`);
    assert.equal(await page.getByRole('banner', { name: 'Application bar' }).getByText(ACCOUNT.email).count(), 0, `${width}: the bar carries an address`);
    await trigger.click();
    const menu = page.getByRole('menu', { name: 'You' });
    await menu.waitFor({ timeout: 5_000 });
    assert.deepEqual((await menu.getByRole('menuitem').allTextContents()).map((item) => item.trim()), ACCOUNT_GROUP);
    // Whose menu it is, said where it is opened and nowhere else.
    const whose = page.locator('.surface').filter({ has: page.getByRole('menu', { name: 'You' }) });
    assert.equal(await whose.getByText(ACCOUNT.email).count(), 1, `${width}: the menu does not say whose it is`);
    // And the name is read in the colour the rows are: the head inherits the surface's quieter
    // one, which reads on white and disappears on the dark theme's surface. Compared
    // against a row rather than against a token, so it holds in both themes.
    const nameColour = await whose.getByText(ACCOUNT.display_name).first().evaluate((el) => getComputedStyle(el).color);
    const rowColour = await menu.getByRole('menuitem').first().evaluate((el) => getComputedStyle(el).color);
    assert.equal(nameColour, rowColour, `${width}: the person's name is drawn in ${nameColour}, the rows in ${rowColour}`);
    await menu.getByRole('menuitem', { name: /About Hubtask/ }).click();
    assert.equal(new URL(page.url()).pathname, '/installation');
    assert.deepEqual(failures, []);
  });
}

/** A second hub, with nothing in it: the case the twist must not promise anything about. */
const EMPTY_HUB = { id: '01a0e2e0-0000-7000-8000-00000000000e', type: 'HUB', parent_id: null, name: 'Nothing here', order_key: 'a1', version: 1 };

test('chromium: 1280 px — a hub with nothing in it offers nothing to open, and is still a place', async (t) => {
  // A hub's row carries no twist (the navigation concept, §2.3): it opens the hub's level, which
  // is where a collection is made - so an empty hub promises nothing it cannot keep.
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/api/v1/containers')) {
      if (url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB, EMPTY_HUB] } });
      // The level of the empty hub answers an empty page; the other hub keeps its collection.
      const parent = url.searchParams.get('parent_id');
      return route.fulfill({ json: { ...PAGE, data: parent === EMPTY_HUB.id ? [] : [COLLECTION] } });
    }
    if (url.pathname.endsWith(`/api/v1/containers/${EMPTY_HUB.id}`)) return route.fulfill({ json: EMPTY_HUB });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  // On a page that reads no level itself: *Today* reads every hub's collections for their
  // figures, which would settle the empty hub before the tree is asked.
  await page.goto(`${served.origin}/trash`);

  const tree = page.locator('aside.sidenav').getByRole('tree', { name: 'Hubs' });
  const row = tree.getByRole('treeitem', { name: new RegExp(EMPTY_HUB.name) });
  await row.waitFor({ timeout: 15_000 });
  assert.equal(await tree.locator('[aria-expanded]').count(), 0, 'a hub row offers to be opened where it stands');

  await row.click();
  await page.waitForFunction((id) => location.pathname === `/hubs/${id}`, EMPTY_HUB.id, { timeout: 5_000 })
    .catch(() => assert.fail('an empty hub is not a place'));
  // Its level is drawn, with no collection it does not hold.
  const level = page.locator('aside.sidenav').getByRole('region', { name: EMPTY_HUB.name });
  await level.getByRole('heading', { name: EMPTY_HUB.name }).waitFor({ timeout: 5_000 });
  assert.equal(await level.getByRole('treeitem').count(), 0, 'an empty hub draws a collection');
  assert.deepEqual(failures, []);
});

// The way out to the accessibility statement, which the EAA asks to be findable and
// `design-system.md` §10 asks the application to offer. It is one link and it is in two places,
// each of which is the only place its reader has: "About Hubtask" while there is a session, and
// the foot of the sign-in screen while there is not - a barrier at the door is the one that
// nothing behind the door makes up for.
test('chromium: the accessibility statement is reachable signed in, and signed out only as the operator sets it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const { page, failures, close } = await open(browser, 1280);
  t.after(close);

  // Signed in: on the page the account group already offers for the versions, with the two other
  // pages about the software, and each says where it goes before it is followed.
  await page.goto(`${served.origin}/installation`);
  const about = page.getByRole('heading', { name: 'About the software' });
  await about.waitFor({ timeout: 10_000 });
  const links = page.locator('main section a');
  assert.deepEqual((await links.allTextContents()).map((each) => each.trim()), [
    STATEMENT,
    'Licence, on hubtask.eu',
    'Source code, on github.com',
  ]);
  const statement = page.getByRole('link', { name: STATEMENT });
  assert.equal(await statement.getAttribute('href'), 'https://hubtask.eu/accessibility/');
  assert.equal(await statement.getAttribute('target'), '_blank', 'it replaces what the reader was in the middle of');
  assert.equal(await statement.getAttribute('rel'), 'noopener');

  // And it is there when the manifest is not: the reader having the worst time of it is the one
  // a client must not hide the statement from.
  await page.route('**/api/v1/meta/capabilities', (route) => route.abort());
  await page.reload();
  await page.getByRole('heading', { name: 'About the software' }).waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('link', { name: STATEMENT }).count(), 1, 'the statement went with the manifest');

  const signedOut = await open(browser, 1280, { signedIn: false });
  t.after(signedOut.close);
  // Signed out, the foot carries the operator's links and nothing of the project's (UC-ID-18 check
  // 5): an installation that set no accessibility statement shows none, rather than the
  // project's standing in for a service it does not run.
  assert.equal(await signedOut.page.locator('footer a[href*="hubtask.eu"]').count(), 0, 'the sign-in screen points at hubtask.eu');

  assert.deepEqual(failures, []);
  assert.deepEqual(signedOut.failures, []);
});

// The band under the sign-in screen on a phone. The frame reserves the bottom bar's height below
// `medium` so that the last row of a list is reachable rather than under the bar - but the bar is
// drawn for a session, and the reservation was not conditioned on one. Signed out the screen
// therefore ended 56 px above the bottom with nothing in the gap, and scrolled into it.
test('chromium: 375 px — signed out, nothing is reserved for a bar that is not drawn', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 375, { signedIn: false });
  t.after(close);

  assert.equal(await page.getByRole('navigation', { name: 'Sections' }).count(), 0, 'a bottom bar without a session');
  const box = await page.evaluate(() => ({ document: document.documentElement.scrollHeight, window: window.innerHeight }));
  assert.equal(box.document, box.window, `the sign-in screen scrolls ${box.document - box.window} px into nothing`);

  assert.deepEqual(failures, []);
});
