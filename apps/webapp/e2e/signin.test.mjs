// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The sign-in card, walked: the steps the server names, the rules under a password field, and the
// two things no screenshot can answer - whether it is operable from the keyboard, and whether the
// one address bar shows one address.
//
// Chromium only, for the reason the shell walk gives: what is asserted here is behaviour rather
// than engine-specific layout, and the engines job loads the same bundle in all three.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const RULES = {
  workspace_host: 'contoso.hubtask.eu',
  methods: ['PASSWORD', 'OIDC'],
  providers: [
    { id: 'p-entra', display_name: 'Contoso Entra ID', kind: 'MICROSOFT', scope: 'workspace' },
    { id: 'p-lab', display_name: 'Contoso Lab', kind: 'GENERIC', scope: 'workspace' },
  ],
  password: {
    min_length: 14, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
    breach_check: false, history_count: 0, not_current: false,
  },
  legal: { imprint_url: 'https://example.invalid/imprint', privacy_url: 'https://example.invalid/privacy' },
};

const TOKENS = {
  token_type: 'Bearer', access_token: 'e2e-access', refresh_token: 'e2e-refresh',
  access_token_expires_at: '2099-01-01T00:00:00Z', refresh_token_expires_at: '2099-01-01T00:00:00Z',
  session: { id: 's1', created_at: '2026-09-24T00:00:00Z', current: true },
};

/** What the sign-in screens read, and nothing else: an unsigned visitor reaches nothing else. */
function stubFor({ answer, onCheck }) {
  return async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/api/v1/auth/sign-in-rules')) return route.fulfill({ json: RULES });
    if (path.endsWith('/api/v1/auth/password:check')) {
      const violations = onCheck?.(request.postDataJSON()) ?? [];
      return route.fulfill({ json: { violations } });
    }
    if (path.endsWith('/api/v1/auth/sessions') && request.method() === 'POST') {
      return route.fulfill(answer());
    }
    if (path.endsWith('/api/v1/auth/sessions:verify')) return route.fulfill({ status: 201, json: TOKENS });
    if (path.endsWith('/api/v1/meta/capabilities')) {
      return route.fulfill({ json: { product_version: 'e2e', api_version: 'v1', tenancy_mode: 'multi', item_types: [], view_layouts: [], supported_locales: [{ locale: 'en', direction: 'ltr' }], roles: [], limits: {}, features: { sign_in_rules: true } } });
    }
    return route.fulfill({ status: 404, json: { code: 'errors.not_found' } });
  };
}

async function open(origin, stub) {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', stub);
  const page = await context.newPage();
  await page.goto(origin);
  await page.waitForSelector('text=to contoso.hubtask.eu');
  return { browser, page };
}

const refused = () => ({ status: 401, json: { code: 'errors.unauthenticated', detail_code: 'auth.sign_in_failed', status: 401, request_id: 'req_e2e' } });
const owed = () => ({ status: 202, json: { pending_token: 'p1', methods: ['TOTP', 'RECOVERY'], expires_at: '2099-01-01T00:00:00Z' } });

