// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workspace's sign-in rules, walked as an administrator meets them (UC-ID-12, SC-06): every rule
// says where its value came from and whether it is locked, choices looser than the installation
// allows are not offered, a refusal lands at the rule it is about, and a rule that cannot be read is
// a sentence and a retry rather than a spinner.
//
// Chromium only: what is asserted is behaviour and text, not engine-specific layout.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

/** One rule as the contract answers it. */
const rule = (value, installation, { lock = null, source = 'DEFAULT', above = 'DEFAULT' } = {}) =>
  ({ value, installation, lock, source, installation_source: above });

/** A workspace that tightened one rule, sits under one installation lock and one requirement. */
function policy() {
  return {
    password: {
      min_length: rule(16, 12, { source: 'WORKSPACE', above: 'INSTANCE' }),
      min_lowercase: rule(0, 0), min_uppercase: rule(0, 0), min_digits: rule(0, 0), min_symbols: rule(0, 0),
      min_classes: rule(0, 0), max_repeat: rule(0, 0),
      common_passwords: rule(true, true), context_words: rule(true, true),
      breach_check: rule(false, false),
      max_age_days: rule(0, 0),
      history_count: rule(5, 5, { lock: 'INSTANCE', source: 'INSTANCE', above: 'INSTANCE' }),
      min_age_hours: rule(0, 0),
    },
    mfa_required_for: rule('ADMINS', 'ADMINS', { source: 'INSTANCE', above: 'INSTANCE' }),
    methods: rule(['PASSWORD', 'OIDC'], ['PASSWORD', 'OIDC']),
    session: { max_days: rule(30, 30), idle_minutes: rule(30, 30, { source: 'INSTANCE', above: 'INSTANCE' }) },
    legal: {
      imprint_url: rule('', ''), privacy_url: rule('', ''), terms_url: rule('', ''), accessibility_url: rule('', ''),
    },
    rotation_from: null,
  };
}

const WORKSPACE = {
  id: 'w1', slug: 'acme', display_name: 'Acme', status: 'ACTIVE', default_locale: 'en',
  default_time_zone: 'Europe/Berlin', require_admin_totp: true, created_at: '2026-09-01T00:00:00Z', version: 3,
};

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, answer) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    // `answer` hands back the fulfilment it started, or nothing where the route is not its own.
    const handled = answer(route, path);
    if (handled) return handled;
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  return { page, close: () => context.close() };
}

/** The origin line under a control, found by the control's label. */
async function originOf(page, label) {
  return page.getByLabel(label, { exact: true }).evaluate((input) => {
    const row = input.closest('[data-rule]');
    return row?.querySelector('[data-origin]')?.textContent?.trim() ?? '';
  });
}

test('chromium: every rule says where it comes from, and looser choices are not offered', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, (route, path) =>
    path === '/tenant' ? route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy() } }) : undefined);
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  await page.getByLabel('Minimum length', { exact: true }).waitFor();

  // Set here, and the installation's value beside it because the installation decided one.
  const own = await originOf(page, 'Minimum length');
  assert.match(own, /Set here/, own);
  assert.match(own, /Installation: 12/, own);

  // Locked above: said, and the control is not usable.
  const locked = await originOf(page, 'Remember previous passwords');
  assert.match(locked, /Set by the installation/, locked);
  assert.equal(await page.getByLabel('Remember previous passwords', { exact: true }).isDisabled(), true);

  // Nobody decided: Hubtask's default, and no installation column - the private installation's case.
  const plain = await originOf(page, 'Check against known breaches');
  assert.match(plain, /Hubtask.s default/, plain);
  assert.doesNotMatch(plain, /Installation/, plain);

  // The installation requires a factor of administrators: "Nobody" is not a choice here.
  const options = await page.getByLabel('Required of', { exact: true }).locator('option').allTextContents();
  assert.ok(!options.includes('Nobody'), `a looser choice is offered: ${options.join(', ')}`);
  assert.ok(options.includes('Everyone'), options.join(', '));

  // A number below the installation's is not offered either.
  assert.equal(await page.getByLabel('Minimum length', { exact: true }).getAttribute('min'), '12');

  // The installation ends idle sessions after thirty minutes: "off" (an empty field) is not a choice,
  // and neither is a longer time.
  // By prefix: a required field's name carries its marker.
  const idle = page.getByLabel(/^End an idle session after, in minutes/);
  assert.equal(await idle.getAttribute('required'), '', 'an idle bound the installation set can be emptied');
  assert.equal(await idle.getAttribute('max'), '30');

  // The password's own way in says where it comes from too.
  const way = await page.locator('ul.ways li').first().locator('[data-origin]').textContent();
  assert.match(way ?? '', /Hubtask.s default/, way ?? '');

  // The eighteenth rule has its control.
  assert.ok(await page.getByLabel('Earliest change after, in hours', { exact: true }).isVisible());
});

