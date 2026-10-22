// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A security key as the second factor (UC-ID-21 checks 5, 6 and 9; UC-ID-22 check 8), against a
// stubbed API with Chromium's virtual authenticator holding a key for `localhost` that verifies
// nobody - presence only, as a key without a PIN: the factor's step after the password, a prompt
// closed, the setup the rule demands during sign-in, and the fresh proof.
//
// Chromium only, and served at `localhost`, for the reasons `passkeysignin.test.mjs` gives.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { generateKeyPairSync, randomBytes } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { PASSKEYS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
const origin = served.origin.replace('127.0.0.1', 'localhost');

const CREDENTIAL = randomBytes(16);
const CHALLENGE = Buffer.alloc(32, 0x2a).toString('base64url');
const GRANT = 'hbt_sup_e2e';
const CODES = Array.from({ length: 10 }, (_, index) => `ABCD-EFGH-IJKL-MN${String(index).padStart(2, '0')}`);

const RULES = {
  workspace_host: 'localhost',
  methods: ['PASSWORD'],
  providers: [],
  password: {
    min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
    breach_check: false, history_count: 0, not_current: false,
  },
  legal: {},
};

const TOKENS = {
  token_type: 'Bearer', access_token: 'e2e-access', refresh_token: 'e2e-refresh',
  access_token_expires_at: '2099-01-01T00:00:00Z', refresh_token_expires_at: '2099-01-01T00:00:00Z',
  session: { id: 's1', created_at: '2026-10-17T00:00:00Z', current: true },
};

const KEY_OPTIONS = {
  expires_at: '2099-01-01T00:05:00Z',
  public_key: {
    rpId: 'localhost', challenge: CHALLENGE, timeout: 300000, userVerification: 'preferred',
    allowCredentials: [{ type: 'public-key', id: CREDENTIAL.toString('base64url'), transports: ['usb'] }],
  },
};

const FACTOR_STEP = {
  mfa_required: true, pending_token: 'hbt_pnd_e2e', expires_at: '2099-01-01T00:05:00Z',
  methods: ['PASSKEY', 'RECOVERY'], passkey_options: KEY_OPTIONS,
};

/** A key that holds a credential for `localhost` and verifies nobody, attached to the page. */
async function attachKey(context, page, { holding = true } = {}) {
  const cdp = await context.newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2', transport: 'usb', hasResidentKey: false,
      hasUserVerification: false, isUserVerified: false, automaticPresenceSimulation: true,
    },
  });
  if (!holding) return;
  const { privateKey } = generateKeyPairSync('ec', { namedCurve: 'P-256' });
  await cdp.send('WebAuthn.addCredential', {
    authenticatorId,
    credential: {
      credentialId: CREDENTIAL.toString('base64'), isResidentCredential: false, rpId: 'localhost',
      privateKey: privateKey.export({ format: 'der', type: 'pkcs8' }).toString('base64'), signCount: 4,
    },
  });
}

/** The card's reads and the sign-in's routes, with what each step sent. */
async function openCard(browser, { first = FACTOR_STEP, extra } = {}) {
  const sent = { verifies: [], enrollOptions: [], enrolls: [] };
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: RULES });
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { product_version: 'e2e', api_version: 'v1', tenancy_mode: 'single', item_types: [], view_layouts: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], roles: [], limits: {}, features: { sign_in_rules: true } } });
    }
    if (path === '/auth/sessions' && request.method() === 'POST') return route.fulfill({ status: 202, json: first });
    if (path === '/auth/sessions:verify') {
      sent.verifies.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: TOKENS });
    }
    if (extra && (await extra(route, path, sent))) return undefined;
    return route.fulfill({ status: 404, json: { code: 'errors.not_found' } });
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(origin);
  await page.waitForSelector('text=to localhost');
  return { context, page, sent, failures };
}

async function password(page) {
  await page.locator('input[type="email"]').fill('bert@example.org');
  await page.locator('input[type="password"]').fill('correct horse battery');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
}

