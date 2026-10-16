// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Passkeys on *Password and sign-in* (UC-ID-22 checks 1 to 6), walked in the browser against a
// stubbed API with Chromium's virtual authenticator standing in for the device: the list, the fresh
// proof, the device, the name, and the ten codes once; ten reached; a prompt closed; a challenge run
// out; renaming and removing; the last factor a workspace requires; a key beside the app.
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
async function open(browser, { list = PASSKEYS, complete, account, onRemoved, afterDisable } = {}) {
  const sent = { starts: [], proofs: [], completes: [], renames: [], removals: [], disables: [] };
  let held = [...list.passkeys];
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me/passkeys') {
      return route.fulfill({ json: { ...list, passkeys: held } });
    }
    if (path === '/accounts/me' && account) {
      return route.fulfill({ json: { ...ACCOUNT, ...account } });
    }
    if (path.startsWith('/accounts/me/passkeys/')) {
      const id = decodeURIComponent(path.slice('/accounts/me/passkeys/'.length));
      if (request.method() === 'PATCH') {
        const { name } = request.postDataJSON();
        sent.renames.push({ id, name });
        held = held.map((each) => (each.id === id ? { ...each, name } : each));
        return route.fulfill({ json: held.find((each) => each.id === id) });
      }
      if (request.method() === 'DELETE') {
        const proof = request.headers()['x-hubtask-step-up'];
        sent.removals.push({ id, proof: proof ?? null });
        if (proof !== GRANT) {
          return route.fulfill({
            status: 403,
            json: {
              code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_r',
              params: { methods: 'PASSWORD' },
            },
          });
        }
        held = held.filter((each) => each.id !== id);
        onRemoved?.(list);
        return route.fulfill({ status: 204, body: '' });
      }
    }
    if (path === '/auth/mfa:disable') {
      sent.disables.push(request.headers()['x-hubtask-step-up'] ?? null);
      if (request.headers()['x-hubtask-step-up'] !== GRANT) {
        return route.fulfill({
          status: 403,
          json: {
            code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_d',
            params: { methods: 'PASSWORD' },
          },
        });
      }
      afterDisable?.(list, account);
      return route.fulfill({ status: 204, body: '' });
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
  // Check 3: the limit is said before it is reached.
  await panel.getByText('An account holds up to 10.').waitFor();

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
  assert.equal(await panel.getByText('of at most', { exact: false }).count(), 0, 'the count stood beside the reason');
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

const LAPTOP = { id: '01936f2a-7c1e-7000-8000-0000000000b1', name: 'Laptop', created_at: '2026-02-01T09:00:00Z', last_used_at: '2026-06-01T09:00:00Z', rp_id: 'localhost' };
const DRAWER = { id: '01936f2a-7c1e-7000-8000-0000000000b2', name: 'Key in the drawer', created_at: '2026-03-01T09:00:00Z', last_used_at: null, rp_id: 'localhost' };

async function proveWithPassword(page) {
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('Your password').fill('the one in force');
  await dialog.getByRole('button', { name: 'Prove it' }).click();
}

// Check 2 and 4: a name changed with the session alone, and a passkey removed behind a fresh proof,
// gone from the list at once.
test('chromium: a passkey is renamed with the session alone, and another removed behind a fresh proof', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, sent, failures } = await open(browser, {
    list: { passkeys: [LAPTOP, DRAWER], limit: 10, can_add: true, can_remove: true, rp_id: 'localhost' },
  });
  t.after(() => context.close());
  const panel = page.getByTestId('passkeys');
  const laptop = panel.getByRole('listitem').filter({ hasText: 'Laptop' });
  await panel.getByText('You hold 2 of at most 10.').waitFor();

  await laptop.getByRole('button', { name: 'Rename' }).click();
  const field = panel.getByRole('textbox', { name: 'New name for Laptop' });
  assert.equal(await field.inputValue(), 'Laptop');
  await field.fill('Old laptop, at home');
  await panel.getByRole('button', { name: 'Save name' }).click();
  await panel.getByText('Renamed to Old laptop, at home.').waitFor();
  assert.deepEqual(sent.renames, [{ id: LAPTOP.id, name: 'Old laptop, at home' }]);
  assert.deepEqual(sent.proofs, [], 'renaming asked for a fresh proof');
  await panel.getByText('Old laptop, at home', { exact: true }).waitFor();

  const drawer = panel.getByRole('listitem').filter({ hasText: 'Key in the drawer' });
  await drawer.getByRole('button', { name: 'Remove' }).click();
  await page.getByRole('dialog').waitFor();
  // Only the row asked about waits for the proof; the others and *Add a passkey* stay as they were.
  const waiting = await panel.locator('[aria-busy="true"]').count();
  assert.ok(waiting <= 1, `${waiting} controls show they are working`);
  assert.equal(await panel.getByRole('button', { name: 'Add a passkey' }).getAttribute('aria-busy'), null);
  await proveWithPassword(page);
  await panel.getByText('Key in the drawer was removed. It no longer works.').waitFor();
  assert.deepEqual(sent.removals, [{ id: DRAWER.id, proof: null }, { id: DRAWER.id, proof: GRANT }]);
  assert.equal(await panel.getByRole('listitem').count(), 1, 'the removed passkey is still listed');
  await panel.getByText('You hold 1 of at most 10.').waitFor();
  assert.deepEqual(failures, []);
});