test('the card is reached, filled and submitted from the keyboard alone', async () => {
  const { origin, close } = await serve(DIST);
  const { browser, page } = await open(origin, stubFor({ answer: () => ({ status: 201, json: TOKENS }) }));
  try {
    // Tab from the document, naming every stop: the order of the DOM has to be the order that
    // makes sense (2.4.3), and every stop has to draw the ring (2.4.7). A screenshot cannot
    // answer either, which is why this walk is here rather than in the browser pane.
    const stops = [];
    for (let index = 0; index < 8; index += 1) {
      await page.keyboard.press('Tab');
      stops.push(await page.evaluate(() => {
        const active = document.activeElement;
        if (!active || active === document.body) return 'body';
        const ringed = active.matches(':focus-visible') || active.closest('.shell') !== null;
        return `${active.tagName.toLowerCase()}:${(active.getAttribute('aria-label') ?? active.textContent ?? '').trim().slice(0, 24)}:${ringed}`;
      }));
    }
    assert.ok(!stops.includes('body'), `focus fell to the document: ${stops.join(' | ')}`);
    assert.match(stops[0], /^input:/, `the first stop is the address, not ${stops[0]}`);
    assert.match(stops[1], /^input:/, `the second stop is the password, not ${stops[1]}`);
    assert.match(stops[2], /^button:Show password/, `the eye follows the password, not ${stops[2]}`);

    // Typed, not filled: `fill` sets a value, and what is being asserted is that a person can do
    // this with their hands. From the top of a fresh document, so the count of stops above does
    // not decide where this begins.
    await page.reload();
    await page.waitForSelector('text=to contoso.hubtask.eu');
    await page.keyboard.press('Tab');
    await page.keyboard.type('walker@example.invalid');
    await page.keyboard.press('Tab');
    await page.keyboard.type('a-long-enough-password');
    await page.keyboard.press('Enter');

    await page.waitForFunction(() => !document.querySelector('input[type="email"]'));
  } finally {
    await browser.close();
    await close();
  }
});

test('the eye shows the password and says which state it is in', async () => {
  const { origin, close } = await serve(DIST);
  const { browser, page } = await open(origin, stubFor({ answer: refused }));
  try {
    const field = page.locator('input[type="password"], input[name="password"]').first();
    await field.fill('a-long-enough-password');
    const eye = page.getByRole('button', { name: 'Show password' });
    assert.equal(await eye.getAttribute('aria-pressed'), 'false');
    await eye.click();
    assert.equal(await page.locator('input[autocomplete="current-password"]').getAttribute('type'), 'text');
    const hide = page.getByRole('button', { name: 'Hide password' });
    assert.equal(await hide.getAttribute('aria-pressed'), 'true');
    await hide.click();
    assert.equal(await page.locator('input[autocomplete="current-password"]').getAttribute('type'), 'password');
  } finally {
    await browser.close();
    await close();
  }
});

test('a refusal is one sentence, and it is announced rather than only drawn', async () => {
  const { origin, close } = await serve(DIST);
  const { browser, page } = await open(origin, stubFor({ answer: refused }));
  try {
    await page.locator('input[type="email"]').fill('walker@example.invalid');
    await page.locator('input[autocomplete="current-password"]').fill('whatever-it-was');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    const alert = page.locator('[role="alert"]');
    await alert.waitFor();
    const text = await alert.innerText();
    assert.match(text, /Sign-in failed/);
    // T-02: nothing on the screen says which half was wrong.
    assert.ok(!/address .*(unknown|not found)/i.test(text), text);
    // The password is out of the field, and out of the DOM with it.
    assert.equal(await page.locator('input[autocomplete="current-password"]').inputValue(), '');
  } finally {
    await browser.close();
    await close();
  }
});

test('a second factor becomes the second step, with the code field and the identity line', async () => {
  const { origin, close } = await serve(DIST);
  const { browser, page } = await open(origin, stubFor({ answer: owed }));
  try {
    await page.locator('input[type="email"]').fill('walker@example.invalid');
    await page.locator('input[autocomplete="current-password"]').fill('whatever-it-was');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await page.waitForSelector('text=Signing in as');
    assert.ok(await page.locator('text=walker@example.invalid').count());

    // One field, six places: pasting works, which is what one field buys and six do not.
    const code = page.getByLabel('Code from your authenticator');
    await code.focus();
    await page.evaluate(() => navigator.clipboard?.writeText?.('482197')).catch(() => {});
    await code.fill('482197');
    const places = await page.locator('[class*="place"]:not([class*="places"])').allInnerTexts();
    assert.deepEqual(places, ['4', '8', '2', '1', '9', '7'], 'the picture shows what was typed');

    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await page.waitForFunction(() => !document.querySelector('input[autocomplete="one-time-code"]'));
  } finally {
    await browser.close();
    await close();
  }
});

