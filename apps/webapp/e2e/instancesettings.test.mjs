// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The installation's defaults, walked as an operator meets them (UC-INS-09 checks 1 to 4, 7): every
// key in words with the control of its kind, the lock worded by kind and absent for a language, a
// value that was undecided saved as its kind, a refusal at its row, and no control while a file
// enforces the level.
//
// Against a stubbed API: what is asserted is what the screen shows and what it sends. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const tighten = { workspace_may: 'TIGHTEN' };

/** One key of each kind, decided and undecided, as the server declares them. */
const LEVEL = {
  sign_in: {
    min_length: { set: true, value: 14, locked: true, kind: 'COUNT', minimum: 8, maximum: 128, ...tighten },
    min_digits: { set: false, locked: false, kind: 'COUNT', minimum: 0, ...tighten },
    breach_check: { set: false, locked: false, kind: 'FLAG', ...tighten },
    mfa_required_for: { set: true, value: 'ADMINS', locked: false, kind: 'CHOICE', options: ['NONE', 'ADMINS', 'EVERYONE'], ...tighten },
    methods: { set: false, locked: false, kind: 'CHOICES', options: ['PASSWORD', 'OIDC'], ...tighten },
  },
  legal: {
    accessibility_url: { set: false, locked: false, kind: 'LINK', workspace_may: 'REPLACE' },
    imprint_url: { set: true, value: 'https://provider.example/imprint', locked: true, kind: 'LINK', workspace_may: 'REPLACE' },
  },
  localisation: {
    locale: { set: true, value: 'de', locked: false, kind: 'LOCALE' },
    time_zone: { set: false, locked: false, kind: 'TIME_ZONE' },
    week_start: { set: false, locked: false, kind: 'CHOICE', options: ['MONDAY', 'SATURDAY', 'SUNDAY'] },
  },
  quotas: {
    items: { set: true, value: 0, locked: false, kind: 'COUNT', minimum: 0, workspace_may: 'RAISE' },
    export_jobs: { set: true, value: 3, locked: true, kind: 'COUNT', minimum: 0, workspace_may: 'RAISE' },
  },
  ai: {
    sources: { set: true, value: 'OFFERED', locked: true, kind: 'CHOICE', options: ['EITHER', 'OFFERED', 'OWN', 'NONE'], ...tighten },
  },
  source: 'database',
  is_enforced_from_file: false,
};

