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
const ITIL = template('itil_service_desk', ['workflows', 'service_desk', 'knowledge']);
ITIL.document.types = { incident: { name: 'Incident' }, service_request: { name: 'Service request' } };
ITIL.hub_document = { template_pages: { how_we_work: { title: 'How we work with this desk' } } };
/** What taking any of them would do: nothing, as far as this walk looks. */
const PLAN = {
  template: 'itil_service_desk', upgrade: [], document: {}, mode: 'MERGE', base_hash: 'b0', plan_hash: 'p0', changes: [], operations: [],
  flags: { hides_data: false, changes_permissions: false }, counts: { added: 0, changed: 0, removed: 0 }, refusals: [],
};
const CATALOGUE = [
  ITIL,
  template('internal_it', ['workflows', 'service_desk']),
  template('household', ['workflows']),
  // One of every other family, so the filter holds all of them at 375 px.
  template('scrum_team', ['workflows', 'planning']),
  template('classic_project', ['workflows', 'planning']),
  template('okr', ['goals']),
  template('approvals', ['workflows']),
  template('risk_register', ['workflows']),
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
      if (path.endsWith(':plan')) return route.fulfill({ json: PLAN });
      return stub(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', (error) => failures.push(String(error)));
    await page.goto(`${served.origin}/administration/templates`);
    // Each template a card on the canvas (page-system §9.1), named by its button.
    const card = (name) => page.getByRole('listitem').filter({ has: page.getByRole('button', { name, exact: true }) });
    const itil = card('ITIL service desk');
    await itil.waitFor({ timeout: 15_000 });
    const said = await itil.innerText();
    assert.match(said, /ITIL 4/u, said);
    assert.match(said, /Service desk/u, `no product chip: ${said}`);
    assert.notEqual(await card('Internal IT').locator('.description').innerText(), await itil.locator('.description').innerText());
    // What taking it creates is said in its dialog, beside the questions.
    assert.equal(await page.locator('[data-template-preview]').count(), 0);
    await itil.getByRole('button', { name: 'ITIL service desk', exact: true }).click();
    const dialog = page.locator('dialog[open]');
    await dialog.locator('[data-template-preview]').waitFor({ timeout: 5_000 });
    assert.equal(await dialog.getAttribute('data-size'), 'l');
    const preview = await dialog.locator('[data-template-preview]').innerText();
    assert.match(preview, /Entry types\s+Incident, Service request/u, preview);
    assert.match(preview, /Pages\s+How we work with this desk/u, preview);
    await page.keyboard.press('Escape');
    await page.waitForFunction(() => document.querySelector('dialog[open]') === null, null, { timeout: 5_000 });

    // The family filter is a chip - on a phone inside *Filters* - and the page stays inside its width.
    const sideways = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    assert.equal(sideways, 0, `the page scrolls sideways by ${sideways} px`);
    if (width < 600) await page.getByRole('button', { name: /^Filters/u }).click();
    else await page.getByRole('button', { name: 'Family: All' }).click();
    const families = page.getByRole('group', { name: 'Families' }).last();
    const filter = await families.evaluate((element) => ({ scroll: element.scrollWidth, client: element.clientWidth }));
    assert.ok(filter.scroll <= filter.client, `the family filter scrolls sideways at ${width} px: ${filter.scroll} > ${filter.client}`);
    await families.getByRole('radio', { name: 'Service', exact: true }).check();
    await card('Household').waitFor({ state: 'detached', timeout: 5_000 });
    assert.equal(await page.getByRole('list', { name: 'Catalogue' }).getByRole('listitem').count(), 2);
    assert.deepEqual(failures, []);
  });
}
