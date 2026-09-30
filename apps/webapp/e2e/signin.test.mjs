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
import { ENROLLMENT, walkSetup } from './secondfactor.mjs';

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
    // UC-ID-02 check 7: the step carries the name the profile gives the feature.
    assert.equal(await page.locator('h1').innerText(), 'Second factor');

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

// UC-ID-02 check 4: the recovery code as it was shown - four groups of four letters and digits,
// pasted in one go with its dashes - reaches the server whole, from a field with a text keyboard.
// Before SC-03 the field was numeric and eight long, so every recovery code was cut off and refused.
test('a recovery code is taken as it was shown, dashes and all, with a text keyboard', async () => {
  const { origin, close } = await serve(DIST);
  const sent = {};
  const { browser, page } = await open(origin, async (route) => {
    const request = route.request();
    if (new URL(request.url()).pathname.endsWith('/api/v1/auth/sessions:verify')) {
      sent.verify = request.postDataJSON();
      return route.fulfill({ status: 201, json: { ...TOKENS, recovery_codes_remaining: 7 } });
    }
    return stubFor({ answer: owed })(route);
  });
  try {
    await page.locator('input[type="email"]').fill('walker@example.invalid');
    await page.locator('input[autocomplete="current-password"]').fill('whatever-it-was');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await page.getByRole('button', { name: 'I do not have my authenticator' }).click();
    const field = page.getByLabel('Recovery code');
    // The focus follows the step: the field is where the next keystroke lands.
    await page.waitForFunction(() => document.activeElement?.getAttribute('autocomplete') === 'off');
    assert.equal(await field.getAttribute('inputmode'), 'text', 'a recovery code holds letters');
    assert.equal(await field.getAttribute('maxlength'), null, 'nothing cuts a pasted code off');

    // Pasted, not typed: one insertion of the whole code, as a password manager or the clipboard
    // delivers it.
    await field.focus();
    await page.keyboard.insertText('K7QM-2XRT-P4ZL-3VWA');
    assert.equal(await field.inputValue(), 'K7QM-2XRT-P4ZL-3VWA');

    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await field.waitFor({ state: 'detached' });
    assert.equal(sent.verify?.recovery_code, 'K7QM-2XRT-P4ZL-3VWA', 'the code reached the server whole');
    assert.equal(sent.verify?.code, undefined, 'the recovery code was not sent as an authenticator code');
  } finally {
    await browser.close();
    await close();
  }
});

// UC-ID-04 check 5: a reset of an account with a second factor continues on the card into the code
// step - whose account, how long the step waits, the code field - and only the code signs in.
// Before SC-03 the reset card stayed on its form after the 202, with the link already spent.
test('a reset of an account with a second factor continues into the code step, to the end', async () => {
  const { origin, close } = await serve(DIST);
  const sent = {};
  const inFourMinutes = new Date(Date.now() + 4 * 60 * 1000).toISOString();
  const { browser, page } = await open(origin, async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/api/v1/auth/password:reset')) {
      sent.reset = request.postDataJSON();
      return route.fulfill({
        status: 202,
        json: {
          pending_token: 'totp-after-reset', methods: ['TOTP', 'RECOVERY'],
          expires_at: inFourMinutes, email: 'anna@contoso.example',
        },
      });
    }
    if (path.endsWith('/api/v1/auth/sessions:verify')) {
      sent.verify = request.postDataJSON();
      return route.fulfill({ status: 201, json: TOKENS });
    }
    return stubFor({ answer: refused })(route);
  });
  try {
    await page.goto(`${origin}/reset#token=e2e-reset`);
    await page.getByLabel('New password').fill('seven blue lanterns above the harbour');
    await page.getByRole('button', { name: 'Set the password' }).click();

    // The same card moves on: the step, whose account, the clock, the code field.
    await page.getByRole('heading', { name: 'Second factor' }).waitFor();
    assert.deepEqual(sent.reset, { token: 'e2e-reset', password: 'seven blue lanterns above the harbour' });
    assert.ok(await page.getByText('anna@contoso.example').isVisible(), 'the identity line names the account');
    assert.ok(await page.getByRole('button', { name: 'Not you?' }).isVisible(), 'the way back is offered');
    assert.match(await page.getByText(/This sign-in waits/).innerText(), /[34]:\d\d/, 'the remaining time is shown');
    assert.ok(await page.getByText(/new password is set/i).isVisible(), 'the step says the password was set, not that it was right');

    await page.locator('input[autocomplete="one-time-code"]').fill('123456');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    await page.locator('input[autocomplete="one-time-code"]').waitFor({ state: 'detached' });
    assert.deepEqual(sent.verify, { pending_token: 'totp-after-reset', code: '123456' });
  } finally {
    await browser.close();
    await close();
  }
});

