// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The compact bar's contract (F10-16; ADR-0061 decision 1's table, issue 904): at 375 px the bar
// holds ☰ · the page's title · the page's menu.
//
// Two halves, and the walk proves both. **The title**: every route a signed-in reader can reach
// hands its title to the bar, so nothing shows the wordmark above its own `h1` — the count of
// pages that entitle nothing is the assertion, and it is zero. **The menu**: a page whose actions
// fold into one control has that control in the bar rather than on a line of its own under the
// title, and the list in it is the head's own folded list rather than a second one.
//
// Chromium only: this is a layout contract with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { fallback, unstubbedSoFar } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e3-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Bar Walker', email: 'bar@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e3-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'House', order_key: 'a0', version: 1 };
const COLLECTION = { id: '01a0e2e3-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Kitchen', order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

async function stub(route) {
  const url = new URL(route.request().url());
  const path = url.pathname.replace(/^.*\/api\/v1/, '');
  // The stream, **accepted and empty**: the connection is what the mark in the bar reads since
  // issue 1017, so a walk that refused it would draw *Reconnecting…* on every screenshot of every
  // screen. It carries the server's own reconnect suggestion and no records; what a walk needs
  // from the stream is that it was opened.
  if (path === '/stream') {
    return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
  }
  if (path === '/sync:snapshot') {
    const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
  }
  if (path === '/sync:pull') return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  if (path === '/accounts/me') return route.fulfill({ json: ACCOUNT });
  if (path === '/meta/capabilities') {
    return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], supported_locales: [{ locale: 'en', direction: 'ltr' }] } });
  }
  // The workspace, in the shape the screen fills its fields from: a page envelope here would put
  // `undefined` in a field typed as a string, and the screen would throw on the first render.
  if (path === '/tenant') {
    return route.fulfill({ json: { id: '01a0e2e3-0000-7000-8000-000000000009', display_name: 'Walk', default_locale: 'en', default_time_zone: 'Europe/Berlin', require_admin_totp: false, version: 1 } });
  }
  if (path === '/containers' && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
  if (path === '/containers') return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (path === `/containers/${COLLECTION.id}`) return route.fulfill({ json: COLLECTION });
  if (path === `/containers/${HUB.id}`) return route.fulfill({ json: HUB });
  if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
  // The frame's own reads, in the shapes the API answers them, and a record of anything this
  // walk never prepared: a guess nobody notices is what `fallback` exists to prevent.
  return fallback(route, route.request(), path);
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, width = 375) {
  const context = await browser.newContext({ viewport: { width, height: 812 } });
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  // Named with the address that was open when it happened: a walk of thirty routes that reports
  // only "TypeError" tells you nothing about which of the thirty to look at.
  page.on('pageerror', (error) => failures.push(`${new URL(page.url()).pathname}: ${error}`));
  return { page, failures, close: () => context.close() };
}

/**
 * Every route a signed-in reader reaches, as `lib/routes.ts` patterns them.
 *
 * The signed-out three are not here — `/redeem`, `/auth/callback` and `/oauth/consent` are
 * reached without a session, where the bar deliberately carries the wordmark and nothing else.
 */
const ROUTES = [
  '/', '/search', '/jumble', '/archive', '/trash', '/installation',
  // Your settings, which is a section of eight screens since ADR-0065 decision 3.
  '/profile', '/profile/appearance', '/profile/notifications', '/profile/security',
  '/profile/sessions', '/profile/devices', '/profile/apps', '/profile/tokens',
  '/administration', '/administration/workspace', '/administration/people',
  '/administration/groups', '/administration/permissions', '/administration/service-accounts',
  // `/administration/rules/:id` is not here: the editor is a canvas with a stub of its own in
  // `rules.test.mjs`, and what it would add to a walk of the bar is a second copy of that stub.
  '/administration/apps', '/administration/rules', '/administration/runs', '/administration/webhooks', '/administration/quotas',
  '/administration/backup', '/administration/retention', '/administration/restore',
  '/administration/audit', '/administration/privacy', '/administration/identity-provider',
  '/administration/ai',
];

