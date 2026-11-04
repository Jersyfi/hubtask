// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A module switched on is drawn on at once, without a reload (UC-INS-21), with a card that says
// where it is now and what it still needs (user-experience.md §2.8), and a module's page
// reached while it is off says so in place of its forms - no read refused, no form that fails -
// with the switch at hand. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { productWalk } from './productwalk.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const template = (key, modules) => ({
  key, version: 1, name: key, origin: 'HUBTASK', provenance: `${key}@1`, created_at: '2026-09-01T00:00:00Z',
  update_available: false, taken_version: null, hub_document: {},
  document: { modules: Object.fromEntries(modules.map((name) => [name, { switch: 'ON' }])) },
});

/** The workspace's switches, kept in memory as the server keeps them. */
function modulesBackend(off) {
  const states = ['PLANNING', 'TIME', 'SERVICE_DESK'].map((module) => ({ module, on: !off.includes(module), source: 'DEFAULT' }));
  return (method, path) => {
    if (path === '/tenant/modules' && method === 'GET') return { json: { modules: states } };
    // The administration's frame reads which AI answers; a workspace that configured none is the
    // contract's 404, which the walk counts as a failed request, so this one has a provider.
    if (path === '/ai-provider' && method === 'GET') {
      return { json: { kind: 'NOOP', jurisdiction: 'EEA', processing_allowed: false, has_api_key: false, created_at: '2026-09-01T00:00:00Z', version: 1 } };
    }
    if (path === '/config/templates' && method === 'GET') return { json: { data: [template('itil_service_desk', ['workflows', 'service_desk']), template('kanban_team', ['workflows'])] } };
    const one = /^\/tenant\/modules\/([A-Z_]+)$/.exec(path);
    if (one && method === 'PUT') {
      const state = states.find((each) => each.module === one[1]);
      Object.assign(state, { on: true, source: 'WORKSPACE' });
      return { json: { modules: states } };
    }
    return undefined;
  };
}

test('chromium: a module switched on is drawn on without a reload', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['PLANNING', 'TIME', 'SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/modules`);
  for (const module of ['PLANNING', 'TIME']) {
    const row = page.locator(`[data-module="${module}"]`);
    const control = row.getByRole('switch');
    await control.waitFor({ timeout: 10_000 });
    assert.equal(await control.isChecked(), false, `${module} starts off`);
    await row.locator('label').first().click();
    await row.locator(`[data-reset="${module}"]`).waitFor({ timeout: 5_000 });
    assert.equal(await control.isChecked(), true, `${module} is drawn off after its write switched it on`);
    // The card under the switch says where the product is now (user-experience.md §2.8), one at a time.
    const card = page.locator(`[data-switched-on="${module}"]`);
    await card.waitFor({ timeout: 5_000 });
    assert.equal(await page.locator('[data-switched-on]').count(), 1, 'an earlier card stayed');
  }
  const time = page.locator('[data-switched-on="TIME"]');
  assert.equal(await time.getByRole('link', { name: 'Find it in Time' }).getAttribute('href'), '/time');
  assert.equal(await time.getByRole('link', { name: /^Set up time/u }).getAttribute('href'), '/time?tab=settings');
  await time.getByRole('button', { name: 'Close' }).click();
  await time.waitFor({ state: 'detached', timeout: 5_000 });
  // Beside each switch that is on, its product's settings (user-experience.md §8.2); none while off.
  assert.equal(await page.locator('[data-module="TIME"]').getByRole('link', { name: 'Time settings' }).getAttribute('href'), '/time?tab=settings');
  assert.equal(await page.locator('[data-module-settings="SERVICE_DESK"]').count(), 0, 'a module that is off links its settings');
  assert.deepEqual(walk.errors(), []);
});

// §2.8: the card's *Start with…* is the templates that need the module, one press each - at the
// workspace the press opens *Create hub* with that template chosen.
test('chromium: a module switched on starts with the templates that need it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;

  await page.goto(`${served.origin}/administration/modules`);
  const row = page.locator('[data-module="SERVICE_DESK"]');
  await row.getByRole('switch').waitFor({ timeout: 10_000 });
  await row.locator('label').first().click();
  const card = page.locator('[data-switched-on="SERVICE_DESK"]');
  const start = card.locator('[data-start-with]');
  await start.getByRole('button', { name: 'ITIL service desk', exact: true }).waitFor({ timeout: 5_000 });
  assert.equal(await start.locator('[data-start-template]').count(), 1, 'a template that does not need the desk is offered');
  await start.getByRole('button', { name: 'ITIL service desk', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.locator('[data-hub-template="itil_service_desk"] [aria-pressed="true"]').waitFor({ timeout: 5_000 });
  assert.deepEqual(walk.errors(), []);
});

test('chromium: the service desk page with the module off says so, reads nothing and offers the switch', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const walk = await productWalk(browser, { modules: [], answer: modulesBackend(['SERVICE_DESK']) });
  t.after(walk.close);
  const { page } = walk;
  const asked = [];
  page.on('request', (request) => asked.push(new URL(request.url()).pathname));

  await page.goto(`${served.origin}/administration/service-desk`);
  await page.getByText('Service desk is switched off in this workspace').waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('main form').count(), 0, 'a form is offered that the server refuses');
  assert.equal(await page.getByRole('navigation', { name: 'Where you are in the administration' }).count(), 1, 'no trail back to the administration');
  assert.deepEqual(asked.filter((path) => /business-calendars|sla-policies|reply-templates|requester-organisations/.test(path)), [], 'the desk was read while off');

  await page.getByRole('button', { name: 'Switch Service desk on' }).click();
  await page.waitForTimeout(300);
  assert.ok(walk.written.some((each) => each.method === 'PUT' && each.path === '/tenant/modules/SERVICE_DESK'), 'the switch was not sent');
  assert.deepEqual(walk.errors(), []);
});
