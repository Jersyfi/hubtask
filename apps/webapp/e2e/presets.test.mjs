// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Presets of one's own and the workspace's (UC-WRK-30 checks 1-4), walked over the shared
// fixture's structure hub:
//
// * *Save as preset* on a collection's Structure panel sends the collection and the choice - for
//   me, or for the workspace where the reader may shape it; a member is not offered the workspace;
// * creating a collection offers Hubtask's, the workspace's and the reader's own presets, each
//   marked with where it comes from, and a saved one chosen is sent as `preset_id`;
// * the workspace's *Presets* view renames and withdraws a workspace preset, draws the default's
//   withdrawal unavailable with its reason, chooses the default and sets a lock - each sent as the
//   server expects.
//
// Chromium only; what the server decides is its own tests'.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, DEFAULT_STRUCTURE, MANIFEST, PAGE, PERMISSIONS, PRESETS, PRESET_SETTINGS, SHAPE_HUB, SHAPES, WORKSPACE, forgetUnstubbed, showStructures, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
showStructures();

/** This walk's own saved presets: identifiers no other fixture record uses. */
const SHOP = { origin: 'WORKSPACE', id: '01a0e2e0-0000-7000-8000-0000000027f1', name: 'Weekly shop', version: 2, settings: PRESET_SETTINGS.LIST };
const CLIENT = { origin: 'OWN', id: '01a0e2e0-0000-7000-8000-0000000027f2', name: 'Client project', version: 1, settings: PRESET_SETTINGS.PROJECT };
const DEFAULTED = { origin: 'WORKSPACE', id: '01a0e2e0-0000-7000-8000-0000000027f3', name: 'Household', version: 1, settings: PRESET_SETTINGS.TASKS };

/** A signed-in tab whose reader holds `role` on the workspace; every write is recorded. */
async function as(browser, role, { width = 1280, defaultPreset = DEFAULTED.id } = {}) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  forgetUnstubbed();
  const written = [];
  const structure = { ...DEFAULT_STRUCTURE, default_preset: null, default_preset_id: defaultPreset };
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (method !== 'GET') written.push({ method, path, body: request.postDataJSON?.() ?? null, ifMatch: request.headers()['if-match'] });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, workspace_permissions: PERMISSIONS[role] } });
    if (path === '/memberships') {
      const rows = url.searchParams.get('scope_type') === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role }] : [];
      return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
    }
    if (path === '/presets' && method === 'GET') return route.fulfill({ json: [...PRESETS, SHOP, DEFAULTED, CLIENT] });
    if (path === '/presets' && method === 'POST') {
      const body = request.postDataJSON();
      return route.fulfill({ status: 201, json: { origin: body.origin, id: '01a0e2e0-0000-7000-8000-0000000027f9', name: body.name, version: 1, settings: PRESET_SETTINGS.LIST } });
    }
    if (path.startsWith('/presets/') && method === 'PATCH') return route.fulfill({ json: { ...SHOP, ...request.postDataJSON(), version: SHOP.version + 1 } });
    if (path.startsWith('/presets/') && method === 'DELETE') return route.fulfill({ status: 204, body: '' });
    if (path === '/tenant' && method === 'GET') return route.fulfill({ json: { ...WORKSPACE, structure } });
    if (path === '/tenant/structure' && method === 'PATCH') return route.fulfill({ json: { ...structure, ...request.postDataJSON() } });
    if (path === '/containers' && method === 'POST') {
      return route.fulfill({ status: 201, json: { ...SHAPES.list, id: '01a0e2e0-0000-7000-8000-0000000027fa', name: request.postDataJSON().name } });
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
  return { context, page, failures, written };
}

