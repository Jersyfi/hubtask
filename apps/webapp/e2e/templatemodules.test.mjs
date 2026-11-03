// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A template a fresh workspace cannot take because its modules are off (UC-CFG-01): the dialog
// names the module in words, offers its owner to switch it on, plans again and applies - and
// shows neither a message code nor a key path.
//
// Chromium only; what the server decides is its own tests'.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, DEFAULT_STRUCTURE, MANIFEST, PAGE, PERMISSIONS, PRESETS, WORKSPACE, forgetUnstubbed, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SCRUM = {
  key: 'scrum_team', version: 1, name: 'Scrum team', origin: 'HUBTASK', provenance: 'scrum_team@1', document: {},
  created_at: '2026-09-01T00:00:00Z', update_available: false, taken_version: null,
};
const counts = { added: 0, changed: 0, removed: 0 };
const flags = { hides_data: false, changes_permissions: false };
const refused = {
  template: SCRUM.key, upgrade: [], document: {}, mode: 'MERGE', base_hash: 'b0', plan_hash: null, changes: [], operations: [], flags, counts,
  refusals: [
    { path: '/types', code: 'module.off', params: { module: 'WORKFLOWS' } },
    { path: '/workflows', code: 'module.off', params: { module: 'WORKFLOWS' } },
  ],
};
const planned = {
  ...refused, base_hash: 'b1', plan_hash: 'p1', refusals: [], counts: { ...counts, added: 1 },
  changes: [{ path: 'workflows.scrum', section: 'workflows', key: 'scrum', op: 'ADD', fields: [] }],
};

async function as(browser, role) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  forgetUnstubbed();
  const written = [];
  let workflowsOn = false;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (method !== 'GET') written.push({ method, path, body: request.postDataJSON?.() ?? null });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS[role] } });
    if (path === '/memberships') {
      const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    if (path === '/tenant' && method === 'GET') return route.fulfill({ json: { ...WORKSPACE, structure: { ...DEFAULT_STRUCTURE, default_preset: null, default_preset_id: null } } });
    if (path === '/presets' && method === 'GET') return route.fulfill({ json: PRESETS });
    if (path === '/config/templates' && method === 'GET') return route.fulfill({ json: { data: [SCRUM] } });
    if (path === `/config/templates/${SCRUM.key}:plan`) return route.fulfill({ json: workflowsOn ? planned : refused });
    if (path === '/tenant/modules/WORKFLOWS' && method === 'PUT') {
      workflowsOn = true;
      return route.fulfill({ json: { data: [] } });
    }
    if (path === '/config:apply') return route.fulfill({ json: { version: 2, applied: true } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  return { context, page, failures, written };
}

async function openScrum(page) {
  await page.goto(`${served.origin}/administration/presets`);
  const card = page.locator(`[data-template="${SCRUM.key}"]`);
  await card.waitFor({ timeout: 15_000 });
  await card.locator(`[data-take="${SCRUM.key}"]`).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor({ timeout: 5_000 });
  return dialog;
}

test('chromium: an empty workspace switches Workflows on and takes Scrum team', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  const dialog = await openScrum(page);
  const said = await dialog.textContent();
  assert.match(said, /Needs Workflows, which is switched off here/);
  for (const raw of ['module.off', 'config.section_withheld', '/workflows', '/types']) {
    assert.ok(!said.includes(raw), `the dialog shows ${raw}`);
  }
  await dialog.getByRole('button', { name: 'Switch on Workflows and use this template' }).click();
  await dialog.getByText('Adds the workflow “scrum”').waitFor({ timeout: 5_000 });
  assert.ok(written.some((write) => write.method === 'PUT' && write.path === '/tenant/modules/WORKFLOWS' && write.body?.switch === 'ON'), 'the module was not switched on');
  await dialog.locator('[data-apply-template]').click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.ok(written.some((write) => write.path === '/config:apply'), 'the template was not applied');
  assert.deepEqual(failures, []);
});