// Check 5: a key without a PIN completes the step after the password - and only that.
test('chromium: a security key without a PIN completes the step after the password', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent, failures } = await openCard(browser);
  await attachKey(context, page);
  await password(page);
  const key = page.getByRole('button', { name: 'Use my security key' });
  await key.waitFor();
  assert.equal(await page.locator('input[inputmode="numeric"]').count(), 0, 'a code field without an app');
  await page.getByText('also asks for your security key or passkey').waitFor();
  await key.click();
  await page.waitForFunction(() => !document.querySelector('input[type="email"]') && !document.body.textContent?.includes('Use my security key'));

  assert.equal(sent.verifies.length, 1);
  const body = sent.verifies[0];
  assert.equal(body.pending_token, 'hbt_pnd_e2e');
  assert.equal(body.raw_id, CREDENTIAL.toString('base64url'));
  assert.equal('code' in body || 'user_handle' in body, false, 'one proof, and no handle');
  const clientData = JSON.parse(Buffer.from(body.client_data_json, 'base64url').toString('utf8'));
  assert.equal(clientData.type, 'webauthn.get');
  assert.equal(clientData.challenge, CHALLENGE);
  const flags = Buffer.from(body.authenticator_data, 'base64url')[32];
  assert.ok(flags & 0x01, 'the key proved presence');
  assert.equal(flags & 0x04, 0, 'the key verified nobody - and was taken as the factor all the same');
  assert.deepEqual(failures, []);
});

// Check 9 and P-11: reached and named, the wait said; a closed prompt says nothing, the focus kept.
test('chromium: the key is reached by keyboard, the wait said, and a closed prompt silent', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, failures } = await openCard(browser);
  await password(page);
  await page.getByRole('button', { name: 'Use my security key' }).waitFor();
  await page.evaluate(() => {
    navigator.credentials.get = () => new Promise(() => {});
  });
  let held;
  for (let presses = 0; presses < 12; presses++) {
    await page.keyboard.press('Tab');
    held = await page.evaluate(() => document.activeElement?.textContent?.trim());
    if (held === 'Use my security key') break;
  }
  assert.equal(held, 'Use my security key', 'the key is not reached by Tab');
  await page.keyboard.press('Enter');
  await page.getByRole('status').filter({ hasText: 'Touch your security key' }).waitFor();

  await page.evaluate(() => {
    navigator.credentials.get = () => Promise.reject(new DOMException('The operation was cancelled.', 'NotAllowedError'));
  });
  await page.reload();
  await page.waitForSelector('text=to localhost');
  await password(page);
  await page.getByRole('button', { name: 'Use my security key' }).click();
  await page.waitForFunction(() => document.activeElement?.textContent?.includes('Use my security key'));
  assert.equal(sent.verifies.length, 0);
  assert.equal(await page.locator('[role="alert"]').count(), 0, 'a closed prompt was reported as a failure');
  assert.deepEqual(failures, []);
});