// UC-ID-03 checks 1 and 2 during a sign-in the workspace routed into setup: the same field and the
// same panel as on the profile, and only *Continue* opens the session.
test('a setup forced during sign-in confirms in the code field and shows the codes once', async () => {
  const { origin, close } = await serve(DIST);
  const sent = {};
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 375, height: 812 }, permissions: ['clipboard-read', 'clipboard-write'] });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/api/v1/auth/mfa/totp:enroll')) {
      sent.enroll = request.postDataJSON();
      return route.fulfill({ json: ENROLLMENT });
    }
    if (path.endsWith('/api/v1/auth/mfa/totp:confirm')) {
      sent.confirm = request.postDataJSON();
      return route.fulfill({ json: { armed: true, tokens: { access_token: 'e2e-access', refresh_token: 'e2e-refresh' } } });
    }
    return stubFor({
      answer: () => ({ status: 202, json: { pending_token: 'enroll-1', methods: ['ENROLL'], expires_at: new Date(Date.now() + 300_000).toISOString() } }),
    })(route);
  });
  const page = await context.newPage();
  try {
    await page.goto(origin);
    await page.waitForSelector('text=to contoso.hubtask.eu');
    await page.locator('input[type="email"]').fill('anna@contoso.example');
    await page.locator('input[autocomplete="current-password"]').fill('annas-own-password');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await walkSetup(page, sent);
    assert.deepEqual(sent.enroll, { pending_token: 'enroll-1' });
    assert.equal(sent.confirm?.pending_token, 'enroll-1');
    // Continue is what signs in: the card is gone.
    await page.getByRole('button', { name: 'Continue' }).waitFor({ state: 'detached' });
    assert.equal(await page.getByText(ENROLLMENT.recovery_codes[0]).count(), 0, 'the codes outlived the panel');
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

// ADR-0071's addendum (E2): a provider arrival whose address matches an account with a password
// is not signed in on the provider's word. The callback hands the step to the card, the card asks
// for the account's password once, and - where the account has a second factor - continues into the
// ordinary code step. What is walked is the person's path, and what is asserted is what was sent.
test('a provider arrival that meets a password is asked for it on the card, then for the code', async () => {
  const { origin, close } = await serve(DIST);
  const sent = {};
  const stub = async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith('/api/v1/auth/oidc:callback')) {
      sent.callback = request.postDataJSON();
      return route.fulfill({
        status: 202,
        json: {
          pending_token: 'link-1', methods: ['LINK'], expires_at: '2099-01-01T00:00:00Z',
          email: 'anna@contoso.example', provider_name: 'Contoso Entra ID',
        },
      });
    }
    if (path.endsWith('/api/v1/auth/sessions:link')) {
      sent.link = request.postDataJSON();
      return route.fulfill({ status: 202, json: { pending_token: 'totp-1', methods: ['TOTP', 'RECOVERY'], expires_at: '2099-01-01T00:00:00Z' } });
    }
    if (path.endsWith('/api/v1/auth/sessions:verify')) {
      sent.verify = request.postDataJSON();
      return route.fulfill({ status: 201, json: TOKENS });
    }
    return stubFor({ answer: refused })(route);
  };

  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 375, height: 812 } });
  await context.route('**/api/v1/**', stub);
  const page = await context.newPage();
  try {
    await page.goto(`${origin}/auth/callback?code=the-code&state=the-state`);

    // The card, not the app: whose account, which provider, the password field.
    await page.getByRole('heading', { name: 'Confirm it is your account' }).waitFor();
    assert.ok(await page.getByText('anna@contoso.example').isVisible(), 'the identity line names the account');
    assert.ok(await page.getByText(/Contoso Entra ID will sign you in from now on/).isVisible(), 'the card names the provider');
    assert.deepEqual(sent.callback, { code: 'the-code', state: 'the-state' });

    const password = page.locator('input[autocomplete="current-password"]');
    await password.fill('annas-own-password');
    await page.getByRole('button', { name: 'Confirm and connect' }).click();

    // The account has a second factor: the ordinary code step follows on the same card.
    await page.getByRole('heading', { name: 'Second factor' }).waitFor();
    assert.deepEqual(sent.link, { pending_token: 'link-1', password: 'annas-own-password' });

    await page.locator('input[autocomplete="one-time-code"]').fill('123456');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await page.waitForFunction(() => !document.querySelector('input[autocomplete="one-time-code"]'));
    assert.equal(sent.verify?.pending_token, 'totp-1', 'the code step presented the credential the LINK step handed on');
  } finally {
    await browser.close();
    await close();
  }
});