async function open(browser, { level = LEVEL, refuse = undefined, width = 1280 } = {}) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:elevate') {
      return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
    }
    if (path === '/admin/settings' && request.method() === 'GET') return route.fulfill({ json: level });
    if (path === '/admin/settings' && request.method() === 'PUT') {
      const body = request.postDataJSON();
      sent.push(body);
      if (refuse) return route.fulfill({ status: 422, contentType: 'application/problem+json', json: refuse });
      return route.fulfill({ json: level });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/instance/settings`);
  await page.getByRole('row').filter({ hasText: 'Minimum length' }).waitFor({ timeout: 15_000 });
  return { page, sent, close: () => context.close() };
}

test('chromium: every value is named in words, and the lock says what a workspace may still do', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser);
  t.after(close);

  const text = (await page.locator('main').textContent()) ?? '';
  // No key as the contract spells it, and no value as the contract spells it.
  assert.doesNotMatch(text, /sign_in\.|legal\.|localisation\.|quotas\.|ai\.sources|min_length|ADMINS|OFFERED/);

  const row = (name) => page.getByRole('row').filter({ hasText: name });
  assert.match(await row('Minimum length').textContent() ?? '', /14.*not change it/);
  assert.match(await row('Second factor').textContent() ?? '', /Owners and administrators.*make it stricter/i);
  assert.match(await row('Entries').textContent() ?? '', /Unlimited.*go higher/);
  assert.match(await row('Exports').textContent() ?? '', /3.*not go higher/);
  assert.match(await row('Imprint').textContent() ?? '', /provider\.example.*not change it/);
  assert.match(await row('Accessibility').textContent() ?? '', /Each workspace decides.*use their own/);
  // A language is a default and never locked: its row says nothing about what a workspace may do.
  const language = (await row('Language').textContent()) ?? '';
  assert.doesNotMatch(language, /stricter|change it|higher|their own/);
});

test('chromium: an undecided value is saved as its kind, every kind from its own control', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.getByRole('button', { name: 'Change these values' }).click();
  const form = page.locator('form');

  const digits = form.getByRole('spinbutton', { name: 'Digits, at least' });
  assert.equal(await digits.getAttribute('min'), '0');
  await digits.fill('2');
  await form.getByRole('combobox', { name: 'Check against known breaches' }).selectOption('true');
  await form.getByRole('checkbox', { name: 'Password' }).check();
  await form.getByRole('combobox', { name: 'The week starts on' }).selectOption('SUNDAY');
  await form.getByRole('textbox', { name: 'Accessibility' }).fill('https://provider.example/a11y');
  // The statement's lock: a link is replaced by a workspace's own, or not.
  const locks = form.getByRole('combobox', { name: 'Workspaces may' });
  assert.ok(await locks.count() > 0);
  // No lock for a language, a zone or a week start.
  assert.equal(await form.locator('[data-value="localisation.week_start"]').getByRole('combobox', { name: 'Workspaces may' }).count(), 0);

  await form.getByRole('button', { name: 'Save' }).click();
  await page.getByText('Saved.').waitFor({ timeout: 10_000 });

  const body = sent.at(-1);
  assert.deepEqual(body.sign_in.min_digits, { set: true, value: 2, locked: false });
  assert.equal(typeof body.sign_in.min_length.value, 'number');
  assert.equal(body.sign_in.breach_check.value, true);
  assert.deepEqual(body.sign_in.methods.value, ['PASSWORD']);
  assert.equal(body.localisation.week_start.value, 'SUNDAY');
  assert.equal(body.legal.accessibility_url.value, 'https://provider.example/a11y');
  assert.equal(body.localisation.time_zone, undefined, 'an undecided key is not sent');
  // Nothing read-only goes back.
  assert.equal(body.sign_in.min_length.kind, undefined);
});

// The terms' version sits beside its link, and its lock - which is the link's - reads the link's
// while the form is open, not the one the server last answered.
test("chromium: the terms' version follows its link, in place and in its lock", async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const replace = { kind: 'LINK', workspace_may: 'REPLACE' };
  const level = {
    ...LEVEL,
    legal: {
      imprint_url: { set: false, locked: false, ...replace, position: 0 },
      privacy_url: { set: false, locked: false, ...replace, position: 1 },
      terms_url: { set: true, value: 'https://provider.example/terms', locked: false, ...replace, position: 2 },
      terms_version: { set: true, value: '3', locked: false, kind: 'TEXT', workspace_may: 'REPLACE', position: 3 },
      accessibility_url: { set: false, locked: false, ...replace, position: 4 },
    },
  };
  const { page, close } = await open(browser, { level });
  t.after(close);

  await page.getByRole('button', { name: 'Change these values' }).click();
  const form = page.locator('form');
  const order = await form.locator('[data-value^="legal."]').evaluateAll(
    (rows) => rows.map((row) => row.getAttribute('data-value')),
  );
  assert.deepEqual(order, ['legal.imprint_url', 'legal.privacy_url', 'legal.terms_url', 'legal.terms_version', 'legal.accessibility_url']);

  const lockOf = (key) => form.locator(`[data-value="legal.${key}"]`).getByRole('combobox', { name: 'Workspaces may' });
  await lockOf('terms_url').selectOption('locked');
  assert.equal(await lockOf('terms_version').inputValue(), 'locked');
  assert.equal(await lockOf('terms_version').locator('option:checked').textContent(), 'not change it');
  await lockOf('terms_url').selectOption('open');
  assert.equal(await lockOf('terms_version').inputValue(), 'open');
});

test('chromium: a refusal lands at the value it is about', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, {
    refuse: {
      type: 'about:blank', title: 'validation_failed', status: 422, code: 'validation_failed',
      detail_code: 'auth.policy_value_invalid', params: { switch: 'quotas.items' },
      field_errors: [{ path: '/quotas/items', code: 'auth.policy_value_invalid' }],
    },
  });
  t.after(close);

  await page.getByRole('button', { name: 'Change these values' }).click();
  await page.locator('form').getByRole('button', { name: 'Save' }).click();
  const field = page.locator('[data-value="quotas.items"]');
  await field.getByRole('alert').or(field.locator('[aria-invalid="true"]')).first().waitFor({ timeout: 10_000 });
});

test('chromium: a level a file enforces is shown with its path and no control', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, {
    level: { ...LEVEL, is_enforced_from_file: true, source: '/etc/hubtask/instance.json' },
  });
  t.after(close);

  await page.getByText('/etc/hubtask/instance.json').waitFor({ timeout: 10_000 });
  assert.equal(await page.getByRole('button', { name: 'Change these values' }).count(), 0);
});

test('chromium: at 375 px the form keeps every value and its lock inside the screen', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { width: 375 });
  t.after(close);

  await page.getByRole('button', { name: 'Change these values' }).click();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(overflow <= 0, `the page scrolls sideways by ${overflow}px`);
});
