// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A managed person adds an address, and the link confirms it (UC-ID-20 check 7), walked: the profile
// offers *Add an address* to a person with a sign-in name and no address and to nobody else; the
// mailed link opens a card that confirms on a press, never on opening, with the token out of the
// address before anything is sent.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const LENA = { ...ACCOUNT, display_name: 'Lena', email: null, sign_in_name: 'lena' };
const TOKEN = 'hbt_mfa_e2e-address-token';

const served = await serve(DIST);
test.after(() => served.close());

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

async function open(browser, { me = LENA, signedIn = true, confirm = 204, methods = undefined } = {}) {
  const sent = { addresses: [], confirmations: [] };
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (methods && path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, sign_in_rules: true } } });
    }
    if (methods && path === '/auth/sign-in-rules') return route.fulfill({ json: rulesWith(methods) });
    if (path === '/accounts/me') return route.fulfill({ json: me });
    if (path === `/accounts/${me.id}:add-address`) {
      sent.addresses.push(request.postDataJSON());
      return route.fulfill({ status: 202, json: { expires_at: '2026-10-13T09:00:00Z' } });
    }
    if (path === '/auth/address:confirm') {
      sent.confirmations.push(request.postDataJSON());
      if (confirm === 204) return route.fulfill({ status: 204 });
      return route.fulfill({
        status: confirm,
        json: { code: 'unauthenticated', detail_code: 'auth.address_link_invalid', status: confirm, request_id: 'req_c' },
      });
    }
    return stub(route);
  });
  if (signedIn) {
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
  }
  const page = await context.newPage();
  return { page, sent, close: () => context.close() };
}

test('a managed person adds an address on the profile, and a link goes to it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  await page.getByRole('heading', { name: 'A mail address' }).waitFor({ timeout: 15_000 });
  await page.getByText('You sign in with your sign-in name, lena,').waitFor();
  await page.getByLabel('Email address').fill('lena@example.org');
  await page.getByRole('button', { name: 'Send the link' }).click();
  await page.getByText('A link went to lena@example.org. It works for a day.').first().waitFor();
  assert.deepEqual(sent.addresses, [{ email: 'lena@example.org' }]);
});

test('where the password is not a way in, the profile says why instead of offering an address', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { methods: ['OIDC'] });
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  await page.getByRole('heading', { name: 'A mail address' }).waitFor({ timeout: 15_000 });
  await page.getByText('An address cannot be added in this workspace').waitFor();
  assert.equal(await page.getByRole('button', { name: 'Send the link' }).count(), 0, 'the form is offered');
});

test('where only the password is a way in, the profile offers the address', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { methods: ['PASSWORD'] });
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  await page.getByRole('button', { name: 'Send the link' }).waitFor({ timeout: 15_000 });
});

test('a person with an address is offered none', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { me: ACCOUNT });
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  await page.getByRole('button', { name: 'Save' }).waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('heading', { name: 'A mail address' }).count(), 0);
});

test('the mailed link confirms on a press, with the token out of the address first', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, { signedIn: false });
  t.after(close);

  await page.goto(`${served.origin}/confirm-address#token=${TOKEN}`);
  await page.getByRole('heading', { name: 'Confirm your address' }).waitFor({ timeout: 15_000 });
  assert.equal(new URL(page.url()).hash, '', 'the token stayed in the address');
  assert.equal(sent.confirmations.length, 0, 'opening the link confirmed it');

  await page.getByRole('button', { name: 'Confirm', exact: true }).click();
  await page.getByText('The address is confirmed.').waitFor();
  assert.deepEqual(sent.confirmations, [{ token: TOKEN }]);
});

test('a link that cannot confirm says so', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { signedIn: false, confirm: 401 });
  t.after(close);

  await page.goto(`${served.origin}/confirm-address#token=${TOKEN}`);
  await page.getByRole('button', { name: 'Confirm', exact: true }).click({ timeout: 15_000 });
  await page.getByText('That confirmation link cannot be used.').waitFor();
});
