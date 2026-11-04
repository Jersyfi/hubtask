// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The template catalogue says what each template does (user-experience.md §5.2): a sentence with
// the standard it follows and the products it switches on, and a filter by family - so a newcomer
// tells *Internal IT* from *ITIL service desk* without taking either. Chromium only.

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

const template = (key, modules) => ({
  key, version: 1, name: key, origin: 'HUBTASK', provenance: `${key}@1`, created_at: '2026-09-01T00:00:00Z',
  update_available: false, taken_version: null, hub_document: {},
  document: { modules: Object.fromEntries(modules.map((name) => [name, { switch: 'ON' }])) },
});
const CATALOGUE = [
  template('itil_service_desk', ['workflows', 'service_desk', 'knowledge']),
  template('internal_it', ['workflows', 'service_desk']),
  template('household', ['workflows']),
];

for (const [width, scheme] of [[1280, 'light'], [375, 'dark']]) {
  test(`chromium ${width} px, ${scheme}: each template says what it does, and the catalogue filters by family`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const context = await browser.newContext({ viewport: { width, height: 900 }, colorScheme: scheme });
    t.after(() => context.close());
    forgetUnstubbed();
    await context.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url());
      const path = url.pathname.replace(/^.*\/api\/v1/, '');
      if (path === '/memberships') {
        const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' }] : [];
        return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
      }
      if (path === '/tenant' && route.request().method() === 'GET') return route.fulfill({ json: { ...WORKSPACE, structure: { ...DEFAULT_STRUCTURE, default_preset: null, default_preset_id: null } } });
      if (path === '/presets' && route.request().method() === 'GET') return route.fulfill({ json: PRESETS });
      if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS.OWNER } });
      if (path === '/config/templates' && route.request().method() === 'GET') return route.fulfill({ json: { data: CATALOGUE } });
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));
    await page.goto(`${served.origin}/administration/presets`);
    const itil = page.locator('[data-template="itil_service_desk"]');
    await itil.waitFor({ timeout: 15_000 });
    const said = await itil.innerText();
    assert.match(said, /ITIL 4/u, said);
    assert.match(said, /Service desk/u, `no product chip: ${said}`);
    assert.notEqual(await page.locator('[data-template="internal_it"] .summary').innerText(), await itil.locator('.summary').innerText());

    await page.getByRole('radiogroup', { name: 'Families' }).getByRole('radio', { name: 'Service', exact: true }).click();
    await page.locator('[data-template="household"]').waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await page.locator('[data-template]').count(), 2);
    assert.deepEqual(failures, []);
  });
}