test('chromium: a refusal is shown at the rule it is about', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, (route, path) => {
    if (path !== '/tenant') return undefined;
    if (route.request().method() === 'PATCH') {
      return route.fulfill({
        status: 422,
        json: {
          code: 'errors.validation', detail_code: 'auth.policy_loosens', status: 422, request_id: 'req_x',
          field_errors: [{ path: '/sign_in_policy/session_max_days', code: 'auth.policy_loosens' }],
        },
      });
    }
    return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy() } });
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  const field = page.getByLabel('End a session after, in days', { exact: true });
  await field.waitFor();
  await field.fill('20');
  await page.getByRole('button', { name: 'Save', exact: true }).click();

  await page.waitForFunction(() => document.querySelector('[aria-invalid="true"]') !== null);
  assert.equal(await field.getAttribute('aria-invalid'), 'true', 'the refusal is not at the rule');
});

test('chromium: a rule that cannot be read is a sentence and a retry', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  // Refused until the reader asks again: what is asserted is that the screen offers the asking and
  // that asking is what reads the rule again. The stream is held open and silent, because a stream
  // that reconnected would re-read everything on its own and decide the race instead of the click.
  let mended = false;
  let reads = 0;
  const { page, close } = await open(browser, (route, path) => {
    if (path === '/stream') return new Promise(() => {});
    if (path !== '/tenant') return undefined;
    reads += 1;
    if (!mended) return route.fulfill({ status: 500, json: { code: 'errors.internal', status: 500, request_id: 'req_y' } });
    return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy() } });
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  const retry = page.getByRole('button', { name: 'Try again' });
  await retry.waitFor({ timeout: 15_000 });
  assert.equal(await page.locator('[aria-busy="true"]').count(), 0, 'a failed read is still drawn as loading');
  // The sentence holds while the engine keeps trying behind it - it is not traded for a spinner.
  await page.waitForTimeout(1500);
  assert.ok(await retry.isVisible(), 'the sentence and its retry did not hold');
  const before = reads;
  mended = true;
  // By the keyboard: the button is gone the moment the read succeeds, which a pointer click's
  // own bookkeeping reads as a click that never landed.
  await retry.focus();
  await page.keyboard.press('Enter');
  await page.getByLabel('Minimum length', { exact: true }).waitFor();
  assert.ok(reads > before, 'asking again read nothing');
});

/** A provider as the listing answers it. */
const provider = (id, name, scope, { enabled = true, offered = enabled } = {}) => ({
  id, scope, issuer: `https://${id}.example`, client_id: 'hubtask', display_name: name, kind: 'GENERIC',
  provisioning: 'INVITED_ONLY', position: 0, enabled, offered_here: offered,
  allowed_email_domains: [], allowed_directories: [], created_at: '2026-09-01T00:00:00Z', version: 1,
});

// UC-ID-12 check 6 and UC-ID-11 check 8: every way in - the password, the workspace's own providers
// and the ones the installation offers - is one row with one switch, in one list; the last way in
// that is on cannot be switched off.
test('chromium: the ways to sign in are one list, one switch each, and the last cannot go', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const sent = [];
  const onlyProvider = policy();
  onlyProvider.methods = rule(['OIDC'], ['PASSWORD', 'OIDC'], { source: 'WORKSPACE' });
  const { page, close } = await open(browser, (route, path) => {
    const request = route.request();
    if (path === '/tenant' && request.method() === 'GET') {
      return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: onlyProvider } });
    }
    if (path === '/identity-providers') {
      return route.fulfill({ json: [
        provider('own', 'Contoso Entra ID', 'workspace'),
        provider('platform', 'The platform', 'installation', { enabled: true, offered: false }),
      ] });
    }
    if (path.endsWith(':offer')) {
      sent.push({ path, body: request.postDataJSON() });
      return route.fulfill({ json: provider('platform', 'The platform', 'installation', { offered: true }) });
    }
    return undefined;
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  const list = page.getByRole('list', { name: 'Ways to sign in' });
  await list.waitFor();
  const rows = (await list.getByRole('listitem').allTextContents()).map((row) => row.replace(/\s+/g, ' ').trim());
  assert.equal(rows.length, 3, rows.join(' | '));
  assert.match(rows[0], /Password/);
  assert.equal(await list.getByRole('switch').count() + await list.locator('input[type="checkbox"]').count() > 0, true);

  // The password is off and one provider is on: that provider is the last way in.
  const own = list.getByRole('listitem').filter({ hasText: 'Contoso Entra ID' }).locator('input');
  assert.equal(await own.isDisabled(), true, 'the last way in can be switched off');
  assert.match(rows[1], /only way in/i, rows[1]);

  // Taking the installation's provider is the one verb, from this list.
  await list.getByRole('listitem').filter({ hasText: 'The platform' }).locator('input').check();
  await page.waitForFunction(() => true);
  await new Promise((resolve) => setTimeout(resolve, 300));
  assert.deepEqual(sent, [{ path: '/identity-providers/platform:offer', body: { offered: true } }]);
});

