// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Today's product bands (user-experience.md §3.3, UC-WRK-34 check 8): *Waiting for your yes*
// where the service desk is on, *Time today* where time is on, each fed by its product's own read,
// and neither - nor its read - where the module is off. Walked at desk and phone width, light and
// dark.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PERMISSIONS, fallback } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-0000000000b1', kind: 'USER', display_name: 'Band Walker', email: 'bands@example.invalid', status: 'ACTIVE', locale: 'en', timezone: 'UTC', onboarding_completed_at: '2026-09-01T00:00:00Z' };
const HUB = { id: '01a0e2e0-0000-7000-8000-0000000000b2', type: 'HUB', parent_id: null, name: 'Operations', order_key: 'a0', version: 1 };
const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };
const ENTRY = '01a0e2e0-0000-7000-8000-0000000000b3';
const APPROVAL = {
  id: '01a0e2e0-0000-7000-8000-0000000000b4', item_id: ENTRY, item_title: 'New laptop for the design team', item_number: 42,
  on_behalf_of: null, workflow: 'purchase', step: 'Budget', item_version: 3, state: 'PENDING', current_level: 1,
  levels: [{ rule: 'ANY', count: 1, approvers: [ACCOUNT.id], roles: [], needed: 1 }], decisions: [],
  requested_by: ACCOUNT.id, requested_at: new Date(Date.now() - 3_600_000).toISOString(), decided_at: null, due_at: null, may_decide: true, version: 1,
};
const today = new Date().toISOString().slice(0, 10);
const AGENT = { id: '01a0e2e0-0000-7000-8000-0000000000c1', account_id: null, name: 'Request sorter', purpose: 'Sorts new requests', kind: 'TICKET_TRIAGE', owner_id: ACCOUNT.id, status: 'ACTIVE' };
const act = (n, kind) => ({ id: `01a0e2e0-0000-7000-8000-0000000000d${n}`, agent_id: AGENT.id, use_case: 'UpdateItem', class: kind, input: {}, status: 'PENDING', decided_by: null, decided_at: null, code: '', created_at: new Date(Date.now() - 600_000).toISOString() });
const ACTS = [act(1, 'REVERSIBLE'), act(2, 'IRREVERSIBLE')];

