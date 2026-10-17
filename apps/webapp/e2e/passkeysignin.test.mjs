// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Signing in with a passkey from the card (UC-ID-21 checks 1, 2, 4 and 9), against a stubbed API with
// Chromium's virtual authenticator holding a real passkey for `localhost`: the button asks the device
// and the server is sent a real assertion; the address field offers passkeys once it has focus; a
// closed prompt says nothing; a refusal is the wrong password's sentence; no passkey, no button.
//
// Chromium only: the virtual authenticator is a DevTools protocol of Chromium's own. Served at
// `localhost`, because a browser makes no ceremony for an address.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { generateKeyPairSync, randomBytes } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
const origin = served.origin.replace('127.0.0.1', 'localhost');

const CREDENTIAL = randomBytes(16);
const HANDLE = Buffer.alloc(32, 0x33);
const CHALLENGE = Buffer.alloc(32, 0x2a).toString('base64url');

const RULES = {
  workspace_host: 'localhost',
  methods: ['PASSWORD', 'PASSKEY'],
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

/** The card's reads, the passkey's two routes, and a record of what each step sent. */
async function open(browser, { rules = RULES, complete } = {}) {
  const sent = { options: 0, completes: [] };
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: rules });
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { product_version: 'e2e', api_version: 'v1', tenancy_mode: 'single', item_types: [], view_layouts: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], roles: [], limits: {}, features: { sign_in_rules: true } } });
    }
    if (path === '/auth/sessions:passkey-options') {
      sent.options += 1;
      return route.fulfill({
        json: {
          public_key: { rpId: 'localhost', challenge: CHALLENGE, timeout: 300000, userVerification: 'required' },
          expires_at: '2099-01-01T00:05:00Z',
        },
      });
    }
    if (path === '/auth/sessions:passkey') {
      sent.completes.push(request.postDataJSON());
      if (complete) return complete(route);
      return route.fulfill({ status: 201, json: TOKENS });
    }
    return route.fulfill({ status: 404, json: { code: 'errors.not_found' } });
  });
  // Every request to the device, as the page made it: which mediation, and nothing else.
  await context.addInitScript(() => {
    const original = navigator.credentials.get.bind(navigator.credentials);
    window.__mediations = [];
    navigator.credentials.get = (options) => {
      window.__mediations.push(options?.mediation ?? '');
      return original(options);
    };
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  const cdp = await context.newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2', transport: 'internal', hasResidentKey: true,
      hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
    },
  });
  const { privateKey } = generateKeyPairSync('ec', { namedCurve: 'P-256' });
  await cdp.send('WebAuthn.addCredential', {
    authenticatorId,
    credential: {
      credentialId: CREDENTIAL.toString('base64'), isResidentCredential: true, rpId: 'localhost',
      privateKey: privateKey.export({ format: 'der', type: 'pkcs8' }).toString('base64'),
      userHandle: HANDLE.toString('base64'), signCount: 7,
    },
  });
  await page.goto(origin);
  await page.waitForSelector('text=to localhost');
  return { context, page, sent, failures };
}

test('chromium: Sign in with a passkey asks the device and sends the server its assertion', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, failures } = await open(browser);
  const button = page.getByRole('button', { name: 'Sign in with a passkey' });
  await button.click();
  await page.waitForFunction(() => !document.querySelector('input[type="email"]'));

  assert.equal(sent.completes.length, 1);
  const body = sent.completes[0];
  assert.equal(body.raw_id, CREDENTIAL.toString('base64url'), 'the device named the passkey it holds');
  assert.equal(body.user_handle, HANDLE.toString('base64url'), 'a passkey signing in names its account by handle');
  const clientData = JSON.parse(Buffer.from(body.client_data_json, 'base64url').toString('utf8'));
  assert.equal(clientData.type, 'webauthn.get');
  assert.equal(clientData.challenge, CHALLENGE);
  assert.equal(clientData.origin, origin);
  // The device verified the person: the UV flag of the authenticator data (identity.md §23.1).
  const flags = Buffer.from(body.authenticator_data, 'base64url')[32];
  assert.ok(flags & 0x04, 'the assertion carries no user verification');
  assert.ok(await page.evaluate(() => window.__mediations.includes('optional')));
  assert.deepEqual(failures, []);
});

// Check 1: the address field offers this workspace's passkeys once it has focus, with options fetched
// then - not at page load.
test('chromium: the address field offers passkeys when it gains focus, and not before', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, failures } = await open(browser);
  assert.equal(sent.options, 0, 'options were fetched before anybody asked');
  await page.locator('input[type="email"]').focus();
  await page.waitForFunction(() => window.__mediations.includes('conditional'));
  assert.equal(sent.options, 1);
  assert.deepEqual(failures, []);
});

// Check 4, P-11: a prompt the person closes says nothing and leaves focus on the button.
test('chromium: a closed device prompt says nothing and leaves focus on the button', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, failures } = await open(browser);
  await page.evaluate(() => {
    navigator.credentials.get = () => Promise.reject(new DOMException('The operation was cancelled.', 'NotAllowedError'));
  });
  await page.getByRole('button', { name: 'Sign in with a passkey' }).click();
  await page.waitForFunction(() => document.activeElement?.textContent?.includes('Sign in with a passkey'));
  assert.equal(sent.completes.length, 0);
  assert.equal(await page.locator('[role="alert"]').count(), 0, 'a closed prompt was reported as a failure');
  assert.deepEqual(failures, []);
});

// Check 4: a refused passkey reads as a wrong password does.
test('chromium: a refused passkey is the wrong password’s sentence', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures } = await open(browser, {
    complete: (route) => route.fulfill({
      status: 401,
      json: { code: 'errors.unauthenticated', detail_code: 'auth.sign_in_failed', status: 401, request_id: 'req_pk' },
    }),
  });
  await page.getByRole('button', { name: 'Sign in with a passkey' }).click();
  await page.locator('[role="alert"]').getByText('Sign-in failed. Check the email address and the password.').waitFor();
  assert.deepEqual(failures, []);
});

// Check 9: the button is reached from the keyboard and named, and the wait is said once.
test('chromium: the passkey button is reached by keyboard and the wait is said', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures } = await open(browser);
  let held;
  await page.evaluate(() => {
    navigator.credentials.get = () => new Promise(() => {});
  });
  for (let presses = 0; presses < 12; presses++) {
    await page.keyboard.press('Tab');
    held = await page.evaluate(() => document.activeElement?.textContent?.trim());
    if (held === 'Sign in with a passkey') break;
  }
  assert.equal(held, 'Sign in with a passkey', 'the button is not reached by Tab');
  await page.keyboard.press('Enter');
  await page.getByRole('status').filter({ hasText: 'Confirm on your device' }).waitFor();
  assert.deepEqual(failures, []);
});

// Check 1: where the rules name no passkey, the card offers none.
test('chromium: no passkey in the rules, no passkey button', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, failures } = await open(browser, { rules: { ...RULES, methods: ['PASSWORD'] } });
  assert.equal(await page.getByRole('button', { name: 'Sign in with a passkey' }).count(), 0);
  await page.locator('input[type="email"]').focus();
  await page.waitForTimeout(300);
  assert.equal(sent.options, 0, 'options were fetched where passkeys are no way in');
  assert.deepEqual(failures, []);
});