// UC-ID-22 check 8: the setup the rule demands offers a key beside the app, and the codes come first.
test('chromium: the setup during sign-in adds a security key, shows the codes, then opens the session', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const registration = {
    expires_at: '2099-01-01T00:05:00Z',
    public_key: {
      rp: { id: 'localhost', name: 'Acme' },
      user: { id: Buffer.alloc(32, 0x44).toString('base64url'), name: 'bert@example.org', displayName: 'Bert' },
      challenge: CHALLENGE,
      pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
      timeout: 300000, excludeCredentials: [],
      authenticatorSelection: { residentKey: 'preferred', requireResidentKey: false, userVerification: 'preferred' },
      attestation: 'none',
    },
  };
  const { context, page, sent, failures } = await openCard(browser, {
    first: { mfa_required: true, pending_token: 'hbt_pnd_enroll', expires_at: '2099-01-01T00:05:00Z', methods: ['ENROLL'], passkey_setup: true },
    extra: async (route, path, record) => {
      if (path === '/auth/mfa/totp:enroll') {
        await route.fulfill({ json: { secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/Hubtask:bert?secret=JBSWY3DPEHPK3PXP', recovery_codes: CODES } });
        return true;
      }
      if (path === '/auth/mfa/passkey:enroll-options') {
        record.enrollOptions.push(route.request().postDataJSON());
        await route.fulfill({ json: registration });
        return true;
      }
      if (path === '/auth/mfa/passkey:enroll') {
        record.enrolls.push(route.request().postDataJSON());
        await route.fulfill({
          status: 201,
          json: {
            passkey: { id: '01936f2a-7c1e-7000-8000-0000000000e1', name: 'Passkey 1', created_at: '2026-10-17T09:00:00Z', last_used_at: null },
            recovery_codes: CODES, tokens: TOKENS,
          },
        });
        return true;
      }
      return false;
    },
  });
  await attachKey(context, page, { holding: false });
  await password(page);
  await page.getByText('a passkey or security key, or an authenticator app').waitFor();
  await page.getByRole('button', { name: 'Add a passkey or security key' }).click();
  await page.getByLabel('I have stored these codes. They will not be shown again.').check();
  assert.ok(await page.locator('input[type="email"]').count() === 0);
  await page.getByRole('button', { name: 'Continue' }).click();
  await page.waitForFunction(() => !document.body.textContent?.includes('Add a passkey or security key'));

  assert.deepEqual(sent.enrollOptions, [{ pending_token: 'hbt_pnd_enroll' }]);
  assert.equal(sent.enrolls.length, 1);
  const body = sent.enrolls[0];
  assert.equal(body.pending_token, 'hbt_pnd_enroll');
  assert.equal(body.name, 'Passkey 1');
  const clientData = JSON.parse(Buffer.from(body.client_data_json, 'base64url').toString('utf8'));
  assert.equal(clientData.type, 'webauthn.create');
  assert.equal(clientData.challenge, CHALLENGE);
  assert.deepEqual(failures, []);
});

// Check 6: the fresh proof offers the key among the account's ways, and the device answers it.
test('chromium: the step-up prompt proves the caller with a security key', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const sent = { removals: [], options: 0, proofs: [] };
  const held = [{ id: '01936f2a-7c1e-7000-8000-0000000000a0', name: 'Old laptop', created_at: '2026-10-01T09:00:00Z', last_used_at: null }];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me/passkeys') return route.fulfill({ json: { ...PASSKEYS, passkeys: held } });
    if (path.startsWith('/accounts/me/passkeys/') && request.method() === 'DELETE') {
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      sent.removals.push(proof);
      if (proof !== GRANT) {
        return route.fulfill({
          status: 403,
          json: { code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_k', params: { methods: 'PASSWORD PASSKEY' } },
        });
      }
      held.length = 0;
      return route.fulfill({ status: 204, body: '' });
    }
    if (path === '/auth/step-up:passkey-options') {
      sent.options += 1;
      return route.fulfill({ json: KEY_OPTIONS });
    }
    if (path === '/auth/step-up') {
      sent.proofs.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { step_up_token: GRANT, expires_at: '2099-01-01T00:00:00Z', method: 'PASSKEY' } });
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
  await attachKey(context, page);
  await page.goto(`${origin}/profile/security`);
  const panel = page.getByTestId('passkeys');
  await panel.getByRole('listitem').filter({ hasText: 'Old laptop' }).getByRole('button', { name: 'Remove' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: 'Use a passkey or security key' }).click();
  await page.locator('[data-tone="success"]', { hasText: 'Old laptop was removed. It no longer works.' }).waitFor();

  assert.equal(sent.options, 1);
  assert.equal(sent.proofs.length, 1);
  assert.equal(sent.proofs[0].raw_id, CREDENTIAL.toString('base64url'));
  assert.equal('password' in sent.proofs[0], false);
  assert.deepEqual(sent.removals, [null, GRANT]);
  assert.deepEqual(failures, []);
});
