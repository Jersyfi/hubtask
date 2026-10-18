// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A paused workspace (UC-INS-08 check 2, UC-INS-13 check 11): the sign-in screen says it is paused,
// that the data is kept and that the provider knows why - with "Your provider's account page" where
// the refusal carries one, which the server does only for an owner or administrator who completed
// the proof - and a person whose workspace was paused under their session is signed out to the same
// notice. Against the stubbed API; Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const ACCOUNT_PAGE = 'https://shop.example/account';

const RULES = {
  workspace_host: 'familie-berger.example.com', methods: ['PASSWORD'], providers: [],
  password: {
    min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
    breach_check: false, history_count: 0, not_current: false,
  },
  legal: {},
};

const paused = (params) => ({
  status: 403,
  json: { code: 'forbidden', detail_code: 'access.tenant_suspended', status: 403, request_id: 'req_e2e', ...(params ? { params } : {}) },
});

async function signInTo(browser, answer, verify) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: RULES });
    if (path === '/auth/sessions' && request.method() === 'POST') return route.fulfill(answer);
    if (path === '/auth/sessions:verify' && verify) return route.fulfill(verify);
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { product_version: 'e2e', api_version: 'v1', tenancy_mode: 'multi', item_types: [], view_layouts: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], roles: [], limits: {}, features: { sign_in_rules: true } } });
    }
    return route.fulfill({ status: 404, json: { code: 'errors.not_found' } });
  });
  const page = await context.newPage();
  await page.goto(served.origin);
  await page.waitForSelector('text=to familie-berger.example.com');
  await page.locator('input[type="email"]').fill('maria@example.invalid');
  await page.locator('input[autocomplete="current-password"]').fill('the right password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  if (verify) {
    await page.getByLabel('Code from your authenticator').fill('482197');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  }
  await page.getByTestId('paused').waitFor({ timeout: 5_000 });
  return { page, context };
}

test('chromium: an owner signing in to a paused workspace is told so, with the provider\'s account page', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await signInTo(browser, paused({ account_url: ACCOUNT_PAGE }));
  t.after(() => context.close());

  const notice = await page.getByTestId('paused').innerText();
  assert.match(notice, /This workspace is paused/);
  assert.match(notice, /Your data is kept in full/);
  const link = page.getByRole('link', { name: 'Your provider\'s account page' });
  assert.equal(await link.getAttribute('href'), ACCOUNT_PAGE);
  assert.equal(await page.locator('[role="alert"]').count(), 0, 'the pause reads as a refusal of what was typed');
});

test('chromium: after the second step, the paused notice replaces the code form', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const owed = { status: 202, json: { pending_token: 'p1', methods: ['TOTP', 'RECOVERY'], expires_at: '2099-01-01T00:00:00Z' } };
  const { page, context } = await signInTo(browser, owed, paused({ account_url: ACCOUNT_PAGE }));
  t.after(() => context.close());

  assert.equal(await page.getByRole('link', { name: 'Your provider\'s account page' }).getAttribute('href'), ACCOUNT_PAGE);
  assert.equal(await page.getByLabel('Code from your authenticator').count(), 0, 'the refused step stays to be retried');
});

test('chromium: anybody else is told the workspace is paused, without a link', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await signInTo(browser, paused());
  t.after(() => context.close());

  assert.match(await page.getByTestId('paused').innerText(), /This workspace is paused/);
  assert.equal(await page.getByRole('link', { name: 'Your provider\'s account page' }).count(), 0);
});

test('chromium: a workspace paused under a session signs the person out to the same notice', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());

  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: RULES });
    if (path === '/quotas') return route.fulfill(paused());
    return stub(route);
  });
  await page.goto(`${served.origin}/administration/quotas`);
  await page.getByTestId('paused').waitFor({ timeout: 5_000 });
  assert.match(await page.getByTestId('paused').innerText(), /This workspace is paused/);
  assert.equal(await page.evaluate(() => sessionStorage.getItem('hubtask.bearer')), null, 'the credential was kept');
});