async function openPanel(page, collection) {
  await page.goto(`${served.origin}/collections/${collection.id}`);
  await page.getByRole('heading', { name: collection.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  await page.waitForTimeout(600);
  await page.getByRole('button', { name: `Actions for ${collection.name}` }).click();
  await page.getByRole('menuitem', { name: 'Structure' }).click();
  const panel = page.locator('[data-structure-panel]');
  await panel.waitFor({ timeout: 5_000 });
  return panel;
}

test('chromium: Save as preset - for the workspace, sent with the collection (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  const panel = await openPanel(page, SHAPES.list);
  // The picker marks each preset with where it comes from (check 2).
  const options = await panel.getByLabel('Preset', { exact: true }).locator('option').allTextContents();
  for (const expected of ["List (Hubtask's)", "Weekly shop (The workspace's)", 'Client project (Yours)']) {
    assert.ok(options.includes(expected), `the picker offers ${JSON.stringify(options)}, not ${expected}`);
  }
  await panel.getByRole('button', { name: 'Save as preset' }).click();
  const dialog = page.locator('[data-save-preset]');
  await dialog.waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByRole('radio', { name: 'For the workspace' }).count(), 1, 'the workspace is not offered to its owner');
  assert.equal(await dialog.getByRole('radio', { name: 'Replace “Weekly shop”' }).count(), 1, "the workspace's preset cannot be replaced");
  // A name is asked for before anything is sent.
  await page.getByRole('button', { name: 'Save preset' }).click();
  assert.equal(await dialog.getByText('Give the preset a name.').count(), 1, 'a missing name is not said');
  await dialog.getByRole('radio', { name: 'For the workspace' }).check();
  await dialog.getByRole('textbox', { name: 'Name' }).fill('Groceries');
  await page.getByRole('button', { name: 'Save preset' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  const saved = written.find((write) => write.path === '/presets');
  assert.deepEqual(saved?.body, { name: 'Groceries', origin: 'WORKSPACE', container_id: SHAPES.list.id });
  assert.deepEqual(failures, []);
});

test('chromium: a member saves for themselves alone, and replacing their own keeps its name (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, written } = await as(browser, 'MEMBER');
  t.after(() => context.close());

  // A member holds no Structure panel on a collection they do not shape, so the walk grants the
  // hub: the workspace's shape is still not theirs.
  await context.route('**/api/v1/memberships**', (route) => {
    const url = new URL(route.request().url());
    const rows = url.searchParams.get('scope_type') === 'TENANT'
      ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'MEMBER' }]
      : url.searchParams.get('scope_type') === 'HUB'
        ? [{ id: 'm1', scope_type: 'HUB', scope_id: SHAPE_HUB.id, account_id: ACCOUNT.id, role: 'ADMIN' }]
        : [];
    return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: [] } });
  });
  const panel = await openPanel(page, SHAPES.list);
  await panel.getByRole('button', { name: 'Save as preset' }).click();
  const dialog = page.locator('[data-save-preset]');
  await dialog.waitFor({ timeout: 5_000 });
  assert.equal(await dialog.getByRole('radio', { name: 'For the workspace' }).count(), 0, 'the workspace is offered to a member');
  assert.equal(await dialog.getByRole('radio', { name: 'Replace “Weekly shop”' }).count(), 0, "a member may replace the workspace's preset");
  await dialog.getByRole('radio', { name: 'Replace “Client project”' }).check();
  await page.getByRole('button', { name: 'Save preset' }).click();
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  const replaced = written.find((write) => write.path === `/presets/${CLIENT.id}`);
  assert.deepEqual(replaced?.body, { container_id: SHAPES.list.id });
  assert.equal(replaced?.ifMatch, '"1"');
  assert.deepEqual(failures, []);
});

test("chromium: creating a collection offers every origin, marked, and sends a saved one's id (check 2)", async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures, written } = await as(browser, 'OWNER');
  t.after(() => context.close());

  await page.goto(`${served.origin}/hubs/${SHAPE_HUB.id}`);
  await page.getByRole('heading', { name: SHAPE_HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.getByRole('button', { name: 'Create collection' }).first().click();
  const dialog = page.locator('dialog[open]');
  await dialog.getByRole('radio', { name: 'Weekly shop' }).waitFor({ timeout: 5_000 });
  for (const [name, origin] of [['List', "Hubtask's"], ['Weekly shop', "The workspace's"], ['Client project', 'Yours']]) {
    const radio = dialog.getByRole('radio', { name, exact: true });
    assert.equal(await radio.count(), 1, `${name} is not offered`);
    const described = await radio.evaluate((input) => input.closest('.row')?.textContent ?? '');
    assert.ok(described.includes(origin), `${name} is marked ${JSON.stringify(described)}, not ${origin}`);
  }
  await dialog.getByRole('textbox', { name: 'Collection name' }).fill('Acme');
  await dialog.getByRole('radio', { name: 'Client project' }).check();
  await dialog.getByRole('button', { name: 'Create collection' }).click();
  await page.waitForFunction(() => document.querySelector('dialog[open]') === null, null, { timeout: 5_000 }).catch(() => {});
  const created = written.find((write) => write.path === '/containers');
  assert.equal(created?.body.preset_id, CLIENT.id);
  assert.equal(created?.body.preset, undefined);
  assert.deepEqual(failures, []);
});
