// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Accounts without a mail address, walked (UC-ID-20 checks 1, 2 and 8): *People* offers *Add
// someone without an address* exactly where the workspace's rule allows it, the start password is
// shown once to copy or print and kept nowhere, and *Sign-in* settings draws the switch with its
// source and the reason it cannot be switched on under an "off" above.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { HUB, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const rule = (value, installation, { lock = null, source = 'DEFAULT', above = 'DEFAULT' } = {}) =>
  ({ value, installation, lock, source, installation_source: above });

function policy(managed) {
  return {
    password: {
      min_length: rule(12, 12), min_lowercase: rule(0, 0), min_uppercase: rule(0, 0), min_digits: rule(0, 0),
      min_symbols: rule(0, 0), min_classes: rule(0, 0), max_repeat: rule(0, 0),
      common_passwords: rule(true, true), context_words: rule(true, true), breach_check: rule(false, false),
      max_age_days: rule(0, 0), history_count: rule(0, 0), min_age_hours: rule(0, 0),
    },
    mfa_required_for: rule('NONE', 'NONE'),
    methods: rule(['PASSWORD'], ['PASSWORD']),
    session: { max_days: rule(30, 30), idle_minutes: rule(0, 0) },
    legal: { imprint_url: rule('', ''), privacy_url: rule('', ''), terms_url: rule('', ''), accessibility_url: rule('', '') },
    managed_accounts: managed,
    rotation_from: null,
  };
}

const WORKSPACE = {
  id: 'w1', slug: 'household', display_name: 'Household', status: 'ACTIVE', default_locale: 'en',
  default_time_zone: 'UTC', require_admin_totp: false, created_at: '2026-09-01T00:00:00Z', version: 3,
};

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, managed, sent) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy(managed) } });
    }
    if (path === '/accounts:create-managed') {
      sent.create = request.postDataJSON();
      return route.fulfill({
        status: 201,
        headers: { 'cache-control': 'no-store' },
        json: {
          account: { id: '01a0e2e0-0000-7000-8000-0000000000c1', kind: 'USER', sign_in_name: 'lena', display_name: 'Lena', status: 'ACTIVE' },
          membership: { id: 'm-lena', scope_type: 'HUB', scope_id: HUB.id, account_id: '01a0e2e0-0000-7000-8000-0000000000c1', role: 'CONTRIBUTOR' },
          start_password: 'aaaa-bbbb-cccc-dddd',
        },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, close: () => context.close() };
}

test('People offers an account without an address where the rule allows it, and shows the start password once', async () => {
  const browser = await chromium.launch();
  const sent = {};
  const { page, close } = await open(browser, rule(true, true), sent);
  try {
    await page.goto(`${served.origin}/administration/people`);
    await page.getByRole('heading', { name: 'Add someone without an address' }).waitFor();
    const form = page.locator('form').filter({ has: page.getByLabel('Sign-in name') });
    await form.getByLabel('Sign-in name').fill('Lena');
    await form.getByRole('textbox', { name: /^Name/ }).fill('Lena');
    // The place first: the roles offered are the ones the reader may give there.
    await form.getByLabel('Where').selectOption(HUB.id);
    await form.getByLabel('Role').locator('option[value="OWNER"]').waitFor({ state: 'attached' });
    await form.getByLabel('Role').selectOption('OWNER');
    await form.getByRole('button', { name: 'Add', exact: true }).click();

    await page.getByText('Start password for lena').waitFor();
    assert.deepEqual(sent.create, {
      display_name: 'Lena', sign_in_name: 'Lena', role: 'OWNER', scope_type: 'HUB', scope_id: HUB.id,
    });
    assert.ok(await page.getByRole('button', { name: 'Print' }).isVisible(), 'the start password cannot be printed');
    // Shown masked until asked for, and not on a slip: the slip exists only while printing.
    assert.equal(await page.locator('[data-slip]').count(), 0, 'the slip is in the document outside printing');
  } finally {
    await close();
    await browser.close();
  }
});

test('People draws no such form where the workspace switched managed accounts off', async () => {
  const browser = await chromium.launch();
  const { page, close } = await open(browser, rule(false, true, { source: 'WORKSPACE' }), {});
  try {
    await page.goto(`${served.origin}/administration/people`);
    await page.getByRole('heading', { name: 'Give somebody a role' }).waitFor();
    assert.equal(await page.getByRole('heading', { name: 'Add someone without an address' }).count(), 0);
  } finally {
    await close();
    await browser.close();
  }
});

test('Sign-in settings draws the switch, and says why it cannot be switched on under an off above', async () => {
  const browser = await chromium.launch();
  const { page, close } = await open(browser, rule(false, false, { lock: 'INSTANCE', source: 'INSTANCE', above: 'INSTANCE' }), {});
  try {
    await page.goto(`${served.origin}/administration/sign-in`);
    const control = page.getByRole('switch', { name: 'Accounts without a mail address allowed' });
    await control.waitFor();
    assert.equal(await control.isDisabled(), true, 'a switch the installation locked can be changed here');
    await page.getByText('Set by the installation', { exact: false }).first().waitFor();
  } finally {
    await close();
    await browser.close();
  }
});