test('chromium: 375 px — every page hands its title to the bar', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/`);
  await page.getByRole('navigation', { name: 'Sections' }).waitFor({ timeout: 15_000 });

  const silent = [];
  for (const route of ROUTES) {
    await page.goto(`${served.origin}${route}`);
    // The title arrives with the screen rather than with the navigation, so it is waited for
    // rather than read at once - a page that never sets one is what the timeout finds.
    const title = await page
      .locator('[data-bar="title"]')
      .textContent({ timeout: 4_000 })
      .catch(() => undefined);
    if (!title || title.trim() === '') silent.push(route);
  }

  assert.deepEqual(silent, [], `these pages show the wordmark above their own heading: ${silent.join(', ')}`);
  assert.deepEqual(failures, []);
});

test('chromium: the administration is a section, and the tree is not in it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);

  // Outside the section: the workspace's tree, with the hub in it.
  await page.goto(`${served.origin}/`);
  await page.getByRole('treeitem', { name: HUB.name }).waitFor({ timeout: 15_000 });

  await page.goto(`${served.origin}/administration/people`);
  // Exact, because the trail on the screen is a landmark too and its name contains the word.
  const section = page.getByRole('navigation', { name: 'Administration', exact: true });
  await section.waitFor({ timeout: 10_000 });

  // The section's own list replaces the tree rather than joining it (ADR-0063 decision 7).
  assert.equal(await page.getByRole('navigation', { name: 'Workspace' }).count(), 0, 'the workspace tree is drawn inside the section');
  assert.equal(await page.getByRole('treeitem', { name: HUB.name }).count(), 0, 'a hub is drawn inside the section');

  // The way out is the first row, and the row the reader is on is announced as current.
  const rows = (await section.getByRole('treeitem').allTextContents()).map((row) => row.trim());
  assert.equal(rows[0], 'The workspace', `the first row is ${JSON.stringify(rows[0])}`);
  assert.equal(rows.length, 18, `the section holds ${rows.length} rows`);
  // Exact again: "People" and "People's requests" are both rows of this list.
  assert.equal(await section.getByRole('treeitem', { name: 'People', exact: true }).getAttribute('aria-current'), 'page');

  // And it leads out: back to the overview, where the tree is again.
  await section.getByRole('treeitem', { name: 'The workspace', exact: true }).click();
  await page.waitForFunction(() => location.pathname === '/', null, { timeout: 5_000 });
  await page.getByRole('treeitem', { name: HUB.name }).waitFor({ timeout: 10_000 });

  assert.deepEqual(failures, []);
});

/** The section's screens, and the word each puts at the end of its trail. */
const SECTION = [
  ['/administration/workspace', 'Workspace'],
  ['/administration/people', 'People'],
  ['/administration/groups', 'Groups'],
  ['/administration/permissions', 'What each role means'],
  ['/administration/service-accounts', 'Service accounts'],
  ['/administration/apps', 'Third-party apps'],
  ['/administration/webhooks', 'Webhooks'],
  ['/administration/quotas', 'Limits'],
  ['/administration/backup', 'Backup'],
  ['/administration/retention', 'Retention and holds'],
  ['/administration/restore', 'Restore'],
  ['/administration/audit', 'The trail'],
  ['/administration/privacy', "People's requests"],
  ['/administration/identity-provider', 'Sign-in provider'],
  ['/administration/ai', 'AI'],
];

test('chromium: every screen of the section says where it is and leads back', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser, 1280);
  t.after(close);

  await page.goto(`${served.origin}/administration`);
  await page.getByRole('navigation', { name: 'Administration', exact: true }).waitFor({ timeout: 15_000 });

  const untrailed = [];
  const unheaded = [];
  for (const [route, word] of SECTION) {
    await page.goto(`${served.origin}${route}`);
    const trail = page.getByRole('navigation', { name: 'Where you are in the administration' });
    // By role, not by selector, and that is the point: the trail is in the DOM on a screen whose
    // head has folded, but it is not in the accessibility tree - which is what "the screen says
    // where it is" means. A `locator('nav')` would have found it and proved nothing.
    const shown = await trail.textContent({ timeout: 4_000 }).catch(() => undefined);
    // The trail names the section and the screen, and its first crumb is the way back.
    if (!shown || !shown.includes('Administration') || !shown.includes(word)) untrailed.push(route);
    // One `h1` per screen, whether drawn or read: two headings is two answers to what a page is.
    if (await page.locator('main h1').count() !== 1) unheaded.push(route);
  }

  assert.deepEqual(untrailed, [], `these screens do not say where they are: ${untrailed.join(', ')}`);
  assert.deepEqual(unheaded, [], `these screens do not have exactly one heading: ${unheaded.join(', ')}`);

  // And the trail's first crumb is a link out, not decoration. It leads to the section's front
  // door, which is the section's **first screen** (ADR-0065 decision 1): the column lists every
  // screen, so an index beside it would be that list drawn twice. The address is replaced rather
  // than added to, so the reader's back button still goes where they came from.
  await page.goto(`${served.origin}/administration/quotas`);
  await page
    .getByRole('navigation', { name: 'Where you are in the administration' })
    .getByRole('link', { name: 'Administration' })
    .click();
  await page.waitForFunction(() => location.pathname === '/administration/workspace', null, { timeout: 5_000 });
  // The row the reader lands on is the one the column marks, so arriving says where they are.
  assert.equal(
    await page
      .getByRole('navigation', { name: 'Administration', exact: true })
      .getByRole('treeitem', { name: 'Workspace', exact: true })
      .getAttribute('aria-current'),
    'page',
  );

  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the section is behind the drawer, and the bottom bar still leaves it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/quotas`);
  await page.getByRole('button', { name: 'Open the navigation' }).click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 10_000 });
  assert.equal(await drawer.getByRole('treeitem', { name: 'Limits' }).count(), 1, 'the section is not in the drawer');
  assert.equal(await drawer.getByRole('treeitem', { name: HUB.name }).count(), 0, 'the tree is in the drawer inside the section');
  await page.keyboard.press('Escape');

  // The bottom bar is the frame's and stays whatever section the reader is in - it is how they
  // leave one on a phone.
  const bar = page.getByRole('navigation', { name: 'Sections' });
  assert.equal(await bar.getByRole('link', { name: 'Overview' }).count(), 1, 'no way out of the section on a phone');

  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the page menu is in the bar, and it is the head\'s own list', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  const menu = page.locator('[data-bar="menu"] button');
  await menu.waitFor({ timeout: 15_000 });

  // One control, and it is the bar's: the head hands its list over rather than drawing a second
  // one, so nothing under the title answers to the same name.
  // The accessible name rather than an attribute: `IconButton` names itself with visually hidden
  // text, which is what a screen reader and `getByRole` both read.
  const label = (await menu.innerText()).trim() || (await menu.getAttribute('aria-label')) || '';
  assert.notEqual(label, '', 'the bar\'s menu control has no name');
  assert.equal(
    await page.locator('main').getByRole('button', { name: label, exact: true }).count(),
    0,
    `the head still draws a control called ${label}`,
  );

  await menu.click();
  const opened = page.getByRole('menu');
  await opened.waitFor({ timeout: 5_000 });
  const items = (await opened.getByRole('menuitem').allTextContents()).map((item) => item.trim());
  assert.ok(items.length > 0, 'the bar\'s menu is empty');

  assert.deepEqual(failures, []);
});
