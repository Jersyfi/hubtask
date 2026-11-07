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

// user-experience.md §8.4: a degraded feature is said by its name with its reason and since when,
// the schema as the Upgrade row's state, and a backlog above its alert's line in the warning tone.
// The section that promised later screens is gone.
test('chromium: the health report is said in words, with the upgrade row and the backlogs', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const since = new Date(Date.now() - 2 * 3_600_000).toISOString();
  const context = await browser.newContext({ viewport: { width: 375, height: 812 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/admin/overview') return route.fulfill({ json: census(0) });
    if (path === '/meta/health') {
      return route.fulfill({ json: {
        status: 'degraded', version: '1.4.0',
        degraded_features: [
          { feature: 'semantic_search', reason_code: 'dependency.unavailable', since },
          { feature: 'media', reason_code: 'dependency.unavailable', since },
        ],
        migration: { applied: 140, expected: 141, status: 'behind' },
        backlogs: { sampled_at: new Date().toISOString(), dead_letter_total: 3, outbox_lag_seconds: 4, job_queue_depth: 12, webhook_retry_backlog: 0 },
      } });
    }
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
  const degraded = page.locator('[data-degraded]');
  await degraded.waitFor({ timeout: 15_000 });
  const lines = await degraded.locator('li').allTextContents();
  assert.match(lines[0], /Search by meaning/);
  assert.match(lines[0], /cannot be reached at the moment/);
  assert.match(lines[0], /since 2 hours ago/);
  assert.doesNotMatch(lines.join(' '), /semantic_search/);
  // Search by meaning is changed on the installation's AI screen; files and images have no screen.
  assert.equal(await degraded.locator('li').nth(0).getByRole('button', { name: 'Open its settings' }).count(), 1);
  assert.equal(await degraded.locator('li').nth(1).getByRole('button').count(), 0);

  const upgrade = await page.locator('[data-upgrade]').textContent();
  assert.match(upgrade ?? '', /Version 1\.4\.0/);
  assert.match(upgrade ?? '', /waiting for a migration/);

  assert.equal(await page.locator('[data-backlog="dead_letters"]').getAttribute('data-tone'), 'warning');
  assert.equal(await page.locator('[data-backlog="outbox_lag"]').getAttribute('data-tone'), null);
  assert.equal(await page.locator('[data-backlog="job_queue"]').getAttribute('data-tone'), null);
  const text = await page.locator('main').textContent() ?? '';
  assert.doesNotMatch(text, /What this level will hold/);
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(overflow <= 0, `the page scrolls sideways by ${overflow}px`);
});

/** The installation's screens, and the word each puts at the end of its trail. */
const INSTALLATION_SECTION = [
  ['/instance', 'Installation'],
  ['/instance/workspaces', 'Workspaces'],
  ['/instance/usage', 'Usage'],
  ['/instance/plans', 'Plans'],
  ['/instance/settings', 'Defaults'],
  ['/instance/providers', 'Sign-in providers'],
  ['/instance/ai', 'AI'],
  ['/instance/backup-targets', 'Backup targets'],
  ['/instance/platform', 'Events to the platform'],
  ['/instance/encryption', 'Keyring'],
  ['/instance/operators', 'Operators'],
  ['/instance/journal', 'Journal'],
];

// ux-inventory.md §E8, §E10: breadcrumbs everywhere, the installation included.
test('chromium: every screen of the installation says where it is and leads back', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 812 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: 'multi' } });
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

  const untrailed = [];
  for (const [route, word] of INSTALLATION_SECTION) {
    await page.goto(`${served.origin}${route}`);
    const trail = page.getByRole('navigation', { name: 'Where you are in the installation' });
    const shown = await trail.textContent({ timeout: 4_000 }).catch(() => undefined);
    if (!shown || !shown.includes('Installation') || !shown.includes(word)) untrailed.push(`${route} (${shown})`);
  }
  assert.deepEqual(untrailed, [], `these screens do not say where they are: ${untrailed.join(', ')}`);

  await page.goto(`${served.origin}/instance/journal`);
  await page.getByRole('navigation', { name: 'Where you are in the installation' }).getByRole('link', { name: 'Installation' }).click();
  await page.waitForFunction(() => location.pathname === '/instance', null, { timeout: 5_000 });
});

/** The usage screen against the given answer of `GET /admin/usage`. */
async function usageScreen(browser, answer) {
  const context = await browser.newContext({ viewport: { width: 375, height: 812 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, tenancy_mode: 'multi' } });
    if (path === '/admin/usage') return route.fulfill(answer);
    if (path === '/admin/tenants') return route.fulfill({ json: [{ id: '01a0e2e0-0000-7000-8000-0000000005a1', slug: 'house', display_name: 'House', status: 'ACTIVE', created_at: '2026-08-01T08:00:00Z' }] });
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
  await page.goto(`${served.origin}/instance/usage`);
  return { page, close: () => context.close() };
}

// user-experience.md §8.4: *Usage* draws each workspace's latest people, entries and stored bytes
// once metering is on, and says how to switch it on while it is off - never as a failure.
test('chromium: usage shows each workspace\'s latest counts, and how to switch metering on', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());

  const on = await usageScreen(browser, { json: {
    data: [{ workspace_id: '01a0e2e0-0000-7000-8000-0000000005a1', purged: false, metrics: {
      accounts: [{ day: '2026-11-03', value: 6 }, { day: '2026-11-04', value: 7 }],
      items: [{ day: '2026-11-04', value: 1234 }],
      media_bytes: [{ day: '2026-11-04', value: 2048 }],
    } }],
    page: { next_cursor: null, has_more: false },
    metrics: [{ name: 'items', aggregate: 'HIGHEST', day: 'UTC' }],
    final_through: '2026-11-03',
  } });
  t.after(on.close);
  const row = on.page.locator('[data-usage="01a0e2e0-0000-7000-8000-0000000005a1"]');
  await row.waitFor({ timeout: 15_000 });
  const text = await row.textContent() ?? '';
  assert.match(text, /house/);
  assert.match(text, /7/);
  assert.match(text, /1,234/);
  assert.match(text, /2 kB/);
  const overflow = await on.page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(overflow <= 0, `the page scrolls sideways by ${overflow}px`);

  const off = await usageScreen(browser, { status: 409, contentType: 'application/problem+json', json: {
    type: 'about:blank', title: 'Conflict', status: 409, code: 'conflict', detail_code: 'admin.metering_off',
  } });
  t.after(off.close);
  await off.page.getByText('Metering is off, so no use is offered.').waitFor({ timeout: 15_000 });
  await off.page.getByText(/hubctl admin settings set metering on/).waitFor();
  assert.equal(await off.page.getByRole('alert').filter({ hasText: /conflict/i }).count(), 0);
});
