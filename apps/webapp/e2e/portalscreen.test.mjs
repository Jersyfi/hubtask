// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The requester's portal (user-experience.md §7.4, UC-SVC-05): the kinds of request as tiles with
// a sentence each, no form before one is chosen, the form below the chosen tile with the articles
// beside it as the requester types, and the raise sending the chosen kind. An empty catalogue says
// so. Walked at desk and phone width, light and dark, without sideways scroll. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, PAGE, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const DESK = '01a0e2e0-0000-7000-8000-0000000041d1';
const kind = (key, name, description, icon) => ({ key, name, description, icon, form: { fields: [] }, questions: [] });
const KINDS = [
  kind('incident', 'Something is broken', 'A device, a program or the network does not work as it should.', 'circle-alert'),
  kind('access', 'Access to a system for a new colleague who starts next month in another office', 'Accounts, groups and rights.', 'key'),
  kind('purchase', 'Hardware', '', 'not-an-icon'),
];
const ARTICLE = { page_id: '01a0e2e0-0000-7000-8000-0000000041a1', title: 'When the VPN drops', icon: '', excerpt: 'Restart the «VPN» client', content_language: null, updated_at: '2026-11-01T09:00:00Z' };

// `team` is the desk's team, which the catalogue answers with `may_raise` false; `refusedTeam` is
// a server that still refuses it the catalogue. Either refuses it its own requests, as the server
// does, and the collection has no public form.
async function portal(browser, { width, scheme, kinds, team = false, refusedTeam = false, permissions = ['READ', 'MANAGE_MEMBERS'], logo = null }) {
  const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme });
  forgetUnstubbed();
  const raised = [];
  const asked = [];
  team ||= refusedTeam;
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    asked.push(`${route.request().method()} ${path}`);
    if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, has_access: false, portals: [{ collection_id: DESK, name: 'IT help' }] } });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: team ? permissions : [] } });
    if (path === '/memberships') return route.fulfill({ json: { ...PAGE, data: [], grantable_roles: [] } });
    if (path === `/containers/${DESK}/desk-logo`) return route.fulfill({ json: { collection_id: DESK, logo_media_id: logo } });
    if (path === `/containers/${DESK}`) return route.fulfill({ json: { id: DESK, type: 'COLLECTION', name: 'IT help', description: 'Ask the IT team.', parent_id: null, icon: null, color_token: null, order_key: 'a0', version: 1, private: false, handle: 'ithelp', key_prefix: 'IH', created_at: '2026-11-01T09:00:00Z', updated_at: '2026-11-01T09:00:00Z' } });
    const refused = { status: 403, json: { type: 'about:blank', title: 'Forbidden', status: 403, code: 'forbidden' } };
    if (path === '/request-catalogue' && refusedTeam) return route.fulfill(refused);
    if (path === '/request-catalogue') {
      const manages = team && permissions.includes('MANAGE_MEMBERS') ? { has_public_form: false } : {};
      return route.fulfill({ json: { desk: { name: 'IT help', description: 'Ask the IT team.', icon: null, color_token: null, logo_media_id: logo }, data: kinds, may_raise: !team, ...manages } });
    }
    if (path === '/requests' && team) return route.fulfill(refused);
    if (path === `/collections/${DESK}/public-form` && route.request().method() === 'GET') return route.fulfill({ status: 404, json: { status: 404, code: 'errors.not_found', detail_code: 'public_forms.link_not_found' } });
    if (path === '/requests' && route.request().method() === 'POST') {
      const body = route.request().postDataJSON();
      raised.push(body);
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000041f1', number: 7, collection_id: DESK, entry_type: body.entry_type, title: body.title, notes: null, status: null, is_completed: false, completed_at: null, created_at: '2026-11-04T09:00:00Z', updated_at: '2026-11-04T09:00:00Z', is_own: true } });
    }
    if (path === '/requests') return route.fulfill({ json: { data: [], page_info: { next_cursor: null }, organisation: null } });
    if (path === '/approvals') return route.fulfill({ json: { data: [] } });
    if (path === '/articles') return route.fulfill({ json: { data: url.searchParams.has('text') ? [ARTICLE] : [] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/portal/${DESK}`);
  return { page, failures, raised, asked, close: () => context.close() };
}

async function noSidewaysScroll(page, label) {
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, `${label}: the page scrolls sideways (${scroll.width} of ${scroll.viewport})`);
}

for (const [width, scheme] of [[1280, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: the portal offers its kinds as tiles and the form below the chosen one`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, raised, close } = await portal(browser, { width, scheme, kinds: KINDS });
    t.after(close);
    const tiles = page.getByRole('list', { name: 'Kind of request' });
    await tiles.waitFor({ timeout: 15_000 });
    assert.equal(await tiles.getByRole('listitem').count(), 3);
    assert.match(await tiles.getByRole('listitem').first().innerText(), /A device, a program or the network/);
    // Nothing to fill before a kind is chosen.
    assert.equal(await page.getByLabel('In a few words').count(), 0);
    await noSidewaysScroll(page, `portal tiles ${width} ${scheme}`);

    await page.getByRole('button', { name: 'Ask for this Something is broken' }).click();
    assert.equal(await page.getByRole('button', { name: 'Chosen Something is broken' }).getAttribute('aria-pressed'), 'true');
    const title = page.getByLabel('In a few words');
    await title.waitFor({ timeout: 5_000 });
    assert.equal(await title.evaluate((node) => node === document.activeElement), true, 'the chosen tile moves the requester to the form');
    await title.fill('VPN drops at noon');
    await page.getByText('When the VPN drops').waitFor({ timeout: 5_000 });
    await noSidewaysScroll(page, `portal form ${width} ${scheme}`);
    await page.getByRole('button', { name: 'Send request' }).click();
    await page.waitForURL('**/portal/requests/*', { timeout: 5_000 });
    assert.deepEqual(raised.map((body) => [body.entry_type, body.title]), [['incident', 'VPN drops at noon']]);
    assert.deepEqual(failures, []);
  });
}

