// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Adding a passkey on *Password and sign-in* (UC-ID-22 checks 1, 2, 3 and 6), walked in the browser
// against a stubbed API with Chromium's virtual authenticator standing in for the device: the list,
// the fresh proof, the device, the name, and the ten codes once; ten reached; a prompt closed; a
// challenge run out.
//
// Chromium only: the virtual authenticator is a DevTools protocol of Chromium's own. The bundle is
// opened at `localhost`, because a browser makes no passkey for an address - an IP is no relying
// party - and the stubs name `localhost` as this workspace's host.
//
// With CAPTURE set to a file, the walk writes the ceremony the browser produced there: the server's
// verifier replays it in its own tests (infrastructure/webauthn), so a real browser's answer is held
// to the real checks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, PASSKEYS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
const origin = served.origin.replace('127.0.0.1', 'localhost');

const GRANT = 'hbt_sup_e2e';
const CHALLENGE = Buffer.alloc(32, 0x2a).toString('base64url');
const HANDLE = Buffer.alloc(32, 0x33).toString('base64url');
const CODES = Array.from({ length: 10 }, (_, index) => `ABCD-EFGH-IJKL-MN${String(index).padStart(2, '0')}`);

const OPTIONS = {
  expires_at: '2099-01-01T00:05:00Z',
  public_key: {
    rp: { id: 'localhost', name: 'Acme' },
    user: { id: HANDLE, name: ACCOUNT.email, displayName: ACCOUNT.display_name },
    challenge: CHALLENGE,
    pubKeyCredParams: [{ type: 'public-key', alg: -7 }, { type: 'public-key', alg: -8 }, { type: 'public-key', alg: -257 }],
    timeout: 300000,
    excludeCredentials: [],
    authenticatorSelection: { residentKey: 'preferred', requireResidentKey: false, userVerification: 'preferred' },
    attestation: 'none',
  },
};

/**
 * A signed-in tab on *Password and sign-in* with the passkey routes stood in for, a virtual
 * authenticator attached, and a record of what each step sent.
 */
async function open(browser, { list = PASSKEYS, complete } = {}) {
  const sent = { starts: [], proofs: [], completes: [] };
  let held = [...list.passkeys];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me/passkeys') {
      return route.fulfill({ json: { ...list, passkeys: held } });
    }
    if (path === '/accounts/me/passkeys:start') {
      const proof = request.headers()['x-hubtask-step-up'];
      sent.starts.push(proof ?? null);
      if (proof !== GRANT) {
        return route.fulfill({
          status: 403,
          json: {
            code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_p',
            params: { methods: 'PASSWORD' },
          },
        });
      }
      return route.fulfill({ json: OPTIONS });
    }
    if (path === '/auth/step-up') {
      sent.proofs.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { step_up_token: GRANT, expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    if (path === '/accounts/me/passkeys:complete') {
      const body = request.postDataJSON();
      sent.completes.push(body);
      if (complete) return complete(route);
      const added = {
        id: '01936f2a-7c1e-7000-8000-0000000000a1', name: body.name,
        created_at: '2026-10-16T09:00:00Z', last_used_at: null,
      };
      held = [...held, added];
      return route.fulfill({ status: 201, json: { passkey: added, recovery_codes: CODES } });
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
  const cdp = await context.newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2', transport: 'internal', hasResidentKey: true,
      hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
    },
  });
  await page.goto(`${origin}/profile/security`);
  await page.getByTestId('passkeys').waitFor();
  return { context, page, sent, failures };
}