// Check 5: where the workspace requires a second factor and this is the last one, no *Remove* - the
// sentence stands in its place; a passkey of another address is no factor and goes.
test('chromium: the last factor the workspace requires is not offered for removal, and the profile says why', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const stale = { ...DRAWER, rp_id: 'old.example' };
  const { context, page, failures } = await open(browser, {
    list: {
      passkeys: [LAPTOP, stale], limit: 10, can_add: true, rp_id: 'localhost',
      can_remove: false, cannot_remove_reason: 'auth.mfa_required_by_tenant',
    },
  });
  t.after(() => context.close());
  const panel = page.getByTestId('passkeys');
  const laptop = panel.getByRole('listitem').filter({ hasText: 'Laptop' });
  await laptop.getByText('Your workspace requires a second factor, and this is the last one', { exact: false }).waitFor();
  assert.equal(await laptop.getByRole('button', { name: 'Remove' }).count(), 0);
  assert.equal(await laptop.getByRole('button', { name: 'Rename' }).count(), 1, 'renaming is not the rule\'s business');
  const old = panel.getByRole('listitem').filter({ hasText: 'Key in the drawer' });
  await old.getByText('Made for old.example.', { exact: false }).waitFor();
  assert.equal(await old.getByRole('button', { name: 'Remove' }).count(), 1);
  assert.deepEqual(failures, []);
});

// A key alone: the codes are the key's, counted and renewable; the app is offered to set up, and the
// profile does not say there is no second factor.
test('chromium: with a key alone, the recovery codes are counted and renewable, and the app can still be set up', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await open(browser, {
    list: { passkeys: [LAPTOP], limit: 10, can_add: true, can_remove: true, rp_id: 'localhost' },
    account: {
      has_password: true, has_second_factor: true, has_authenticator_app: false,
      authenticator_app_removable: false, recovery_codes_remaining: 9, second_factor_required: false,
    },
  });
  t.after(() => context.close());
  await page.getByText('Off. Your passkey or security key is your second factor.').waitFor();
  await page.getByText('Left: 9 of 10.').waitFor();
  await page.getByRole('button', { name: 'Make new codes' }).waitFor();
  assert.equal(await page.getByText('there is no second factor on this account yet', { exact: false }).count(), 0);
  assert.equal(await page.getByText('Turn the second factor off').count(), 0, 'an app nobody set up was offered to turn off');
  assert.deepEqual(failures, []);
});

// UC-ID-03 check 5 beside a key: under a rule the app may go while a key remains - and once it has,
// the last key shows the sentence instead of *Remove*, because the list was read again.
test('chromium: beside a key the app may be turned off under a rule, and then the last key stays', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const account = {
    has_password: true, has_second_factor: true, has_authenticator_app: true,
    authenticator_app_removable: true, recovery_codes_remaining: 10, second_factor_required: true,
  };
  const { context, page, sent, failures } = await open(browser, {
    list: { passkeys: [LAPTOP], limit: 10, can_add: true, can_remove: true, rp_id: 'localhost' },
    account,
    afterDisable: (list, held) => {
      Object.assign(list, { can_remove: false, cannot_remove_reason: 'auth.mfa_required_by_tenant' });
      Object.assign(held, { has_authenticator_app: false, authenticator_app_removable: false });
    },
  });
  t.after(() => context.close());
  const panel = page.getByTestId('passkeys');
  await panel.getByRole('listitem').filter({ hasText: 'Laptop' }).getByRole('button', { name: 'Remove' }).waitFor();

  await page.getByText('Turn the second factor off').click();
  await page.getByText('Your recovery codes keep working beside your passkey', { exact: false }).waitFor();
  await page.getByRole('button', { name: 'Turn it off' }).click();
  await proveWithPassword(page);
  await panel.getByText('this is the last one your account holds', { exact: false }).waitFor();
  assert.equal(await panel.getByRole('button', { name: 'Remove' }).count(), 0, 'the last key stayed removable');
  assert.deepEqual(sent.disables, [null, GRANT]);
  assert.deepEqual(failures, []);
});