test('chromium 375 px, light: a portal that offers nothing says so', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await portal(browser, { width: 375, scheme: 'light', kinds: [] });
  t.after(close);
  await page.getByText('Nothing can be asked here yet').waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('list', { name: 'Kind of request' }).count(), 0);
  await noSidewaysScroll(page, 'portal empty');
  assert.deepEqual(failures, []);
});

// UC-SVC-05: the desk's team opening its portal is led to invite its first requester.
test('chromium: the desk\'s team sees its portal as the team, with the way to invite requesters', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, asked, close } = await portal(browser, { width: 1280, scheme: 'light', kinds: [], team: true });
  t.after(close);
  await page.getByText('You see this portal as the desk\'s team').waitFor({ timeout: 15_000 });
  await page.waitForLoadState('networkidle');
  // Nothing a requester alone may read is asked: the console holds no refusal or 404.
  assert.deepEqual(asked.filter((each) => each === 'GET /requests' || each === `GET /collections/${DESK}/public-form`), []);
  await page.getByRole('button', { name: 'Invite requesters' }).click();
  await page.waitForURL('**/administration/people', { timeout: 10_000 });
  assert.deepEqual(failures, []);
});

// UC-SVC-05 check 9: where a server refuses the team the requester catalogue, its head and the logo
// panel are drawn from the collection and the logo read the team holds - not left empty.
test('chromium: the team that shapes the desk sees its name and sets its logo at the portal', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await portal(browser, { width: 1280, scheme: 'light', kinds: [], refusedTeam: true, permissions: ['READ', 'STRUCTURE'], logo: null });
  t.after(close);
  await page.getByText('You see this portal as the desk\'s team').waitFor({ timeout: 15_000 });
  await page.getByRole('heading', { name: 'IT help', level: 1 }).waitFor({ timeout: 10_000 });
  await page.getByText('Logo of this service').first().waitFor({ timeout: 10_000 });
  assert.equal(await page.getByText('Choose a logo').count() > 0, true, 'the logo panel offers a choice');
  assert.equal(await page.getByText(/is below/).count(), 0, 'nothing promises a panel below');
  assert.deepEqual(failures, []);
});