test('the rules under a password are the workspace’s, and the server’s lines wait for the local ones', async () => {
  const { origin, close } = await serve(DIST);
  const asked = [];
  const stub = stubFor({
    answer: refused,
    // The corpus is the server's: this one refuses anything holding "password", and the point of
    // the assertion below is that the *client* did not know that and asked.
    onCheck: (body) => {
      asked.push(body.password);
      return body.password.toLowerCase().includes('password') ? [{ rule: 'common' }] : [];
    },
  });
  const { browser, page } = await open(origin, stub);
  try {
    // The reset screen is the one that sets a password without a session.
    await page.goto(`${origin}/reset#token=e2e-reset`);
    await page.waitForSelector('text=Choose a new password');
    // The credential is out of the address before anything is sent.
    assert.equal(new URL(page.url()).hash, '');

    const field = page.getByLabel('New password');
    await field.fill('short');
    await page.waitForTimeout(700);
    assert.deepEqual(asked, [], 'the server is not asked while the local rules are still open');

    const states = async () => page.locator('li[data-state]').evaluateAll((nodes) => nodes.map((n) => n.dataset.state));
    // `short` is too short and holds no context word: one line open, one met, and the server's
    // line still waiting - which is the whole point, because a password this short is not worth a
    // round trip.
    assert.deepEqual(await states(), ['unmet', 'met', 'server'], 'nothing is being checked yet');

    await field.fill('a-long-enough-password');
    await page.waitForFunction(() => document.querySelector('li[data-state="failed"]') !== null, null, { timeout: 4000 });
    assert.deepEqual(asked, ['a-long-enough-password'], 'asked once, after the typing stopped');
    assert.deepEqual(await states(), ['met', 'met', 'failed']);
    // Rule 3: the state is a character and a word, never the colour alone.
    assert.match(await page.locator('li[data-state="failed"]').innerText(), /Not met/);
    // The field is marked wrong even though the sentence lives in the list.
    assert.equal(await field.getAttribute('aria-invalid'), 'true');

    // The same value again is answered from what is already known.
    await field.fill('short');
    await field.fill('a-long-enough-password');
    await page.waitForTimeout(700);
    assert.deepEqual(asked, ['a-long-enough-password'], 'a value already answered is not asked again');
  } finally {
    await browser.close();
    await close();
  }
});

test('the card is one landmark, one heading, and no sideways scroll at 375 px', async () => {
  const { origin, close } = await serve(DIST);
  const browser = await chromium.launch();
  try {
    const context = await browser.newContext({ viewport: { width: 375, height: 812 } });
    await context.route('**/api/v1/**', stubFor({ answer: refused }));
    const page = await context.newPage();
    await page.goto(origin);
    await page.waitForSelector('text=to contoso.hubtask.eu');

    assert.equal(await page.locator('main').count(), 1);
    assert.equal(await page.locator('h1').count(), 1);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    assert.ok(overflow <= 0, `the page scrolls sideways by ${overflow}px`);
    // 1.4.10's gutter: the card does not touch the edge of the screen.
    const left = await page.locator('section').first().evaluate((node) => node.getBoundingClientRect().left);
    assert.ok(left >= 8, `the card sits ${left}px from the edge`);
  } finally {
    await browser.close();
    await close();
  }
});

test('every provider is a button of its own, and only the pressed one is working', async () => {
  const { origin, close } = await serve(DIST);
  const { browser, page } = await open(origin, async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith('/api/v1/auth/oidc:start')) {
      // Never answered: what is asserted is which button says it is working while it waits.
      return new Promise(() => {});
    }
    return stubFor({ answer: refused })(route);
  });
  try {
    const entra = page.getByRole('button', { name: /Contoso Entra ID/ });
    const lab = page.getByRole('button', { name: /Contoso Lab/ });
    await entra.click();
    await page.waitForFunction(() => document.querySelector('[aria-busy="true"]') !== null);
    assert.equal(await entra.getAttribute('aria-busy'), 'true');
    assert.equal(await lab.getAttribute('aria-busy'), null, 'the other provider says it is working too');
  } finally {
    await browser.close();
    await close();
  }
});