// ADR-0076 §2, UC-ID-12 check 6 (SC-20): a provider the installation withdraws says its day under
// its switch while there is time to act on it, and from the day it has no switch at all.
test('chromium: a provider the installation withdraws says when, under its switch', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const soon = new Date(Date.now() + 5 * 86_400_000).toISOString();
  const { page, close } = await open(browser, (route, path) => {
    if (path === '/tenant') return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy() } });
    if (path === '/identity-providers') {
      return route.fulfill({ json: [
        { ...provider('platform', 'The platform', 'installation', { offered: true }), withdraw_at: soon },
        { ...provider('gone', 'The old platform', 'installation', { offered: false }), withdraw_at: '2026-01-01T00:00:00Z' },
      ] });
    }
    return undefined;
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  const list = page.getByRole('list', { name: 'Ways to sign in' });
  await list.waitFor();
  const withdrawing = list.getByRole('listitem').filter({ hasText: 'The platform' });
  await withdrawing.getByText(/The installation withdraws this provider on .*Switch on another way/).waitFor();
  assert.equal(await withdrawing.locator('input').count(), 1, 'a provider still offered lost its switch');

  // From its day it is a way in nowhere: no switch, a sentence.
  const gone = list.getByRole('listitem').filter({ hasText: 'The old platform' });
  assert.equal(await gone.locator('input').count(), 0, 'a withdrawn provider still has a switch');
  assert.match(await gone.textContent() ?? '', /Switched off by the installation/);
});

// ADR-0076 §4 (SC-20): a workspace the withdrawal left with no way in is told that the password opened
// again, from the same answer the sign-in card reads - in a sentence that names no cause, because the
// server opens the password whatever left the workspace without a way in (SC-31, E2).
test('chromium: a workspace left without a way in is told the password is open again', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const onlyProvider = policy();
  onlyProvider.methods = rule(['OIDC'], ['PASSWORD', 'OIDC'], { source: 'WORKSPACE' });
  const { page, close } = await open(browser, (route, path) => {
    if (path === '/tenant') return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: onlyProvider } });
    if (path === '/identity-providers') {
      return route.fulfill({ json: [
        { ...provider('gone', 'The old platform', 'installation', { offered: false }), withdraw_at: '2026-01-01T00:00:00Z' },
      ] });
    }
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, sign_in_rules: true } } });
    }
    if (path === '/auth/sign-in-rules') {
      return route.fulfill({ json: {
        workspace_host: 'acme.hubtask.eu', methods: ['PASSWORD'], providers: [], legal: {},
        password: {
          min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
          min_classes: 0, max_repeat: 0, common_passwords: true, context_words: true,
          breach_check: false, history_count: 0, not_current: false,
        },
        password_fallback: true,
      } });
    }
    return undefined;
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  await page.getByText('The password is open again').waitFor();
  const said = await page.getByText(/until you switch on a way in/i).textContent() ?? '';
  assert.doesNotMatch(said, /withdr|invitation/i, 'the sentence names one cause of many');
});

