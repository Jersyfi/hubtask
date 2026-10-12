// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A managed account's later steps on *People*, walked (UC-ID-20 checks 5 and 7): a managed person's
// row offers *New start password* and *Add an address*, and nobody else's does; the new start
// password comes after a confirmation and the step-up, shown once to copy or print; the address
// sends a link and says so.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, OTHER, PAGE, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

/** The sign-in rules of a workspace whose methods are the ones named. */
function rulesWith(methods) {
  return {
    workspace_host: 'home.hubtask.eu', methods, providers: [], sign_in_names: true, legal: {},
    password: {
      min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
      min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
      breach_check: false, history_count: 0, not_current: false,
    },
  };
}

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const GRANT = 'hbt_sup_e2e';

/** Lena: a sign-in name and no address. */
export const LENA = { id: '01a0e2e0-0000-7000-8000-0000000000c1', kind: 'USER', display_name: 'Lena', status: 'ACTIVE', has_address: false, managed: true };

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, { methods = undefined } = {}) {
  const sent = { renewals: [], proofs: [], addresses: [] };
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (methods && path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, sign_in_rules: true } } });
    }
    if (methods && path === '/auth/sign-in-rules') return route.fulfill({ json: rulesWith(methods) });
    if (path === '/memberships' && url.searchParams.get('scope_type') === 'TENANT') {
      return route.fulfill({
        json: {
          ...PAGE,
          data: [
            { id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' },
            { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' },
            { id: 'm-lena', scope_type: 'TENANT', account_id: LENA.id, role: 'CONTRIBUTOR' },
          ],
        },
      });
    }
    if (path === `/accounts/${LENA.id}`) return route.fulfill({ json: LENA });
    if (path === `/accounts/${OTHER.id}`) return route.fulfill({ json: { ...OTHER, has_address: true, managed: false } });
    if (path === `/accounts/${LENA.id}:issue-start-password`) {
      const proof = request.headers()['x-hubtask-step-up'];
      sent.renewals.push(proof ?? null);
      if (proof !== GRANT) {
        return route.fulfill({
          status: 403,
          json: { code: 'forbidden', detail_code: 'auth.step_up_required', status: 403, request_id: 'req_r', params: { methods: 'PASSWORD' } },
        });
      }
      return route.fulfill({
        headers: { 'cache-control': 'no-store' },
        json: { account: { ...LENA, sign_in_name: 'lena' }, start_password: 'eeee-ffff-gggg-hhhh' },
      });
    }
    if (path === '/auth/step-up') {
      sent.proofs.push(request.postDataJSON());
      return route.fulfill({ status: 201, json: { step_up_token: GRANT, expires_at: '2099-01-01T00:00:00Z', method: 'PASSWORD' } });
    }
    if (path === `/accounts/${LENA.id}:add-address`) {
      sent.addresses.push(request.postDataJSON());
      return route.fulfill({ status: 202, json: { expires_at: '2026-10-13T09:00:00Z' } });
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

const rowOf = (page, name) => page.getByRole('row').filter({ has: page.getByRole('rowheader', { name, exact: true }) });

test('a managed person gets a new start password after a confirmation and a step-up, shown once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const lena = rowOf(page, 'Lena');
  await lena.getByRole('button', { name: 'New start password' }).waitFor({ timeout: 15_000 });
  // Nobody with an address is offered either action.
  const mara = rowOf(page, OTHER.display_name);
  await mara.waitFor();
  assert.equal(await mara.getByRole('button', { name: 'New start password' }).count(), 0);
  assert.equal(await mara.getByRole('button', { name: 'Add an address' }).count(), 0);

  await lena.getByRole('button', { name: 'New start password' }).click();
  const confirm = page.getByRole('dialog', { name: 'Give a new start password?' });
  await confirm.waitFor();
  assert.match(await confirm.innerText(), /every device they are signed in on is signed out/);
  await confirm.getByRole('button', { name: 'Give a new start password' }).click();

  const proof = page.getByRole('dialog', { name: 'Prove it is you' });
  await proof.waitFor();
  await proof.getByLabel('Your password').fill('the administrator password');
  await proof.getByRole('button', { name: 'Prove it' }).click();

  await page.getByText('New start password for Lena (lena)').waitFor({ timeout: 15_000 });
  assert.deepEqual(sent.renewals, [null, GRANT]);
  assert.equal(sent.proofs.length, 1);
  assert.ok(await page.getByRole('button', { name: 'Print' }).isVisible(), 'the new start password cannot be printed');
  assert.equal(await page.locator('[data-slip]').count(), 0, 'the slip is in the document outside printing');
});

test('an address for a managed person sends a link and says so', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const lena = rowOf(page, 'Lena');
  await lena.getByRole('button', { name: 'Add an address' }).click({ timeout: 15_000 });
  const dialog = page.getByRole('dialog', { name: 'Add an address for Lena' });
  await dialog.waitFor();
  await dialog.getByLabel('Email address').fill('lena@example.org');
  await dialog.getByRole('button', { name: 'Send the link' }).click();
  await dialog.getByText('A link went to lena@example.org. It works for a day.').waitFor();
  assert.deepEqual(sent.addresses, [{ email: 'lena@example.org' }]);
});

test('where the password is not a way in, no address is offered and the reason stands under the table', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { methods: ['OIDC'] });
  t.after(close);

  await page.goto(`${served.origin}/administration/people`);
  const lena = rowOf(page, 'Lena');
  await lena.getByRole('button', { name: 'New start password' }).waitFor({ timeout: 15_000 });
  await page.getByText('People without an address cannot add one here').waitFor();
  assert.equal(await lena.getByRole('button', { name: 'Add an address' }).count(), 0, 'an address is offered');
});
