// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// *Changes* shows a plan before it applies (UC-CFG-01 check 7): an agent's proposal with the same
// preview - its changes and what it reaches, planned afresh and writing nothing -, applied only once
// what it hides was acknowledged; the sandbox's promotion with its reach. Against a stubbed API.
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const PROPOSAL = {
  id: '01a0e2e0-0000-7000-8000-0000000009c1', state: 'WAITING', mode: 'MERGE', base_hash: 'h1', plan_hash: 'p1',
  document: { fields: { cost: null } }, template: null, added: 0, changed: 0, removed: 1,
  hides_data: true, changes_permissions: false, proposed_by: '01a0e2e0-0000-7000-8000-0000000009c2',
  decided_by: null, created_at: '2026-10-30T09:00:00Z', decided_at: null, hub_id: null,
};
const FLAGS = { hides_data: true, changes_permissions: false };
const PLANNED = {
  base_hash: 'h1', plan_hash: 'p2', operations: [], refusals: [], flags: FLAGS, counts: { added: 0, changed: 0, removed: 1 },
  changes: [{ path: 'fields.cost', section: 'fields', key: 'cost', op: 'REMOVE', fields: [] }],
  reach: [{ path: 'fields.cost', hidden_values: 7 }, { path: 'modules.time', people: 12 }],
};
const SANDBOX = {
  exists: true, writes: 2, updated_at: '2026-10-30T10:00:00Z', document: {},
  promotion: {
    base_hash: 'h1', plan_hash: 'p3', operations: [], refusals: [], flags: { hides_data: false, changes_permissions: false },
    counts: { added: 0, changed: 1, removed: 0 },
    changes: [{ path: 'workflows.service', section: 'workflows', key: 'service', op: 'CHANGE', fields: ['shape'] }],
    reach: [{ path: 'workflows.service.shape', entries: 4 }],
  },
};

test('chromium: a proposal is previewed with its reach and applied only once acknowledged', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const planned = [];
  const applied = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/config/proposals') return route.fulfill({ json: { ...PAGE, data: [PROPOSAL] } });
    if (path === '/config/versions') return route.fulfill({ json: { ...PAGE, data: [] } });
    if (path === '/config' && request.method() === 'GET') return route.fulfill({ json: { document: {}, hash: 'h1', withheld: [] } });
    if (path === '/config/sandbox' && request.method() === 'GET') return route.fulfill({ json: SANDBOX });
    if (path === '/config:plan') {
      planned.push(request.postDataJSON());
      return route.fulfill({ json: PLANNED });
    }
    if (path.endsWith(':decide') || path === '/config:apply') {
      applied.push(path);
      return route.fulfill({ json: { state: 'APPLIED', version: 2 } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/administration/changes`);

  const row = page.locator(`[data-proposal="${PROPOSAL.id}"]`);
  await row.waitFor({ timeout: 15_000 });
  assert.match((await row.textContent()) ?? '', /Hides data/);

  // The sandbox's promotion says what it reaches before anybody promotes it.
  const sandbox = page.locator('[data-sandbox]');
  await sandbox.getByText('workflows.service.shape: 4 entries move to another status').waitFor({ timeout: 10_000 });

  // The proposal's preview: planned afresh with its own document, writing nothing.
  await row.locator(`[data-preview="${PROPOSAL.id}"]`).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByText('fields.cost: 7 entries show fewer values - nothing is deleted').waitFor({ timeout: 10_000 });
  assert.match((await dialog.textContent()) ?? '', /modules\.time: 12 people are left without the module/);
  assert.deepEqual(planned.at(-1), { document: PROPOSAL.document, mode: 'MERGE' });
  assert.deepEqual(applied, [], 'the preview applied something');
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 10_000 });

  // Apply waits for the acknowledgement of what the proposal hides.
  const apply = row.locator(`[data-apply="${PROPOSAL.id}"]`);
  assert.equal(await apply.getAttribute('aria-disabled'), 'true');
  await apply.click({ force: true });
  assert.deepEqual(applied, [], 'a proposal that hides data applied unacknowledged');
  await row.getByLabel(/I have seen that this hides data/).check();
  assert.equal(await apply.getAttribute('aria-disabled'), null);
  const decided = page.waitForRequest((request) => request.url().endsWith(`/config/proposals/${PROPOSAL.id}:decide`), { timeout: 10_000 });
  await apply.click();
  assert.deepEqual((await decided).postDataJSON(), { decision: 'APPLY', acknowledge: true });
  assert.deepEqual(failures, []);
});
