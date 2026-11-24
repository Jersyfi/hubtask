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

import { sharedChromium } from './browser.mjs';

import { chooseInRow, rowMenu } from './rowmenu.mjs';

import { MANIFEST, PRESET_SETTINGS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { chipsOf, focused, pick } from './several.mjs';

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
  // The presets as the server holds them after each write, so that a read after a write answers
  // what was written (e2e/AGENTS.md, "No write asserted once"); one default at a time.
  const stored = new Map([[CRAFT.id, CRAFT], [PRACTICE.id, PRACTICE]]);
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
    if (path === '/admin/presets' && method === 'GET') return route.fulfill({ json: { data: [...stored.values()] } });
    if (path === '/admin/presets' && method === 'POST') {
      const body = request.postDataJSON();
      return route.fulfill({ status: 201, json: { id: '01a0e2e0-0000-7000-8000-0000000028a9', name: body.name, version: 1, settings: PRESET_SETTINGS.LIST, is_default: false, locks: [] } });
    }
    if (path.startsWith('/admin/presets/') && method === 'PATCH') {
      const target = stored.get(path.endsWith(CRAFT.id) ? CRAFT.id : PRACTICE.id);
      // The server answers a preset named by its kind with that kind's settings, never the kind itself.
      const { preset, ...change } = request.postDataJSON();
      const next = { ...target, ...change, ...(preset ? { settings: PRESET_SETTINGS[preset] } : {}), version: target.version + 1 };
      if (next.is_default) for (const [id, other] of stored) if (id !== next.id) stored.set(id, { ...other, is_default: false });
      stored.set(next.id, next);
      return route.fulfill({ json: next });
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
  await page.goto(`${served.origin}/instance/templates`);
  // The installation's templates are a row of their own (user-experience.md §8.4).
  await page.getByRole('heading', { name: 'Templates', level: 1 }).first().waitFor({ timeout: 15_000 });
  return { page, written, read, failures, close: () => context.close() };
}

/**
 * The answer to the next `method` on `path`, taken before the act that sends it: the stub records a
 * write before it answers, so once this settles the write is in `written`. It settles without an
 * answer too, and the walk's own assertion says what is missing.
 */
const answered = (page, method, path) =>
  page.waitForResponse((response) => response.request().method() === method && new URL(response.url()).pathname.endsWith(`/api/v1${path}`), { timeout: 10_000 })
    .catch(() => undefined);

test('chromium: an operator creates, renames, starts again, makes default, locks and withdraws presets (checks 1, 3)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, failures, close } = await openDefaults(browser);
  t.after(close);

  const section = page.locator('[data-installation-presets]');
  const rowOf = (id) => section.getByRole('row').filter({ has: page.locator(`[data-installation-preset="${id}"]`) });
  const craft = rowOf(CRAFT.id);
  await craft.waitFor({ timeout: 10_000 });
  const practice = rowOf(PRACTICE.id);

  // Check 1: created from one of Hubtask's, in its Dialog M.
  await page.getByRole('main').getByRole('button', { name: 'New preset' }).click();
  const create = page.locator('[data-installation-preset-create]');
  await create.getByRole('textbox', { name: 'New preset' }).fill('Site visit');
  await create.getByLabel('Starts from').selectOption('PROJECT');
  // Each act below is taken once the one before has ended - its dialog closed -, as a person
  // does: the page takes one write at a time.
  const creating = page.getByRole('dialog', { name: 'New preset' });
  await creating.getByRole('button', { name: 'Create preset' }).click();
  await creating.waitFor({ state: 'hidden', timeout: 10_000 });
  const created = written.find((write) => write.method === 'POST' && write.path === '/admin/presets');
  assert.deepEqual(created?.body, { name: 'Site visit', preset: 'PROJECT' });

  // Check 1: renamed, and its settings started again from one of Hubtask's, on the version read.
  await chooseInRow(practice, 'Rename');
  const renaming = page.getByRole('dialog', { name: 'Rename “Practice appointments”' });
  await renaming.getByRole('textbox').fill('Practice visits');
  await renaming.getByRole('button', { name: 'Rename' }).click();
  await renaming.waitFor({ state: 'hidden', timeout: 10_000 });
  // The read after the write: the row says the new name.
  await practice.getByText('Practice visits', { exact: true }).waitFor({ timeout: 10_000 });
  await chooseInRow(practice, 'Start again from…');
  const restarting = page.getByRole('dialog', { name: /^Start “/ });
  await restarting.getByLabel('Settings').selectOption('LIST');
  await restarting.getByRole('button', { name: 'Start again' }).click();
  await restarting.waitFor({ state: 'hidden', timeout: 10_000 });
  const changes = written.filter((write) => write.method === 'PATCH' && write.path === `/admin/presets/${PRACTICE.id}`);
  assert.deepEqual(changes.map((write) => write.body), [{ name: 'Practice visits' }, { preset: 'LIST' }]);
  assert.equal(changes[0]?.ifMatch, '"1"');

  // Check 3: the default's locks - a whole setting added, the capability lock kept - in the head's
  // *Default and locks…*.
  await page.getByRole('main').getByRole('button', { name: 'Default and locks…' }).click();
  const defaults = page.getByRole('dialog', { name: 'Default and locks…' });
  // The nine settings are one field named by its question (design-system.md §4).
  const locks = defaults.locator('[data-installation-preset-locks]').getByRole('button', { name: /^Locked in every workspace to/u });
  assert.equal(await locks.getAttribute('aria-haspopup'), 'listbox', 'the locks are not one field');
  assert.deepEqual(await chipsOf(locks), ['Views'], "the default's lock is not drawn");
  // Each press writes at once; what the list holds follows the answer.
  const lockWritten = answered(page, 'PATCH', `/admin/presets/${CRAFT.id}`);
  await pick(page, locks, ['Success']);
  assert.equal(await defaults.isVisible(), true, 'Escape closed the dialog, not only the list');
  assert.match(await focused(page), /^button "Locked in every workspace to /u, 'closing the list left focus elsewhere');
  await lockWritten;
  const locked = written.filter((write) => write.method === 'PATCH' && write.path === `/admin/presets/${CRAFT.id}`).at(-1);
  assert.deepEqual(locked?.body, { locks: ['views', 'celebration', 'fields.TASK.DUE_DATE'] });
  assert.equal(locked?.ifMatch, '"3"');
  // The screen after the write: the presets read again draw the lock added.
  await locks.locator('.chips:not(.measure) .chip').nth(1).waitFor({ timeout: 5_000 });
  assert.deepEqual(await chipsOf(locks), ['Views', 'Success'], 'the lock written is not drawn after the read');

  // Check 3: another preset made the default; none made the default. Each choice is made once the
  // read after the one before says who holds the default: the locks' field names it.
  const defaultSelect = defaults.getByLabel('New workspaces start from');
  const locksOf = (name) => defaults.getByRole('button', { name: new RegExp(`^Locked in every workspace to “${name}”`, 'u') });
  let madeDefault = answered(page, 'PATCH', `/admin/presets/${PRACTICE.id}`);
  await defaultSelect.selectOption(PRACTICE.id);
  await madeDefault;
  assert.deepEqual(written.filter((write) => write.path === `/admin/presets/${PRACTICE.id}`).at(-1)?.body, { is_default: true });
  await locksOf('Practice visits').waitFor({ timeout: 10_000 });
  // The default is withdrawn from the preset that holds it now - Practice, once the read after the
  // last write says so.
  madeDefault = answered(page, 'PATCH', `/admin/presets/${PRACTICE.id}`);
  await defaultSelect.selectOption('');
  await madeDefault;
  assert.deepEqual(written.filter((write) => write.path === `/admin/presets/${PRACTICE.id}`).at(-1)?.body, { is_default: false });
  await defaults.locator('[data-installation-preset-locks]').waitFor({ state: 'detached', timeout: 10_000 });
  // And Craft made the default again, for the withdrawal below.
  madeDefault = answered(page, 'PATCH', `/admin/presets/${CRAFT.id}`);
  await defaultSelect.selectOption(CRAFT.id);
  await madeDefault;
  assert.deepEqual(written.filter((write) => write.path === `/admin/presets/${CRAFT.id}`).at(-1)?.body, { is_default: true });
  await locksOf('Craft order').waitFor({ timeout: 10_000 });

  await page.keyboard.press('Escape');
  await defaults.waitFor({ state: 'hidden' });

  // The default's withdrawal is unavailable, with its reason, and sends nothing; another's is sent.
  await rowMenu(craft).click();
  const withdrawDefault = page.getByRole('menuitem', { name: /^Withdraw/ });
  assert.equal(await withdrawDefault.getAttribute('aria-disabled'), 'true', "the default's withdrawal is offered");
  await withdrawDefault.click({ force: true });
  const reason = await withdrawDefault.evaluate((item) => document.getElementById(item.getAttribute('aria-describedby') ?? '')?.textContent ?? '');
  // The try showed the reason's bubble: the first Escape closes it, the second the menu.
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await page.getByRole('menu').waitFor({ state: 'hidden' });
  assert.ok(reason.includes('New workspaces start from this preset'), `the reason reads ${JSON.stringify(reason)}`);
  assert.ok(!written.some((write) => write.method === 'DELETE' && write.path === `/admin/presets/${CRAFT.id}`), "the default's withdrawal was sent");
  await chooseInRow(practice, 'Withdraw');
  const withdrawing = page.getByRole('dialog', { name: /^Withdraw “/ });
  await withdrawing.getByRole('button', { name: 'Withdraw' }).click();
  await withdrawing.waitFor({ state: 'hidden', timeout: 10_000 });
  const withdrawn = written.find((write) => write.method === 'DELETE' && write.path === `/admin/presets/${PRACTICE.id}`);
  assert.ok(withdrawn, 'the withdrawal was not sent');
  // On the version read last: each write of Practice above raised it by one from 1.
  const practiceWrites = written.filter((write) => write.method === 'PATCH' && write.path === `/admin/presets/${PRACTICE.id}`).length;
  assert.equal(withdrawn.ifMatch, `"${PRACTICE.version + practiceWrites}"`);
  assert.deepEqual(failures, []);
});

test('chromium: a file that enforces the level shows the presets and offers no control (check 1)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, written, failures, close } = await openDefaults(browser, { isEnforced: true });
  t.after(close);

  const section = page.locator('[data-installation-presets]');
  const craft = section.getByRole('row').filter({ has: page.locator(`[data-installation-preset="${CRAFT.id}"]`) });
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
  const browser = await sharedChromium(t);
  const { page, read, failures, close } = await openDefaults(browser, { manifest: { ...MANIFEST, tenancy_mode: 'single' } });
  t.after(close);

  await page.waitForTimeout(600);
  assert.equal(await page.locator('[data-installation-presets]').count(), 0, 'the installation level is drawn in single mode');
  assert.ok(!read.includes('/admin/presets'), 'the installation presets were read in single mode');
  assert.deepEqual(failures, []);
});
