// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The first start's setup (UC-INS-01 checks 2, 3, 5, 6), walked as a person meets it: a fresh
// installation shows *Set up Hubtask* in place of the sign-in card, a wrong code is refused at the
// code's field, single mode asks for no short name and multi mode does, and a setup that is
// accepted signs the owner in. An installation that is set up shows the sign-in card.
//
// Against a stubbed API: what is asserted is what the card sends and what it shows. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** The installation's own rules, which is what a host with no workspace answers. */
const RULES = {
  methods: ['PASSWORD'], providers: [],
  password: {
    min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
    breach_check: false, history_count: 0, not_current: false,
  },
  legal: {},
};

const TOKENS = {
  token_type: 'Bearer', access_token: 'e2e-access', refresh_token: 'e2e-refresh',
  access_token_expires_at: '2099-01-01T00:00:00Z', refresh_token_expires_at: '2099-01-01T00:00:00Z',
  session: { id: 's1', created_at: '2026-10-12T00:00:00Z', current: true },
};

const wrongCode = {
  status: 403,
  json: {
    code: 'forbidden', detail_code: 'setup.code_wrong', status: 403, request_id: 'req_s',
    field_errors: [{ path: '/code', code: 'setup.code_wrong' }],
  },
};

async function open(browser, { tenancy = 'single', setupOpen = true, viewport = { width: 1280, height: 900 } } = {}) {
  const context = await browser.newContext({ viewport });
  const sent = [];
  let signedIn = false;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities' && !signedIn) {
      return route.fulfill({
        json: { ...MANIFEST, tenancy_mode: tenancy, features: { ...MANIFEST.features, sign_in_rules: true, setup_open: setupOpen } },
      });
    }
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: RULES });
    if (path === '/auth/password:check') return route.fulfill({ status: 401, json: { code: 'unauthenticated', status: 401 } });
    if (path === '/auth/installation:set-up') {
      const body = request.postDataJSON();
      sent.push(body);
      if (body.code !== '7K3M-Q9TX-2WHR') return route.fulfill(wrongCode);
      signedIn = true;
      return route.fulfill({ status: 201, json: TOKENS });
    }
    return stub(route);
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/`);
  return { page, sent, close: () => context.close() };
}

/** A picture for the evidence, where a walk asked for one (HUBTASK_EVIDENCE_DIR). */
async function shot(page, name) {
  if (process.env.HUBTASK_EVIDENCE_DIR) {
    await page.screenshot({ path: join(process.env.HUBTASK_EVIDENCE_DIR, name), fullPage: true });
  }
}

async function fill(page, { code, slug } = {}) {
  await page.getByLabel('Setup code').fill(code ?? '7K3M-Q9TX-2WHR');
  await page.getByLabel('Name of the workspace').fill('Family Winkel');
  if (slug !== undefined) await page.getByLabel('Short name').fill(slug);
  await page.getByLabel('Your name').fill('Jérôme');
  await page.getByLabel('Email').fill('jerome@example.eu');
  await page.getByLabel('Your password').fill('seven blue lanterns above the harbour');
}

test('chromium: a fresh installation shows the setup card, refuses a wrong code at its field, and signs the owner in', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser);
  t.after(close);

  await page.getByRole('heading', { name: 'Set up Hubtask' }).waitFor();
  assert.equal(await page.getByRole('heading', { name: 'Sign in' }).count(), 0, 'the sign-in card is drawn beside it');
  // Single mode asks for no short name (check 5).
  assert.equal(await page.getByLabel('Short name').count(), 0);
  assert.match(await page.locator('main').textContent() ?? '', /console output when it started/);

  await shot(page, 'setup-desktop.png');

  await fill(page, { code: 'AAAA-BBBB-CCCC' });
  await page.getByRole('button', { name: 'Create' }).click();
  await page.getByText('That setup code is not right.').first().waitFor();
  assert.equal(await page.getByRole('heading', { name: 'Set up Hubtask' }).count(), 1, 'a refusal left the card');
  assert.equal(await page.getByLabel('Name of the workspace').inputValue(), 'Family Winkel', 'a refusal emptied the form');
  await shot(page, 'setup-refused-desktop.png');

  await page.getByLabel('Setup code').fill('7K3M-Q9TX-2WHR');
  await page.getByRole('button', { name: 'Create' }).click();
  await page.getByRole('heading', { name: 'Set up Hubtask' }).waitFor({ state: 'hidden' });

  const accepted = sent.at(-1);
  assert.equal(accepted.code, '7K3M-Q9TX-2WHR');
  assert.equal(accepted.display_name, 'Family Winkel');
  assert.equal(accepted.owner_email, 'jerome@example.eu');
  assert.equal(accepted.owner_display_name, 'Jérôme');
  assert.equal('slug' in accepted, false, 'single mode sent a short name');
  // Signed in (check 6): the pair is held where every other sign-in holds it.
  assert.equal(await page.evaluate(() => sessionStorage.getItem('hubtask.bearer')), 'e2e-access');
});

test('chromium: multi mode asks for the short name and says the address it becomes', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, sent, close } = await open(browser, { tenancy: 'multi' });
  t.after(close);

  await page.getByRole('heading', { name: 'Set up Hubtask' }).waitFor();
  await fill(page, { slug: 'winkel' });
  assert.match(await page.locator('main').textContent() ?? '', /The workspace will be at winkel\./);
  await shot(page, 'setup-multi-desktop.png');
  await page.getByRole('button', { name: 'Create' }).click();
  await page.getByRole('heading', { name: 'Set up Hubtask' }).waitFor({ state: 'hidden' });
  assert.equal(sent.at(-1).slug, 'winkel');
});

test('chromium: the card at 375 px', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { viewport: { width: 375, height: 812 } });
  t.after(close);

  await page.getByRole('heading', { name: 'Set up Hubtask' }).waitFor();
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.equal(overflow, 0, 'the card scrolls sideways at 375 px');
  await shot(page, 'setup-375.png');
});

test('chromium: an installation that is set up shows the sign-in card', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, { setupOpen: false });
  t.after(close);

  await page.getByLabel('Email').waitFor();
  assert.equal(await page.getByRole('heading', { name: 'Set up Hubtask' }).count(), 0);
});
