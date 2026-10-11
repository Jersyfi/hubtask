// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The sign-in card in a workspace with managed accounts (UC-ID-20 checks 3, 4 and 6): the first
// field takes an address or a sign-in name exactly where the workspace has one, a sign-in name
// travels as the identifier, the forgotten-password detour says there is no mail for a name, and a
// change the sign-in still owes after a demanded setup is the next card rather than a session.
// Where the workspace switched the password off, a sign-in name keeps a form of its own beside the
// providers (identity.md §4.1).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ENROLLMENT, walkSetup } from './secondfactor.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const RULES = {
  workspace_host: 'family.hubtask.eu',
  methods: ['PASSWORD'],
  providers: [],
  password: {
    min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
    breach_check: false, history_count: 0, not_current: false,
  },
  legal: {},
};

const MANIFEST = {
  product_version: 'e2e', api_version: 'v1', tenancy_mode: 'multi', item_types: [], view_layouts: [],
  supported_locales: [{ locale: 'en', direction: 'ltr' }], roles: [], limits: {},
  features: { sign_in_rules: true },
};

// The password switched off, one provider the way in for everybody else.
const PROVIDERS_ONLY = {
  methods: ['OIDC'],
  providers: [{ id: 'p-family', display_name: 'Family ID', kind: 'GENERIC', scope: 'workspace' }],
};

function stub({ names, sent, answer, rules }) {
  return async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/api/v1/auth/sign-in-rules')) {
      return route.fulfill({ json: { ...RULES, ...rules, sign_in_names: names } });
    }
    if (path.endsWith('/api/v1/auth/password:check')) return route.fulfill({ json: { violations: [] } });
    if (path.endsWith('/api/v1/meta/capabilities')) return route.fulfill({ json: MANIFEST });
    if (path.endsWith('/api/v1/auth/mfa/totp:enroll')) return route.fulfill({ json: ENROLLMENT });
    if (path.endsWith('/api/v1/auth/mfa/totp:confirm')) {
      sent.confirm = request.postDataJSON();
      return route.fulfill({
        json: {
          armed: true,
          challenge: {
            pending_token: 'change-1', methods: ['PASSWORD_CHANGE'],
            expires_at: new Date(Date.now() + 300_000).toISOString(), password_rules: RULES.password,
          },
        },
      });
    }
    if (path.endsWith('/api/v1/auth/sessions') && request.method() === 'POST') {
      sent.signIn = request.postDataJSON();
      return route.fulfill(answer());
    }
    return route.fulfill({ status: 404, json: { code: 'errors.not_found' } });
  };
}

async function open(names, sent, answer, rules = {}) {
  const { origin, close } = await serve(DIST);
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 375, height: 812 } });
  await context.route('**/api/v1/**', stub({ names, sent, answer, rules }));
  const page = await context.newPage();
  await page.goto(origin);
  await page.waitForSelector('text=to family.hubtask.eu');
  return { page, done: async () => { await browser.close(); await close(); } };
}

const refused = () => ({
  status: 401,
  json: { code: 'errors.unauthenticated', detail_code: 'auth.sign_in_failed', status: 401, request_id: 'req_e2e' },
});

test('where the workspace has a managed account, the first field takes a sign-in name and sends it as the identifier', async () => {
  const sent = {};
  const { page, done } = await open(true, sent, refused);
  try {
    const field = page.getByLabel('Email address or sign-in name');
    await field.waitFor();
    assert.equal(await field.getAttribute('type'), 'text', 'an address field would refuse a name');
    await field.fill('lena');
    await page.locator('input[autocomplete="current-password"]').fill('wrong-start-password');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await page.getByText('Sign-in failed', { exact: false }).waitFor();
    assert.deepEqual(sent.signIn, { identifier: 'lena', password: 'wrong-start-password' });

    // The detour says once, for everybody, that a sign-in name has no mail.
    await page.getByRole('button', { name: 'Forgot your password?' }).click();
    await page.getByText('Signing in with a sign-in name?', { exact: false }).waitFor();
  } finally {
    await done();
  }
});

test('elsewhere the card is unchanged: an address field and no sentence about names', async () => {
  const sent = {};
  const { page, done } = await open(false, sent, refused);
  try {
    const field = page.locator('input[autocomplete^="username"]');
    await field.waitFor();
    assert.equal(await field.getAttribute('type'), 'email');
    assert.equal(await page.getByLabel('Email address or sign-in name').count(), 0);
    assert.equal(await page.getByText('sign-in name', { exact: false }).count(), 0);
    await page.getByRole('button', { name: 'Forgot your password?' }).click();
    await page.getByRole('heading', { name: 'Reset your password' }).waitFor();
    assert.equal(await page.getByText('Signing in with a sign-in name?', { exact: false }).count(), 0);
  } finally {
    await done();
  }
});

test('a demanded setup that the password still owes a change for leads to the change, not to a session', async () => {
  const sent = {};
  const { page, done } = await open(true, sent, () => ({
    status: 202,
    json: { pending_token: 'enroll-1', methods: ['ENROLL'], expires_at: new Date(Date.now() + 300_000).toISOString() },
  }));
  try {
    await page.getByLabel('Email address or sign-in name').fill('lena');
    await page.locator('input[autocomplete="current-password"]').fill('the-start-password');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await page.getByRole('heading', { name: 'Second factor' }).waitFor();
    await walkSetup(page, sent);
    assert.equal(sent.confirm?.pending_token, 'enroll-1');
    // Continue leads to choosing the person's own password.
    await page.getByLabel('New password').waitFor();
  } finally {
    await done();
  }
});

