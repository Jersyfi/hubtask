// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installation's overview leaves a workspace that holds only service accounts out of its
// counts, and says how many there are, so the counts still add up with the list (UC-INS-12 check 5).
//
// Against a stubbed API. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const census = (serviceOnly) => ({
  workspaces_active: 3, workspaces_suspended: 0, workspaces_pending_deletion: 0,
  accounts_active: 7, accounts_total: 9, workspaces_service_only: serviceOnly,
});

async function overview(browser, serviceOnly) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/admin/overview') return route.fulfill({ json: census(serviceOnly) });
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance`);
  await page.getByText('Accounts in all').waitFor();
  const text = await page.locator('main').textContent() ?? '';
  await context.close();
  return text;
}

test('chromium: a service-only workspace is named apart from the counts, in the right number', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  assert.match(await overview(browser, 1), /1 more workspace holds only service accounts and is not counted\./);
  assert.match(await overview(browser, 2), /2 more workspaces hold only service accounts and are not counted\./);
  assert.doesNotMatch(await overview(browser, 0), /service accounts/);
});
