// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An administrative token is created in the web app behind a fresh proof (UC-ID-17 check 3), for
// oneself and for a service account (UC-INS-06 check 2): the server asks, the dialog takes the proof
// once, the mint is sent again with it, and the token is shown once. The proof is the person's
// creating it, never the service account's.

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

const SERVICE = { id: '01a0e2e0-0000-7000-8000-0000000000f1', kind: 'SERVICE', display_name: 'Shop platform', status: 'ACTIVE' };
const OWNER = { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'DELETE_CONTAINER'] };

async function open(browser) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const mints = [];
  const proofs = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, roles: [OWNER], token_scopes: ['account:read', 'admin:tenants'] } });
    }
    if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, has_password: true, has_second_factor: false } });
    if (path === '/auth/service-accounts' && request.method() === 'GET') return route.fulfill({ json: [SERVICE] });
    if (path === '/auth/tokens' && request.method() === 'POST') {
      const proof = request.headers()['x-hubtask-step-up'] ?? null;
      const body = request.postDataJSON();
      mints.push({ proof, body });
      if (body.scopes.includes('admin:tenants') && proof !== 'hbt_sup_tok') {
        return route.fulfill({ status: 403, json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'PASSWORD' }, request_id: 'req_t' } });
      }
      return route.fulfill({ status: 201, json: {
        id: 't-new', name: body.name, prefix: 'hbt_pat_e2e', scopes: body.scopes, created_at: '2026-10-13T00:00:00Z',
        expires_at: body.expires_at, last_used_at: null, revoked_at: null, token: 'hbt_pat_e2e-the-whole-token',
      } });
    }
    if (path === '/auth/step-up') {
      proofs.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { step_up_token: 'hbt_sup_tok', expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, mints, proofs, close: () => context.close() };
}

async function mintAdministrative(page, within) {
  await within.getByLabel(/^What it is for/).fill('Provisioning');
  await within.getByLabel(/^Expires/).fill(new Date(Date.now() + 30 * 86_400_000).toISOString().slice(0, 10));
  await within.getByLabel('admin:tenants', { exact: true }).check();
  await within.getByRole('button', { name: 'Create the token' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  await dialog.getByLabel('Your password').fill('the person creating it');
  await dialog.getByRole('button', { name: 'Prove it' }).click();
  await page.getByText('Your new token').first().waitFor({ timeout: 15_000 });
}

test('chromium: an administrative token of one\'s own is created behind a fresh proof (UC-ID-17 check 3)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, mints, proofs, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/profile/tokens`);
  await page.getByRole('button', { name: 'Create the token' }).waitFor();
  await mintAdministrative(page, page.locator('main'));

  assert.deepEqual(proofs, [{ password: 'the person creating it' }], 'the proof was asked more than once, or not at all');
  assert.deepEqual(mints.map((each) => each.proof), [null, 'hbt_sup_tok']);
  assert.deepEqual(mints[1].body.scopes, ['admin:tenants']);
  assert.equal(mints[1].body.account_id, undefined, 'a token of one\'s own named an account');
});

test('chromium: a service account\'s administrative token is created behind the person\'s fresh proof (UC-INS-06 check 2)', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, mints, proofs, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/service-accounts`);
  const section = page.locator('section', { hasText: SERVICE.display_name });
  await section.getByRole('button', { name: 'Its tokens' }).click();
  await section.getByRole('button', { name: 'Create the token' }).waitFor();
  await mintAdministrative(page, section);

  assert.deepEqual(proofs, [{ password: 'the person creating it' }]);
  assert.deepEqual(mints.map((each) => each.proof), [null, 'hbt_sup_tok']);
  assert.equal(mints[1].body.account_id, SERVICE.id, 'the token was not minted for the service account');
  assert.deepEqual(mints[1].body.scopes, ['admin:tenants']);
});
