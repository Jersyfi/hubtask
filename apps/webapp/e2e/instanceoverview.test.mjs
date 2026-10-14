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

import { MANIFEST, stub } from './fixture.mjs';
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

const OPERATOR = (n) => ({
  tenant_id: '01a0e2e0-0000-7000-8000-0000000005a1', account_id: `01a0e2e0-0000-7000-8000-00000000070${n}`,
  added_at: '2026-10-01T08:00:00Z', display_name: `Operator ${n}`, kind: 'USER',
  workspace_slug: 'house', workspace_name: 'House',
});

/** The overview's text with a register of `count` operators and the health report given. */
async function overviewWith(browser, { operators, health }) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/admin/overview') return route.fulfill({ json: census(0) });
    if (path === '/admin/operators') return route.fulfill({ json: operators });
    if (path === '/meta/health' && health) return route.fulfill({ json: health });
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
  await page.waitForFunction(() => /run this installation/.test(document.querySelector('main')?.textContent ?? ''), null, { timeout: 10_000 });
  const text = await page.locator('main').textContent() ?? '';
  await context.close();
  return text;
}

// UC-INS-12 check 2: the operators are counted right, in the right number, and an empty register is
// said as what it is to the owner who runs the installation through it.
test('chromium: the operators are counted in words, and an empty register names the owner', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const empty = await overviewWith(browser, { operators: [] });
  assert.match(empty, /You run this installation because it has one workspace/);
  assert.doesNotMatch(empty, /0 accounts/);
  assert.match(await overviewWith(browser, { operators: [OPERATOR(1)] }), /1 account may run this installation/);
  assert.match(await overviewWith(browser, { operators: [OPERATOR(1), OPERATOR(2)] }), /2 accounts may run this installation/);
});

// The health report's hint on a provider's installation without an accessibility statement, in words.
test('chromium: the overview says what the health report finds missing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const text = await overviewWith(browser, {
    operators: [OPERATOR(1)],
    health: {
      status: 'ok', version: '0.1.0', degraded_features: [],
      warnings: [{ code: 'config.accessibility_statement_missing', severity: 'info', params: { command: 'hubctl admin settings set legal.accessibility_url <address>' } }],
    },
  });
  assert.match(text, /names no accessibility statement/);
  assert.match(text, /hubctl admin settings set legal\.accessibility_url/);
});

/** A level of four settings in three areas: two decided, one of them locked. */
const LEVEL = {
  sign_in: {
    min_length: { set: true, value: 14, locked: true, kind: 'COUNT' },
    breach_check: { set: false, locked: false, kind: 'FLAG' },
  },
  legal: { imprint_url: { set: false, locked: false, kind: 'LINK' } },
  quotas: { items: { set: true, value: 100, locked: false, kind: 'COUNT' } },
  source: 'database',
  is_enforced_from_file: false,
};

async function overviewOf(browser, tenancy) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: tenancy } });
    if (path === '/admin/overview') return route.fulfill({ json: census(0) });
    if (path === '/admin/settings') return route.fulfill({ json: LEVEL });
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
  await page.waitForFunction(() => /settings decided/.test(document.querySelector('main')?.textContent ?? ''), null, { timeout: 10_000 });
  const text = await page.locator('main').textContent() ?? '';
  await context.close();
  return text;
}

// UC-INS-12 check 1: the overview's line on the defaults counts what *Defaults* counts - every area,
// not the sign-in switches alone.
test('chromium: the overview counts every setting, as the defaults do', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const text = await overviewOf(browser, 'single');
  assert.match(text, /2 of 4 settings decided, 1 of them locked/, text);
});

// deployments.md D1: a single installation cannot suspend or delete its one workspace, so it is
// shown no counter for either; an installation of many is.
test('chromium: only an installation of many counts suspended and deleted workspaces', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const single = await overviewOf(browser, 'single');
  assert.doesNotMatch(single, /Suspended|Being deleted/, single);
  const multi = await overviewOf(browser, 'multi');
  assert.match(multi, /Suspended/);
  assert.match(multi, /Being deleted/);
});