function stubFor({ modulesOn, asked, decided }) {
  return async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (path.endsWith('/api/v1/stream')) return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
    if (path.endsWith('/api/v1/sync:snapshot')) {
      const record = JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: HUB.id, container_id: null, payload: HUB });
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${record}\n{"cursor":"c-e2e"}\n` });
    }
    if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
    if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single', item_types: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], workspace_permissions: PERMISSIONS.OWNER, modules_on: modulesOn } });
    }
    if (path.endsWith('/api/v1/approvals')) {
      asked.push('approvals');
      return route.fulfill({ json: { data: decided.length > 0 ? [] : [APPROVAL] } });
    }
    if (path.endsWith(`/api/v1/approvals/${APPROVAL.id}:decide`)) {
      decided.push(request.postDataJSON());
      return route.fulfill({ json: { ...APPROVAL, state: 'APPROVED', may_decide: false } });
    }
    if (path.endsWith('/api/v1/agent-requests')) {
      asked.push('agent-requests');
      return route.fulfill({ json: { data: ACTS.filter((each) => !decided.some((d) => d.act === each.id)) } });
    }
    const deciding = /\/api\/v1\/agent-requests\/([^/]+):decide$/.exec(path);
    if (deciding) {
      decided.push({ act: deciding[1], ...request.postDataJSON() });
      return route.fulfill({ json: { ...ACTS.find((each) => each.id === deciding[1]), status: 'DECLINED' } });
    }
    if (path.endsWith('/api/v1/agents')) {
      asked.push('agents');
      return route.fulfill({ json: { data: [AGENT] } });
    }
    if (path.endsWith('/api/v1/timer')) {
      asked.push('timer');
      return route.fulfill({ json: { running: true, entry_id: ENTRY, work_type: 'dev', note: '', billable: false, started_at: new Date(Date.now() - 25 * 60_000).toISOString(), elapsed_minutes: 25 } });
    }
    if (/\/api\/v1\/timesheets\/\d{4}-\d{2}-\d{2}$/.test(path)) {
      asked.push('week');
      const week_start = path.slice(-10);
      return route.fulfill({ json: { account_id: ACCOUNT.id, week_start, state: 'OPEN', submitted_minutes: 0, submitted_at: null, decided_at: null, decided_by: null, reason: '', version: 1, records: [], days: [{ day: today, minutes: 150, locked: false }], minutes: 630 } });
    }
    if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') return route.fulfill({ json: { ...PAGE, data: [HUB] } });
    if (path.endsWith('/api/v1/containers')) return route.fulfill({ json: { ...PAGE, data: [] } });
    return fallback(route, request, path);
  };
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { width, scheme = 'light', modulesOn }) {
  const asked = [];
  const decided = [];
  const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme, timezoneId: 'UTC' });
  await context.route('**/api/v1/**', stubFor({ modulesOn, asked, decided }));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/`);
  await page.getByRole('heading', { name: /^(Good morning|Hello|Good evening)/u, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { page, failures, asked, decided, close: () => context.close() };
}

for (const [width, scheme] of [[1280, 'light'], [1280, 'dark'], [375, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: the desk's and time's bands stand on Today where they are on`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, decided, close } = await open(browser, { width, scheme, modulesOn: ['SERVICE_DESK', 'TIME'] });
    t.after(close);

    const yes = page.getByRole('region', { name: 'Waiting for your yes' });
    await yes.waitFor({ timeout: 10_000 });
    assert.match((await yes.textContent()) ?? '', /#42 New laptop for the design team/u);
    assert.equal(await yes.getByRole('link', { name: '#42 New laptop for the design team' }).getAttribute('href'), `/items/${ENTRY}`);

    const time = page.getByRole('region', { name: 'Time today' });
    await time.waitFor({ timeout: 10_000 });
    const words = (await time.textContent()) ?? '';
    assert.match(words, /2:30\s*today, of 10:30 this week/u, words);
    assert.match(words, /Timer running: 0:2[5-6]/u, words);

    const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
    assert.equal(scroll.width, scroll.viewport, `the page scrolls sideways (${scroll.width} of ${scroll.viewport})`);

    // The Today row carries the band's count at desk width; the phone's bar draws marks only.
    const todayRow = page.locator('nav [role="treeitem"], nav a').filter({ hasText: /^\s*Today/u }).first();
    if (width >= 1280) await todayRow.filter({ hasText: /1 waiting/u }).waitFor({ timeout: 10_000 });

    // Accepted at the row: one decision sent, and the band goes once nothing waits.
    await yes.getByRole('button', { name: 'Approve' }).click();
    for (let tries = 0; tries < 50 && decided.length === 0; tries++) await page.waitForTimeout(100);
    assert.deepEqual(decided.map((each) => each.verdict), ['APPROVE']);
    await yes.waitFor({ state: 'detached', timeout: 10_000 });
    if (width >= 1280) assert.doesNotMatch((await todayRow.textContent()) ?? '', /waiting/u, 'the count stays once nothing waits');
    assert.deepEqual(failures, []);
  });
}

test('chromium 1280 px: with the desk and time off, Today draws neither band and asks neither read', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, asked, close } = await open(browser, { width: 1280, modulesOn: ['KNOWLEDGE'] });
  t.after(close);
  await page.waitForLoadState('networkidle');
  assert.equal(await page.getByRole('region', { name: 'Waiting for your yes' }).count(), 0);
  assert.equal(await page.getByRole('region', { name: 'Time today' }).count(), 0);
  assert.deepEqual(asked, [], 'a read of a module that is off was sent');
  assert.deepEqual(failures, []);
});

// §3.3: where agents are on, their held acts wait in the same band; one that cannot be undone is
// approved only where its preview is read, so its row offers Decline alone.
for (const [width, scheme] of [[1280, 'dark'], [375, 'light']]) {
  test(`chromium ${width} px, ${scheme}: an agent's held acts wait for the reader's yes on Today`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, failures, decided, close } = await open(browser, { width, scheme, modulesOn: ['AGENTS'] });
    t.after(close);
    const yes = page.getByRole('region', { name: 'Waiting for your yes' });
    await yes.waitFor({ timeout: 10_000 });
    const reversible = yes.locator(`[data-held-act="${ACTS[0].id}"]`);
    const irreversible = yes.locator(`[data-held-act="${ACTS[1].id}"]`);
    await irreversible.waitFor({ timeout: 10_000 });
    assert.equal(await reversible.getByRole('link', { name: 'Request sorter asks before acting' }).getAttribute('href'), `/agents?tab=requests&agent=${ACTS[0].agent_id}`);
    assert.equal(await reversible.getByRole('button', { name: 'Approve and do it' }).count(), 1);
    assert.equal(await irreversible.getByRole('button', { name: 'Approve and do it' }).count(), 0, 'an act that cannot be undone is approved from the row');
    if (width >= 1280) await page.locator('nav [role="treeitem"], nav a').filter({ hasText: /^\s*Today/u }).filter({ hasText: /2 waiting/u }).first().waitFor({ timeout: 10_000 });
    const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
    assert.equal(scroll.width, scroll.viewport, `the page scrolls sideways (${scroll.width} of ${scroll.viewport})`);
    await irreversible.getByRole('button', { name: 'Decline' }).click();
    for (let tries = 0; tries < 50 && decided.length === 0; tries++) await page.waitForTimeout(100);
    assert.deepEqual(decided, [{ act: ACTS[1].id, approve: false }]);
    await irreversible.waitFor({ state: 'detached', timeout: 10_000 });
    assert.deepEqual(failures, []);
  });
}
