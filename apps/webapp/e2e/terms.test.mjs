// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The terms of use on the cards (UC-ID-19): the box on the invitation card, the TERMS step after a
// sign-in, declining it, and nothing at all where no terms are set.
//
// Chromium only, for the sign-in walk's reason: what is asserted is behaviour, not layout.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const TERMS_URL = 'https://acme.example/terms';

const RULES = {
  workspace_host: 'contoso.hubtask.eu',
  methods: ['PASSWORD', 'OIDC'],
  providers: [{ id: 'p-entra', display_name: 'Contoso Entra ID', kind: 'MICROSOFT', scope: 'workspace' }],
  password: {
    min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: false, context_words: false,
    breach_check: false, history_count: 0, not_current: false,
  },
  legal: { terms_url: TERMS_URL, terms_version: '3', terms_set_by: 'WORKSPACE' },
};

const TOKENS = {
  token_type: 'Bearer', access_token: 'e2e-access', refresh_token: 'e2e-refresh',
  access_token_expires_at: '2099-01-01T00:00:00Z', refresh_token_expires_at: '2099-01-01T00:00:00Z',
  session: { id: 's1', created_at: '2026-09-24T00:00:00Z', current: true },
};

const TERMS_STEP = {
  status: 202,
  json: {
    pending_token: 'p-terms', methods: ['TERMS'], expires_at: '2099-01-01T00:00:00Z',
    terms: { url: TERMS_URL, version: '4', set_by: 'WORKSPACE' },
  },
};

/** The signed-out reads, the requests recorded by path, and a session where one is opened. */
function stubFor(rules, sent, answers = {}) {
  return async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() === 'POST') sent.push({ path, body: request.postDataJSON() });
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: rules });
    if (path === '/auth/password:check') return route.fulfill({ json: { violations: [] } });
    if (path in answers) {
      const answer = answers[path]();
      // `null` is a request left unanswered: what is asserted is what it carried.
      return answer === null ? new Promise(() => {}) : route.fulfill(answer);
    }
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { product_version: 'e2e', api_version: 'v1', tenancy_mode: 'multi', item_types: [], view_layouts: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], roles: [], limits: {}, features: { sign_in_rules: true } } });
    }
    return route.fulfill({ status: 404, json: { code: 'errors.not_found' } });
  };
}

async function openAt(path, stub) {
  const { origin, close } = await serve(DIST);
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', stub);
  const page = await context.newPage();
  await page.goto(`${origin}${path}`);
  return { page, done: async () => { await browser.close(); await close(); } };
}

// Check 2: after the password, the step; agreeing sends the terms as shown and opens the session.
test('a sign-in owed the terms shows them and agreeing opens the session', async () => {
  const sent = [];
  const { page, done } = await openAt('/', stubFor(RULES, sent, {
    '/auth/sessions': () => TERMS_STEP,
    '/auth/sessions:agree-terms': () => ({ status: 201, json: TOKENS }),
  }));
  try {
    await page.locator('input[type="email"]').fill('anna@example.invalid');
    await page.locator('input[autocomplete="current-password"]').fill('a long enough password');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await page.getByRole('heading', { name: 'Terms of use' }).waitFor();
    const link = page.getByRole('link', { name: 'Read the terms of use (version 4)' });
    assert.equal(await link.getAttribute('href'), TERMS_URL);
    assert.equal(await link.getAttribute('target'), '_blank');
    await page.getByText('anna@example.invalid').waitFor();

    await page.getByRole('button', { name: 'Agree and continue' }).click();
    await page.waitForFunction(() => sessionStorage.getItem('hubtask.bearer') === 'e2e-access');
    const agreed = sent.find((each) => each.path === '/auth/sessions:agree-terms');
    assert.deepEqual(agreed?.body, { pending_token: 'p-terms', terms_url: TERMS_URL, terms_version: '4' });
  } finally {
    await done();
  }
});

