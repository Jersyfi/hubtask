// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Connecting a provider from a signed-in session (ADR-0078 §1, UC-ID-10 check 8), walked in the
// browser against a stubbed API: on *Password and sign-in* a person sees the provider they are
// connected to, chooses *Connect* for another, proves it is them, leaves for the provider - stood in
// for by a redirect straight back to the callback - and comes back to the screen with it listed. An
// account without an address is not offered the panel. No real provider is walked (milestone SC's
// Decision 9).
//
// Chromium only: what is asserted is behaviour and text, not engine-specific layout.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const GRANT = 'hbt_sup_connect';
const ENTRA = { id: '01936f2a-7c1e-7000-8000-0000000000a1', display_name: 'Contoso Entra ID', kind: 'MICROSOFT', scope: 'workspace' };
const GOOGLE = { id: '01936f2a-7c1e-7000-8000-0000000000a2', display_name: 'Google', kind: 'GOOGLE', scope: 'installation' };

const RULES = {
  workspace_host: 'acme.hubtask.eu', methods: ['PASSWORD', 'OIDC'], providers: [ENTRA, GOOGLE], legal: {},
  password: {
    min_length: 14, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
    breach_check: false, history_count: 0, not_current: false,
  },
};

async function open(browser, account) {
  const sent = { starts: [], completes: [], proofs: [] };
  const connected = [{ provider_id: ENTRA.id, provider_name: ENTRA.display_name, linked_at: '2026-10-01T09:00:00Z' }];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me') return route.fulfill({ json: { ...account, has_password: true, has_second_factor: false } });
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: RULES });
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, sign_in_rules: true } } });
    }
    if (path === '/accounts/me/identities' && request.method() === 'GET') return route.fulfill({ json: connected });
    if (path === '/accounts/me/identities:connect') {
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      sent.starts.push({ proof, body: request.postDataJSON() });
      if (proof !== GRANT) {
        return route.fulfill({
          status: 403,
          json: { code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_c', params: { methods: 'PASSWORD' } },
        });
      }
      // The provider, stood in for: it signs the person in again and sends the browser straight back.
      return route.fulfill({
        status: 201,
        json: {
          authorization_url: `${served.origin}/auth/callback?code=the-code&state=the-state`,
          expires_at: '2099-01-01T00:00:00Z', provider_id: GOOGLE.id, provider_name: GOOGLE.display_name,
        },
      });
    }
    if (path === '/auth/step-up') {
      sent.proofs.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { step_up_token: GRANT, expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    if (path === '/accounts/me/identities:complete') {
      sent.completes.push(request.postDataJSON());
      const joined = { provider_id: GOOGLE.id, provider_name: GOOGLE.display_name, linked_at: '2026-10-15T09:00:00Z' };
      connected.push(joined);
      return route.fulfill({ json: joined });
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

test('chromium: a signed-in person connects another provider and finds it among their ways in', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, ACCOUNT);
  t.after(close);

  await page.goto(`${served.origin}/profile/security`);
  const panel = page.getByTestId('sign-in-providers');
  await panel.waitFor({ timeout: 15_000 });
  assert.match(await panel.innerText(), /Contoso Entra ID - connected/);
  // Only what is not connected yet is offered.
  assert.equal(await panel.getByRole('button', { name: 'Connect Contoso Entra ID' }).count(), 0);

  // EVIDENCE=<dir> keeps what the person sees, for the owner's look (docs/evidence/).
  if (process.env.EVIDENCE) await panel.screenshot({ path: join(process.env.EVIDENCE, 'before.png') });
  await panel.getByRole('button', { name: 'Connect Google' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  await dialog.getByLabel('Your password').fill('the one in force');
  await dialog.getByRole('button', { name: 'Prove it' }).click();

  // Back on the security settings, with the provider listed.
  await page.getByText('Google - connected').waitFor({ timeout: 15_000 });
  assert.equal(new URL(page.url()).pathname, '/profile/security');
  assert.equal(new URL(page.url()).search, '', 'the code stayed in the address');
  assert.deepEqual(sent.proofs, [{ password: 'the one in force' }]);
  assert.deepEqual(sent.starts.map((each) => each.proof), [null, GRANT]);
  assert.deepEqual(sent.starts[1].body, { provider_id: GOOGLE.id });
  assert.deepEqual(sent.completes, [{ state: 'the-state', authorization_code: 'the-code' }]);
  assert.equal(await panel.getByRole('button', { name: /^Connect / }).count(), 0, 'a connected provider is still offered');
  if (process.env.EVIDENCE) await panel.screenshot({ path: join(process.env.EVIDENCE, 'after.png') });
});

// identity.md §4.1: an account without an address is one no provider reaches - nothing is offered.
test('chromium: an account without an address is offered no provider', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { ...ACCOUNT, email: null, sign_in_name: 'jerome' });
  t.after(close);

  await page.goto(`${served.origin}/profile/security`);
  await page.getByRole('heading', { name: 'Password' }).first().waitFor({ timeout: 15_000 });
  assert.equal(await page.getByTestId('sign-in-providers').count(), 0, 'the panel is drawn for an account without an address');
});