test('the password switched off and a managed account here: the providers, and a sign-in name form after them', async () => {
  const sent = {};
  const { page, done } = await open(true, sent, refused, PROVIDERS_ONLY);
  try {
    await page.getByRole('button', { name: 'Sign in with Family ID' }).waitFor();
    const field = page.getByLabel('Sign-in name');
    await field.waitFor();
    assert.equal(await field.getAttribute('type'), 'text');
    assert.equal(await page.getByLabel('Email address or sign-in name').count(), 0);
    // The address detour is not beside the name: there is no mail for a name.
    assert.equal(await page.getByRole('button', { name: 'Forgot your password?' }).count(), 0);
    await field.fill('lena');
    await page.locator('input[autocomplete="current-password"]').fill('wrong-start-password');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await page.getByText('Sign-in failed', { exact: false }).waitFor();
    assert.deepEqual(sent.signIn, { identifier: 'lena', password: 'wrong-start-password' });
  } finally {
    await done();
  }
});

test('the password switched off and no managed account: the providers are the whole card', async () => {
  const sent = {};
  const { page, done } = await open(false, sent, refused, PROVIDERS_ONLY);
  try {
    await page.getByRole('button', { name: 'Sign in with Family ID' }).waitFor();
    assert.equal(await page.locator('input[autocomplete="current-password"]').count(), 0);
    assert.equal(await page.getByLabel('Sign-in name').count(), 0);
  } finally {
    await done();
  }
});

// The card's first field is drawn once the rules are in hand, never before: drawn from the default,
// it read "Email address" in a workspace with managed accounts until the answer landed, and the
// first thing somebody saw - or a walk photographed - was the wrong field.
async function openSlow({ names, rules, rulesStatus = 200, delay = 1200 }) {
  const { origin, close } = await serve(DIST);
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 375, height: 812 } });
  const answer = stub({ names, sent: {}, answer: refused, rules });
  await context.route('**/api/v1/**', async (route) => {
    if (route.request().url().includes('/auth/sign-in-rules')) {
      await new Promise((resolve) => setTimeout(resolve, delay));
      if (rulesStatus !== 200) return route.fulfill({ status: rulesStatus, json: { code: 'errors.internal' } });
    }
    return answer(route);
  });
  const page = await context.newPage();
  await page.goto(origin);
  return { page, done: async () => { await browser.close(); await close(); } };
}

test('a slow answer: the first field is drawn with the label the workspace has, not the default before it', async () => {
  const { page, done } = await openSlow({ names: true });
  try {
    const field = page.locator('input[autocomplete^="username"]');
    await field.waitFor();
    // At the moment the field first exists - no waiting for a label to change under somebody.
    assert.equal(await page.getByLabel('Email address or sign-in name').count(), 1);
    assert.equal(await field.getAttribute('type'), 'text');
  } finally {
    await done();
  }
});

test('a slow answer with the password off: no password form appears before the providers do', async () => {
  const { page, done } = await openSlow({ names: false, rules: PROVIDERS_ONLY });
  try {
    await page.waitForSelector('text=Sign in', { state: 'visible' });
    // While the answer is outstanding the card offers nothing it may have to take back.
    assert.equal(await page.locator('input[autocomplete="current-password"]').count(), 0);
    await page.getByRole('button', { name: 'Sign in with Family ID' }).waitFor();
    assert.equal(await page.locator('input[autocomplete="current-password"]').count(), 0);
  } finally {
    await done();
  }
});

test('rules that cannot be read: the card falls back to the address field and still works', async () => {
  const { page, done } = await openSlow({ names: true, rulesStatus: 500, delay: 200 });
  try {
    const field = page.locator('input[autocomplete^="username"]');
    await field.waitFor();
    assert.equal(await field.getAttribute('type'), 'email');
    assert.equal(await page.getByLabel('Email address or sign-in name').count(), 0);
  } finally {
    await done();
  }
});

// The change step's line about context words names what is compared: a sign-in name for a person
// who signed in with one - they have no address - and the address for everybody else.
const owesChange = () => ({
  status: 202,
  json: {
    pending_token: 'change-2', methods: ['PASSWORD_CHANGE'],
    expires_at: new Date(Date.now() + 300_000).toISOString(), password_rules: RULES.password,
  },
});

for (const [who, typed, shown, hidden] of [
  ['a sign-in name', 'LENA', 'Not containing your sign-in name', 'Not containing your email address'],
  ['an address', 'anna@contoso.example', 'Not containing your email address', 'Not containing your sign-in name'],
]) {
  test(`the change step after ${who} names what the password is compared with`, async () => {
    const { page, done } = await open(true, {}, owesChange);
    try {
      await page.getByLabel('Email address or sign-in name').fill(typed);
      await page.locator('input[autocomplete="current-password"]').fill('the-start-password');
      await page.getByRole('button', { name: 'Sign in', exact: true }).click();
      await page.getByLabel('New password').waitFor();
      await page.getByText(shown, { exact: false }).waitFor();
      assert.equal(await page.getByText(hidden, { exact: false }).count(), 0);
    } finally {
      await done();
    }
  });
}
