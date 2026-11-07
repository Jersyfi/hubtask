// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A requester who holds nothing but portals (UC-SVC-05): with one portal they start in it, with
// several the start and the navigation list them - and neither says they have no access.
// Chromium only.

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

const DESK = { collection_id: '01a0e2e0-0000-7000-8000-0000000031d1', name: 'Störungen & Anfragen' };
const FACILITIES = { collection_id: '01a0e2e0-0000-7000-8000-0000000031d2', name: 'Facilities' };

async function requester(browser, portals) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  forgetUnstubbed();
  await context.route('**/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, has_access: false, portals } });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: [] } });
    // As the server answers somebody holding nothing at the workspace: the start must not wait on it.
    if (path === '/tenant') return route.fulfill({ status: 403, json: { type: 'about:blank', title: 'Forbidden', status: 403, code: 'forbidden' } });
    if (path === '/memberships') return route.fulfill({ json: { ...PAGE, data: [], grantable_roles: [] } });
    if (path === '/request-catalogue') return route.fulfill({ json: { desk: { name: DESK.name, description: '', icon: null, color_token: null, logo_media_id: null }, data: [] } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { context, page };
}

test('chromium: a requester with one portal starts in it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page } = await requester(browser, [DESK]);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await page.waitForURL(`**/portal/${DESK.collection_id}`, { timeout: 15_000 });
  assert.equal(await page.getByText('still has to give you access').count(), 0, 'the portal says there is no access');
});

test('chromium: a requester with several portals finds them on the start and in the navigation', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page } = await requester(browser, [DESK, FACILITIES]);
  t.after(() => context.close());

  await page.goto(`${served.origin}/`);
  await page.getByText('This is where you get help').waitFor({ timeout: 15_000 });
  assert.equal(await page.getByText('still has to give you access').count(), 0);
  const nav = page.getByRole('navigation', { name: 'Your portals' });
  assert.equal(await nav.getByText(FACILITIES.name).count(), 1, 'the navigation does not list the portal');
  await page.getByRole('button', { name: FACILITIES.name }).click();
  await page.waitForURL(`**/portal/${FACILITIES.collection_id}`, { timeout: 5_000 });
});
