// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installation's presets on *Installation → Defaults* (UC-INS-18 checks 1, 3, 4): creating one
// from one of Hubtask's, renaming one and starting its settings again, making one the default and
// locking its settings - each sent on the version read -, the default's withdrawal drawn
// unavailable with its reason, another's sent; a file that enforces the level shows the presets
// without a control; and in single mode no section at all. What a workspace then sees - the preset
// marked as the installation's, never its own default, the lock not its own to lift - is
// `presets.test.mjs`'s. Against the stubbed API; Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, PRESET_SETTINGS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const MULTI = { ...MANIFEST, tenancy_mode: 'multi' };

/** This walk's presets: identifiers no other fixture record uses. */
const CRAFT = { id: '01a0e2e0-0000-7000-8000-0000000028a1', name: 'Craft order', version: 3, settings: PRESET_SETTINGS.FLOW, is_default: true, locks: ['views', 'fields.TASK.DUE_DATE'] };
const PRACTICE = { id: '01a0e2e0-0000-7000-8000-0000000028a2', name: 'Practice appointments', version: 1, settings: PRESET_SETTINGS.TASKS, is_default: false, locks: [] };

/** The installation's own level, as `/admin/settings` answers it: one key is enough here. */
function level(isEnforced) {
  return {
    sign_in: { min_length: { set: true, value: 12, locked: false, kind: 'COUNT', minimum: 8, maximum: 128, position: 0, workspace_may: 'TIGHTEN' } },
    legal: {}, localisation: {}, quotas: {}, ai: {},
    source: isEnforced ? '/etc/hubtask/instance.yaml' : 'database',
    is_enforced_from_file: isEnforced,
  };
}

