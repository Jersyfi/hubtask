// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The step-up with whatever the account holds (ADR-0075, SC-16), walked in the browser against a
// stubbed API: a provider-only administrator with no second factor changes a sign-in rule by
// confirming at their provider - the browser leaves for it and comes back - and an account that
// holds every way is offered every way.
//
// Chromium only: what is asserted is behaviour and text, not engine-specific layout.

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

const GRANT = 'hbt_sup_e2e';

const rule = (value, installation) => ({ value, installation, lock: null, source: 'DEFAULT', installation_source: 'DEFAULT' });
function policy() {
  return {
    password: {
      min_length: rule(12, 12), min_lowercase: rule(0, 0), min_uppercase: rule(0, 0), min_digits: rule(0, 0),
      min_symbols: rule(0, 0), min_classes: rule(0, 0), max_repeat: rule(0, 0),
      common_passwords: rule(true, true), context_words: rule(true, true), breach_check: rule(false, false),
      max_age_days: rule(0, 0), history_count: rule(0, 0), min_age_hours: rule(0, 0),
    },
    mfa_required_for: rule('NOBODY', 'NOBODY'),
    methods: rule(['PASSWORD', 'OIDC'], ['PASSWORD', 'OIDC']),
    session: { max_days: rule(30, 30), idle_minutes: rule(0, 0) },
    legal: { imprint_url: rule('', ''), privacy_url: rule('', ''), terms_url: rule('', ''), accessibility_url: rule('', '') },
    rotation_from: null,
  };
}
const WORKSPACE = {
  id: 'w1', slug: 'acme', display_name: 'Acme', status: 'ACTIVE', default_locale: 'en',
  default_time_zone: 'Europe/Berlin', require_admin_totp: false, created_at: '2026-09-01T00:00:00Z', version: 3,
};

/** A workspace whose rule changes only with a proof, and a record of what each step sent. */
async function open(browser, methods) {
  const sent = { patches: [], proofs: [], starts: 0 };
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/tenant' && request.method() === 'PATCH') {
      const proof = request.headers()['x-hubtask-step-up'];
      sent.patches.push(proof ?? null);
      if (proof !== GRANT) {
        return route.fulfill({
          status: 403,
          json: {
            code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_s',
            params: { methods, provider: 'Contoso Entra ID' },
          },
        });
      }
      return route.fulfill({ json: { ...WORKSPACE, version: 4, sign_in_policy: policy() } });
    }
    if (path === '/tenant') return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy() } });
    if (path === '/auth/step-up:provider') {
      sent.starts += 1;
      // The provider, stood in for: it signs the person in again and sends the browser straight
      // back to this installation's callback with a code and the state.
      return route.fulfill({
        status: 201,
        json: {
          authorization_url: `${served.origin}/auth/callback?code=the-code&state=the-state`,
          expires_at: '2099-01-01T00:00:00Z', provider_id: '01936f2a-7c1e-7000-8000-0000000000a1',
          provider_name: 'Contoso Entra ID',
        },
      });
    }
    if (path === '/auth/step-up') {
      sent.proofs.push(request.postDataJSON());
      return route.fulfill({
        status: 201,
        json: { step_up_token: GRANT, expires_at: '2099-01-01T00:00:00Z', method: 'PROVIDER' },
      });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

async function changeARule(page) {
  const field = page.getByLabel('End a session after, in days', { exact: true });
  await field.waitFor({ timeout: 15_000 });
  await field.fill('20');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
}

// UC-ID-05 check 5, UC-ID-12: the provider-only administrator without a factor - the account D4 to
// D6 produce - changes a sign-in rule. Before SC-16 the dialog had nothing to offer them.
test('chromium: a provider-only administrator confirms at the provider and the change goes through', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, 'PROVIDER');
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  await changeARule(page);

  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  const text = await dialog.innerText();
  assert.match(text, /You sign in at Contoso Entra ID once more/, text);
  assert.equal(await dialog.locator('input').count(), 0, 'a provider-only account is asked to type something');

  await dialog.getByRole('button', { name: 'Confirm with Contoso Entra ID' }).click();

  // Back where they were, told the confirmation holds.
  await page.getByText('Confirmed with Contoso Entra ID.').waitFor({ timeout: 15_000 });
  assert.equal(new URL(page.url()).pathname, '/administration/sign-in');
  assert.equal(sent.starts, 1);
  assert.deepEqual(sent.proofs, [{ state: 'the-state', authorization_code: 'the-code' }]);
  assert.equal(new URL(page.url()).search, '', 'the code stayed in the address');

  // The same change again: refused once more, answered with the held proof rather than a second
  // dialog, and the note is gone.
  await changeARule(page);
  await page.waitForFunction(() => !document.body.innerText.includes('Confirmed with Contoso Entra ID.'));
  for (let tries = 0; tries < 50 && sent.patches.length < 3; tries += 1) await page.waitForTimeout(100);
  assert.deepEqual(sent.patches, [null, null, GRANT]);
  assert.equal(await page.getByRole('dialog').count(), 0, 'the dialog asked again although the proof was held');
});

// ADR-0075 §1: an account that holds every way is offered every way, one field at a time.
test('chromium: the dialog offers every way the account holds, and a recovery code proves it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, 'PASSWORD TOTP RECOVERY PROVIDER');
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  await changeARule(page);
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();

  // The code first, and the other three beside it.
  assert.ok(await dialog.getByLabel('Code from your authenticator').isVisible());
  for (const other of ['Use your password', 'Use a recovery code', 'Confirm with Contoso Entra ID']) {
    assert.ok(await dialog.getByRole('button', { name: other }).isVisible(), `${other} is not offered`);
  }

  await dialog.getByRole('button', { name: 'Use a recovery code' }).click();
  await dialog.getByLabel('A recovery code').fill('abcd-efgh');
  await dialog.getByRole('button', { name: 'Prove it' }).click();

  await page.waitForFunction(() => document.querySelector('[role="dialog"]') === null);
  // The retry follows the closing; wait for it rather than for a moment.
  for (let tries = 0; tries < 50 && sent.patches.length < 2; tries += 1) await page.waitForTimeout(100);
  assert.deepEqual(sent.proofs, [{ recovery_code: 'abcd-efgh' }]);
  assert.deepEqual(sent.patches, [null, GRANT]);
});