// Check 3: declining opens no session and says whom to ask.
test('declining the terms opens no session and says whom to ask', async () => {
  const sent = [];
  const { page, done } = await openAt('/', stubFor(RULES, sent, { '/auth/sessions': () => TERMS_STEP }));
  try {
    await page.locator('input[type="email"]').fill('anna@example.invalid');
    await page.locator('input[autocomplete="current-password"]').fill('a long enough password');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await page.getByRole('heading', { name: 'Terms of use' }).waitFor();

    await page.getByRole('button', { name: 'Decline' }).click();
    await page.getByText('You are not signed in').waitFor();
    await page.getByText(/ask the workspace's administrators/).waitFor();
    assert.equal(await page.getByRole('heading', { name: 'Sign in' }).count(), 1, 'the card is not back at step one');
    assert.ok(!sent.some((each) => each.path === '/auth/sessions:agree-terms'), 'declining sent an agreement');
    assert.equal(await page.evaluate(() => sessionStorage.getItem('hubtask.bearer')), null);
  } finally {
    await done();
  }
});

// Check 1, UC-ID-07 check 4: the invitation is not accepted without the box, and the terms agreed
// travel with the password and with the provider.
test('the invitation card asks for the agreement and sends the terms with either way in', async () => {
  const sent = [];
  const { page, done } = await openAt('/redeem#token=invitation-token', stubFor(RULES, sent, {
    '/auth/invitations:redeem': () => ({ status: 201, json: TOKENS }),
  }));
  try {
    await page.getByRole('link', { name: 'Read the terms of use (version 3)' }).waitFor();
    const box = page.getByRole('checkbox', { name: 'I agree to the terms of use (version 3)' });

    // A provider without the box: the card says so at the box, and the browser stays.
    await page.getByRole('button', { name: /Contoso Entra ID/ }).click();
    await page.getByRole('alert').filter({ hasText: 'Agree to the terms of use to continue.' }).waitFor();
    assert.ok(!sent.some((each) => each.path === '/auth/oidc:start'), 'the provider started without the agreement');
    assert.equal(await box.getAttribute('aria-invalid'), 'true');

    // The password without the box: the same.
    await page.getByLabel(/New password/).fill('seven blue lanterns over the harbour');
    await page.getByRole('button', { name: 'Set the password and sign in' }).click();
    assert.ok(!sent.some((each) => each.path === '/auth/invitations:redeem'), 'the redemption went without the agreement');

    // Ticked: the sentence goes, and the redemption carries the terms as shown.
    await box.check();
    assert.equal(await page.getByText('Agree to the terms of use to continue.').count(), 0);
    await page.getByLabel(/New password/).fill('seven blue lanterns over the harbour');
    await page.getByRole('button', { name: 'Set the password and sign in' }).click();
    await page.waitForFunction(() => sessionStorage.getItem('hubtask.bearer') === 'e2e-access');
    const redeemed = sent.find((each) => each.path === '/auth/invitations:redeem');
    assert.equal(redeemed?.body.terms_url, TERMS_URL);
    assert.equal(redeemed?.body.terms_version, '3');
  } finally {
    await done();
  }
});

test('the agreement on the invitation card travels with the provider', async () => {
  const sent = [];
  const { page, done } = await openAt('/redeem#token=invitation-token', stubFor(RULES, sent, {
    // Never answered: what is asserted is what the start carried.
    '/auth/oidc:start': () => null,
  }));
  try {
    await page.getByRole('checkbox', { name: 'I agree to the terms of use (version 3)' }).check();
    await page.getByRole('button', { name: /Contoso Entra ID/ }).click();
    await page.waitForFunction(() => document.querySelector('[aria-busy="true"]') !== null);
    const started = sent.find((each) => each.path === '/auth/oidc:start');
    assert.deepEqual(started?.body, {
      provider_id: 'p-entra', invitation_token: 'invitation-token', terms_url: TERMS_URL, terms_version: '3',
    });
  } finally {
    await done();
  }
});

// UC-ID-07 check 1 (finding #1289): the card names the workspace and the invited address before
// anything is typed, read with the token in the body; one that would not redeem says so at once.
test('the invitation card names the workspace and the address', async () => {
  const sent = [];
  const { page, done } = await openAt('/redeem#token=invitation-token', stubFor(RULES, sent, {
    '/auth/invitations:inspect': () => ({ json: { workspace_name: 'Family Winkel', email: 'anna@example.invalid' } }),
  }));
  try {
    await page.getByRole('heading', { name: 'Join Family Winkel' }).waitFor();
    await page.getByText('anna@example.invalid').waitFor();
    const inspected = sent.find((each) => each.path === '/auth/invitations:inspect');
    assert.deepEqual(inspected?.body, { token: 'invitation-token' });
    assert.ok(!page.url().includes('invitation-token'), 'the token stayed in the address');
  } finally {
    await done();
  }

  const refused = [];
  const second = await openAt('/redeem#token=spent-token', stubFor(RULES, refused, {
    '/auth/invitations:inspect': () => ({
      status: 401, json: { code: 'errors.unauthenticated', detail_code: 'auth.redemption_failed', status: 401, request_id: 'req_e2e' },
    }),
  }));
  try {
    await second.page.getByText('That invitation cannot be redeemed. Ask for a new invitation.').waitFor();
  } finally {
    await second.done();
  }
});

// Check 5: where no terms are set, no box and nothing sent about them.
test('no terms, no box', async () => {
  const sent = [];
  const { page, done } = await openAt('/redeem#token=invitation-token', stubFor({ ...RULES, legal: {} }, sent, {
    '/auth/invitations:redeem': () => ({ status: 201, json: TOKENS }),
  }));
  try {
    await page.getByLabel(/New password/).waitFor();
    assert.equal(await page.getByRole('checkbox').count(), 0, 'a box was drawn without terms');
    await page.getByLabel(/New password/).fill('seven blue lanterns over the harbour');
    await page.getByRole('button', { name: 'Set the password and sign in' }).click();
    await page.waitForFunction(() => sessionStorage.getItem('hubtask.bearer') === 'e2e-access');
    const redeemed = sent.find((each) => each.path === '/auth/invitations:redeem');
    assert.ok(redeemed && !('terms_url' in redeemed.body), 'terms were sent where none are set');
  } finally {
    await done();
  }
});
