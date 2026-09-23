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

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e3-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Bar Walker', email: 'bar@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e3-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'House', order_key: 'a0', version: 1 };
const COLLECTION = { id: '01a0e2e3-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Kitchen', order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };

/**
 * The reads that answer a bare array rather than a page envelope.
 *
 * Named, because the difference matters to a screen: a page envelope where a list belongs puts an
 * object through `.map` and the screen throws while rendering. The walk visits every route, so it
 * meets every one of these - which the route-specific suites do not.
 */
const ARRAYS = new Set([
  '/quotas', '/groups', '/retention-policies', '/oauth/clients', '/oauth/grants',
  '/auth/service-accounts', '/auth/tokens', '/auth/sessions', '/sync/devices',
  '/backup-targets', '/backup-schedules', '/backups', '/integrations/webhooks',
  '/integrations/calendar-feeds',
]);

async function stub(route) {
  const url = new URL(route.request().url());
  const path = url.pathname.replace(/^.*\/api\/v1/, '');
  if (path === '/stream') return route.abort();
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
  if (ARRAYS.has(path)) return route.fulfill({ json: [] });
  if (path === '/containers' && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
  if (path === '/containers') return route.fulfill({ json: { ...PAGE, data: [COLLECTION] } });
  if (path === `/containers/${COLLECTION.id}`) return route.fulfill({ json: COLLECTION });
  if (path === `/containers/${HUB.id}`) return route.fulfill({ json: HUB });
  if (/\/(labels|buckets|views|templates|custom-fields|policies|members)$/.test(path)) return route.fulfill({ json: [] });
  return route.fulfill({ json: PAGE });
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser) {
  const context = await browser.newContext({ viewport: { width: 375, height: 812 } });
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
  '/', '/search', '/jumble', '/archive', '/trash', '/profile', '/profile/tokens', '/installation',
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