/** A signed-in operator on *Defaults*; every write is recorded. */
async function openDefaults(browser, { manifest = MULTI, isEnforced = false, width = 1280 } = {}) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const written = [];
  const read = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    if (method !== 'GET') written.push({ method, path, body: request.postDataJSON?.() ?? null, ifMatch: request.headers()['if-match'] });
    else read.push(path);
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/meta/capabilities') return route.fulfill({ json: manifest });
    if (path === '/admin/settings' && method === 'GET') return route.fulfill({ json: level(isEnforced), headers: { ETag: '"1"' } });
    if (path === '/admin/presets' && method === 'GET') return route.fulfill({ json: { data: [CRAFT, PRACTICE] } });
    if (path === '/admin/presets' && method === 'POST') {
      const body = request.postDataJSON();
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000028a9', name: body.name, version: 1, settings: PRESET_SETTINGS.LIST, is_default: false, locks: [] } });
    }
    if (path.startsWith('/admin/presets/') && method === 'PATCH') {
      const target = path.endsWith(CRAFT.id) ? CRAFT : PRACTICE;
      return route.fulfill({ json: { ...target, ...request.postDataJSON(), version: target.version + 1 } });
    }
    if (path.startsWith('/admin/presets/') && method === 'DELETE') return route.fulfill({ status: 204, body: '' });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/instance/settings`);
  await page.getByRole('heading', { name: 'Defaults', level: 1 }).first().waitFor({ timeout: 15_000 });
  return { page, written, read, failures, close: () => context.close() };
}

test('chromium: an operator creates, renames, starts again, makes default, locks and withdraws presets (checks 1, 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, written, failures, close } = await openDefaults(browser);
  t.after(close);

  const section = page.locator('[data-installation-presets]');
  const craft = section.locator(`[data-installation-preset="${CRAFT.id}"]`);
  await craft.waitFor({ timeout: 10_000 });
  const practice = section.locator(`[data-installation-preset="${PRACTICE.id}"]`);

  // Check 1: created from one of Hubtask's.
  const create = section.locator('[data-installation-preset-create]');
  await create.getByRole('textbox', { name: 'New preset' }).fill('Site visit');
  await create.getByLabel('Starts from').selectOption('PROJECT');
  await create.getByRole('button', { name: 'Create preset' }).click();
  await page.waitForTimeout(300);
  const created = written.find((write) => write.method === 'POST' && write.path === '/admin/presets');
  assert.deepEqual(created?.body, { name: 'Site visit', preset: 'PROJECT' });

  // Check 1: renamed, and its settings started again from one of Hubtask's, on the version read.
  await practice.getByRole('textbox').fill('Practice visits');
  await practice.getByRole('button', { name: 'Rename' }).click();
  await page.waitForTimeout(300);
  await practice.getByLabel('Settings').selectOption('LIST');
  await page.waitForTimeout(300);
  const changes = written.filter((write) => write.method === 'PATCH' && write.path === `/admin/presets/${PRACTICE.id}`);
  assert.deepEqual(changes.map((write) => write.body), [{ name: 'Practice visits' }, { preset: 'LIST' }]);
  assert.equal(changes[0]?.ifMatch, '"1"');

  // Check 3: the default's locks - a whole setting added, the capability lock kept.
  const locks = section.locator('[data-installation-preset-locks]');
  assert.equal(await locks.getByRole('checkbox', { name: 'Views' }).isChecked(), true, "the default's lock is not drawn");
  await locks.getByRole('checkbox', { name: 'Success' }).check();
  await page.waitForTimeout(300);
  const locked = written.filter((write) => write.method === 'PATCH' && write.path === `/admin/presets/${CRAFT.id}`).at(-1);
  assert.deepEqual(locked?.body, { locks: ['views', 'celebration', 'fields.TASK.DUE_DATE'] });
  assert.equal(locked?.ifMatch, '"3"');

  // Check 3: another preset made the default; none made the default.
  const defaultSelect = section.getByLabel('New workspaces start from');
  await defaultSelect.selectOption(PRACTICE.id);
  await page.waitForTimeout(300);
  assert.deepEqual(written.filter((write) => write.path === `/admin/presets/${PRACTICE.id}`).at(-1)?.body, { is_default: true });
  await defaultSelect.selectOption('');
  await page.waitForTimeout(300);
  assert.deepEqual(written.filter((write) => write.path === `/admin/presets/${CRAFT.id}`).at(-1)?.body, { is_default: false });

  // The default's withdrawal is unavailable, with its reason, and sends nothing; another's is sent.
  const withdrawDefault = craft.getByRole('button', { name: 'Withdraw “Craft order”' });
  assert.equal(await withdrawDefault.getAttribute('aria-disabled'), 'true', "the default's withdrawal is offered");
  await withdrawDefault.click({ force: true });
  const reason = await withdrawDefault.evaluate((button) => document.getElementById(button.getAttribute('aria-describedby') ?? '')?.textContent ?? '');
  assert.ok(reason.includes('New workspaces start from this preset'), `the reason reads ${JSON.stringify(reason)}`);
  assert.ok(!written.some((write) => write.method === 'DELETE' && write.path === `/admin/presets/${CRAFT.id}`), "the default's withdrawal was sent");
  await practice.getByRole('button', { name: 'Withdraw “Practice appointments”' }).click();
  await page.waitForTimeout(300);
  const withdrawn = written.find((write) => write.method === 'DELETE' && write.path === `/admin/presets/${PRACTICE.id}`);
  assert.ok(withdrawn, 'the withdrawal was not sent');
  assert.equal(withdrawn.ifMatch, '"1"');
  assert.deepEqual(failures, []);
});

test('chromium: a file that enforces the level shows the presets and offers no control (check 1)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, written, failures, close } = await openDefaults(browser, { isEnforced: true });
  t.after(close);

  const section = page.locator('[data-installation-presets]');
  const craft = section.locator(`[data-installation-preset="${CRAFT.id}"]`);
  await craft.waitFor({ timeout: 10_000 });
  const said = await craft.textContent();
  assert.ok(said?.includes('New workspaces start from this preset.'), `the default is not marked: ${JSON.stringify(said)}`);
  assert.ok(said?.includes('Views'), `the default's locks are not said: ${JSON.stringify(said)}`);
  assert.equal(await section.getByRole('textbox').count(), 0, 'a name is offered for change');
  assert.equal(await section.getByRole('button').count(), 0, 'a control is offered');
  assert.equal(await section.locator('select').count(), 0, 'a choice is offered');
  assert.deepEqual(written.filter((write) => write.path.startsWith('/admin/')), []);
  assert.deepEqual(failures, []);
});

test('chromium: a single installation shows no presets of the installation and reads none (check 4)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, read, failures, close } = await openDefaults(browser, { manifest: { ...MANIFEST, tenancy_mode: 'single' } });
  t.after(close);

  await page.getByRole('button', { name: 'Change these values' }).waitFor({ timeout: 10_000 });
  await page.waitForTimeout(600);
  assert.equal(await page.locator('[data-installation-presets]').count(), 0, 'the installation level is drawn in single mode');
  assert.ok(!read.includes('/admin/presets'), 'the installation presets were read in single mode');
  assert.deepEqual(failures, []);
});