test('chromium: a passkey is added with a fresh proof, then the device, named, and the codes shown once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent, failures } = await open(browser);
  t.after(() => context.close());
  const panel = page.getByTestId('passkeys');

  // Check 1: none yet, in words.
  await panel.getByText('No passkey or security key yet.').waitFor();

  // Check 2: the name first, suggested and replaced; then the proof; then the device.
  const name = panel.getByRole('textbox', { name: 'Name' });
  assert.equal(await name.inputValue(), 'Passkey 1');
  await name.fill('Laptop at work');
  await panel.getByRole('button', { name: 'Add a passkey' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Your password').fill('the one in force');
  await dialog.getByRole('button', { name: 'Prove it' }).click();

  await panel.getByText('Laptop at work was added.').waitFor();
  assert.deepEqual(sent.starts, [null, GRANT], 'the start was asked without a proof, then with the one given');
  assert.deepEqual(sent.proofs, [{ password: 'the one in force' }]);
  assert.equal(sent.completes.length, 1);
  const body = sent.completes[0];
  assert.equal(body.name, 'Laptop at work');
  const clientData = JSON.parse(Buffer.from(body.client_data_json, 'base64url').toString('utf8'));
  assert.equal(clientData.type, 'webauthn.create');
  assert.equal(clientData.challenge, CHALLENGE, 'the device signed the challenge the server issued');
  assert.equal(clientData.origin, origin);
  assert.ok(body.raw_id.length > 0 && body.attestation_object.length > 0);

  // Check 6: the ten codes, once, closed only once stored.
  await panel.getByRole('button', { name: 'Show the codes', exact: false }).click().catch(() => undefined);
  await panel.getByLabel('I have stored these codes. They will not be shown again.').check();
  await panel.getByRole('button', { name: 'Done' }).click();
  assert.equal(await page.getByText(CODES[0]).count(), 0, 'the codes outlived the panel');

  // The list reads it back, by the name given.
  await panel.getByText('Laptop at work', { exact: true }).waitFor();
  await panel.getByText('not used yet', { exact: false }).waitFor();

  if (process.env.CAPTURE) {
    await writeFile(process.env.CAPTURE, `${JSON.stringify({ origin, rp_id: 'localhost', challenge: CHALLENGE, ...body }, null, 2)}\n`);
  }
  assert.deepEqual(failures, []);
});

// Check 3: at ten, no button, and the reason in its place.
test('chromium: at ten passkeys none is offered, and the profile says why', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const ten = Array.from({ length: 10 }, (_, index) => ({
    id: `01936f2a-7c1e-7000-8000-0000000000${String(index).padStart(2, '0')}`, name: `Key ${index + 1}`,
    created_at: '2026-10-01T09:00:00Z', last_used_at: index === 0 ? '2026-10-15T09:00:00Z' : null,
  }));
  const { context, page, failures } = await open(browser, {
    list: { passkeys: ten, limit: 10, can_add: false, cannot_add_reason: 'auth.passkey_limit_reached', rp_id: 'localhost' },
  });
  t.after(() => context.close());
  const panel = page.getByTestId('passkeys');
  await panel.getByText('An account holds at most 10 passkeys and security keys.', { exact: false }).waitFor();
  assert.equal(await panel.getByRole('button', { name: 'Add a passkey' }).count(), 0);
  assert.equal(await panel.getByRole('listitem').count(), 10);
  await panel.getByText('last used', { exact: false }).waitFor();
  assert.deepEqual(failures, []);
});

// P-11: a device prompt the person closes says nothing, and focus stays on the button.
test('chromium: a closed device prompt shows nothing and leaves focus on Add a passkey', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent, failures } = await open(browser);
  t.after(() => context.close());
  await page.evaluate(() => {
    navigator.credentials.create = () => Promise.reject(new DOMException('The operation was cancelled.', 'NotAllowedError'));
  });
  const panel = page.getByTestId('passkeys');
  await panel.getByRole('button', { name: 'Add a passkey' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Your password').fill('the one in force');
  await dialog.getByRole('button', { name: 'Prove it' }).click();
  await page.waitForFunction(() => document.activeElement?.textContent?.includes('Add a passkey'));
  assert.equal(sent.completes.length, 0);
  assert.equal(await panel.getByRole('alert').count(), 0, 'a cancelled prompt was reported as a failure');
  assert.equal(await panel.locator('.quiet', { hasText: 'was added' }).count(), 0);
  assert.deepEqual(failures, []);
});

// D14: a challenge that ran out is said, and the session survives - a 409, never a 401.
test('chromium: a challenge that ran out is refused with its sentence and the person stays signed in', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await open(browser, {
    complete: (route) => route.fulfill({
      status: 409,
      json: { code: 'conflict', detail_code: 'auth.passkey_challenge_stale', status: 409, request_id: 'req_stale' },
    }),
  });
  t.after(() => context.close());
  const panel = page.getByTestId('passkeys');
  await panel.getByRole('button', { name: 'Add a passkey' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Your password').fill('the one in force');
  await dialog.getByRole('button', { name: 'Prove it' }).click();
  await panel.getByText('That attempt to add a passkey has run out', { exact: false }).waitFor();
  assert.equal(new URL(page.url()).pathname, '/profile/security', 'the person was sent away');
  assert.ok(await page.getByRole('navigation', { name: 'Your settings', exact: true }).isVisible(), 'the frame is gone - signed out');
  assert.deepEqual(failures, []);
});