// P-04, ADR-0078 §1 (SC-33): before the password goes off, its row says how many people here no
// provider switched on here signs in - the server's number - and says nothing once it is off, or
// where nobody is without one.
test('chromium: the password switch says how many people have no provider here before it goes off', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  let count = 3;
  let asked = 0;
  const { page, close } = await open(browser, (route, path) => {
    if (path === '/tenant') return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy() } });
    if (path === '/identity-providers') {
      return route.fulfill({ json: [provider('own', 'Contoso Entra ID', 'workspace')] });
    }
    if (path === '/tenant/accounts-without-provider') {
      asked += 1;
      return route.fulfill({ json: { count } });
    }
    return undefined;
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  const list = page.getByRole('list', { name: 'Ways to sign in' });
  await list.waitFor();
  const password = list.getByRole('listitem').filter({ hasText: 'Password' }).first();
  await password.getByText(/3 people here have no provider switched on here to sign in with/).waitFor();
  assert.match(await password.locator('[data-reach]').textContent() ?? '', /Forgot your password\?/);
  assert.ok(asked >= 1, 'the count was never asked for');

  // One person: the singular, from the catalogue's plural.
  count = 1;
  await page.reload();
  await list.waitFor();
  await list.getByText(/1 person here has no provider/).waitFor();

  // Nobody without a provider: nothing to say.
  count = 0;
  await page.reload();
  await list.waitFor();
  await list.getByRole('listitem').filter({ hasText: 'Contoso Entra ID' }).waitFor();
  assert.equal(await list.locator('[data-reach]').count(), 0, 'a count of nobody was said');
});

// The password already off: the cost is past, and the row says nothing of it.
test('chromium: a switched-off password says no count', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const onlyProvider = policy();
  onlyProvider.methods = rule(['OIDC'], ['PASSWORD', 'OIDC'], { source: 'WORKSPACE' });
  const { page, close } = await open(browser, (route, path) => {
    if (path === '/tenant') return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: onlyProvider } });
    if (path === '/identity-providers') {
      return route.fulfill({ json: [provider('own', 'Contoso Entra ID', 'workspace')] });
    }
    if (path === '/tenant/accounts-without-provider') return route.fulfill({ json: { count: 3 } });
    return undefined;
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  const list = page.getByRole('list', { name: 'Ways to sign in' });
  await list.getByRole('listitem').filter({ hasText: 'Contoso Entra ID' }).waitFor();
  assert.equal(await list.locator('[data-reach]').count(), 0, 'a switched-off password still says the count');
});

// UC-ID-11 check 8: the provider screen configures; it switches nothing - not with a control, and
// not with the deprecated field in the body it saves (ADR-0076 §5).
test('chromium: the provider screen has no switch of its own', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const saved = [];
  const { page, close } = await open(browser, (route, path) => {
    const request = route.request();
    if (path === '/identity-providers/own' && request.method() === 'PUT') {
      saved.push(request.postDataJSON());
      return route.fulfill({ json: provider('own', 'Contoso Entra ID', 'workspace') });
    }
    if (path === '/identity-providers') {
      return route.fulfill({ json: [
        provider('own', 'Contoso Entra ID', 'workspace'),
        provider('platform', 'The platform', 'installation', { offered: false }),
      ] });
    }
    return undefined;
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/identity-provider`);
  await page.getByText('Contoso Entra ID').first().waitFor();
  await page.getByRole('button', { name: 'Change' }).first().click();
  assert.equal(await page.locator('main input[type="checkbox"], main [role="switch"]').count(), 0,
    'a provider is still switched on its own screen');

  await page.getByRole('button', { name: 'Save the provider' }).click();
  await page.getByText('Saved. The provider answered its metadata.').waitFor();
  assert.equal(saved.length, 1, 'the form was not saved');
  assert.equal('enabled' in saved[0], false, `the form still sends the switch: ${JSON.stringify(saved[0])}`);
});

// UC-ID-12 check 9: the screen reads in German, with no rule falling back to English.
test('chromium: the sign-in rules read in German', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, close } = await open(browser, (route, path) => {
    if (path === '/accounts/me') return route.fulfill({ json: { ...ACCOUNT, locale: 'de' } });
    if (path === '/tenant') return route.fulfill({ json: { ...WORKSPACE, sign_in_policy: policy() } });
    if (path === '/identity-providers') return route.fulfill({ json: [provider('own', 'Contoso Entra ID', 'workspace')] });
    return undefined;
  });
  t.after(close);

  await page.goto(`${served.origin}/administration/sign-in`);
  await page.getByLabel('Mindestlänge', { exact: true }).waitFor({ timeout: 15_000 });
  const text = await page.locator('main').innerText();
  for (const german of ['Wege der Anmeldung', 'Hier festgelegt.', 'Voreinstellung von Hubtask.', 'Frühestens änderbar nach, in Stunden', 'Verlangt von']) {
    assert.ok(text.includes(german), `"${german}" is missing`);
  }
  for (const english of ['Minimum length', 'Ways to sign in', 'Set here.', "Hubtask's default", 'Required of', 'Sessions', 'Imprint', 'Where you are in the administration']) {
    assert.ok(!text.includes(english), `"${english}" fell back to English`);
  }
});
