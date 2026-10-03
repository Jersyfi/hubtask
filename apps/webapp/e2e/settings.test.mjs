// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Your settings as a section (ADR-0065 decision 3), walked where the finding was made: a pinned
// column beside eight screens, two lists that are tables, and rows whose switches line up.
//
// The alignment is **measured**, not read off the DOM. "Each row draws its switches where its own
// text ends" and "every row draws them in the same two places" produce the same markup and
// different pictures, and the picture is what the owner was looking at: the longest category -
// *I am invited to the workspace* - pushed its switches out of line with every other row.
//
// Chromium only: what this asserts is layout, and the layout has no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, MANIFEST, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { ENROLLMENT, walkSetup } from './secondfactor.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

/** Seven categories on one channel: the list a real installation answers, longest name included. */
const CATEGORIES = ['ASSIGNMENT', 'MEMBERSHIP', 'COMMENT', 'INVITATION', 'REMINDER', 'INTEGRATION', 'RETENTION'];

/** Three sessions and three devices: enough that a reader has to scan rather than read. */
const SESSIONS = [
  { id: 's-1', user_agent: 'Firefox on Linux', created_at: '2026-09-01T08:00:00Z', last_used_at: '2026-09-20T08:00:00Z', ip_class: '10.0.0.0/24', current: false },
  { id: 's-2', user_agent: 'Safari on iPhone', created_at: '2026-09-22T08:00:00Z', last_used_at: null, ip_class: null, current: false },
  { id: 's-3', user_agent: 'Chrome on macOS', created_at: '2026-09-10T08:00:00Z', last_used_at: '2026-09-23T08:00:00Z', ip_class: '10.0.1.0/24', current: true },
];
const DEVICES = [
  { id: 'd-1', display_name: 'Work laptop', platform: 'web', last_seen_at: '2026-09-23T07:00:00Z', blocked: false },
  { id: 'd-2', display_name: 'Old phone', platform: 'ios', last_seen_at: '2026-08-01T07:00:00Z', blocked: true },
];

async function answer(route) {
  const url = new URL(route.request().url());
  const path = url.pathname.replace(/^.*\/api\/v1/, '');
  if (path === '/meta/capabilities') {
    return route.fulfill({ json: { ...MANIFEST, notification_categories: CATEGORIES, notification_channels: ['EMAIL'] } });
  }
  if (path === '/auth/sessions') return route.fulfill({ json: SESSIONS });
  if (path === '/sync/devices') return route.fulfill({ json: DEVICES });
  if (path === `/accounts/${ACCOUNT.id}/notification-preferences`) {
    return route.fulfill({ json: { data: CATEGORIES.map((category) => ({ category, channel: 'EMAIL', enabled: true, include_title: false, is_default: true })) } });
  }
  return stub(route);
}

const served = await serve(DIST);
test.after(() => served.close());

async function open(browser, width = 1280) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  await context.route('**/api/v1/**', answer);
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(`${new URL(page.url()).pathname}: ${error}`));
  return { page, failures, close: () => context.close() };
}

/** The section's screens, and the word each puts at the end of its trail. */
const SECTION = [
  ['/profile', 'How the product speaks to you'],
  ['/profile/appearance', 'On this device'],
  ['/profile/notifications', 'What you are told about'],
  // Renamed with the sign-in work (SI-15): the screen holds the password, the recovery codes and
  // the second factor, and was named after the second of the three.
  ['/profile/security', 'Password and sign-in'],
  ['/profile/sessions', 'Where you are signed in'],
  ['/profile/devices', 'Devices that synchronise'],
  ['/profile/apps', 'Apps you have allowed'],
  ['/profile/tokens', 'Access tokens'],
];

test('chromium: 1280 px — the settings are a section, and every screen says where it is', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/profile`);
  const section = page.getByRole('navigation', { name: 'Your settings', exact: true });
  await section.waitFor({ timeout: 15_000 });

  // The section's own list replaces the workspace's tree rather than joining it.
  assert.equal(await page.getByRole('navigation', { name: 'Workspace' }).count(), 0, 'the workspace tree is drawn inside the section');
  const rows = (await section.getByRole('treeitem').allTextContents()).map((row) => row.trim());
  assert.equal(rows[0], 'The workspace', `the first row is ${JSON.stringify(rows[0])}`);
  assert.equal(rows.length, 9, `the section holds ${rows.length} rows`);

  const untrailed = [];
  const unheaded = [];
  const unmarked = [];
  for (const [route, word] of SECTION) {
    await page.goto(`${served.origin}${route}`);
    const trail = page.getByRole('navigation', { name: 'Where you are in your settings' });
    const shown = await trail.textContent({ timeout: 4_000 }).catch(() => undefined);
    if (!shown || !shown.includes('Your settings') || !shown.includes(word)) untrailed.push(route);
    // One `h1` per screen, whether drawn or read: two headings is two answers to what a page is.
    if ((await page.locator('main h1').count()) !== 1) unheaded.push(route);
    // And the column says where the reader is, on every one of them.
    if ((await section.getByRole('treeitem', { selected: true }).count()) !== 1) unmarked.push(route);
  }

  assert.deepEqual(untrailed, [], `these screens do not say where they are: ${untrailed.join(', ')}`);
  assert.deepEqual(unheaded, [], `these screens do not have exactly one heading: ${unheaded.join(', ')}`);
  assert.deepEqual(unmarked, [], `the column marks no row on these: ${unmarked.join(', ')}`);

  // The way out, which is what a section somebody cannot leave would be missing.
  await section.getByRole('treeitem', { name: 'The workspace', exact: true }).click();
  await page.waitForFunction(() => location.pathname === '/', null, { timeout: 5_000 });

  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — what you are told about is a grid, and every row’s switches line up', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/profile/notifications`);
  const table = page.getByRole('table', { name: 'What you are told about' });
  await table.waitFor({ timeout: 15_000 });
  assert.equal(await table.getByRole('row').count(), CATEGORIES.length + 1, 'a row per category and one for the headings');

  // The finding, measured: every "Tell me" sits at one x, and so does every "Name the entry".
  const columns = await table.locator('tbody tr').evaluateAll((rows) =>
    rows.map((row) => [...row.querySelectorAll('input[type="checkbox"]')].map((box) => Math.round(box.getBoundingClientRect().x))),
  );
  assert.equal(columns.length, CATEGORIES.length);
  const first = columns[0];
  for (const [index, row] of columns.entries()) {
    assert.deepEqual(row, first, `row ${index} draws its switches at ${row}, not at ${first}`);
  }

  // The one category nobody can switch off says why, rather than being absent or silently on.
  const invitation = table.getByRole('row').filter({ hasText: 'I am invited to the workspace' });
  assert.equal(await invitation.locator('input[type="checkbox"]:disabled').count(), 1, 'the always-on row is switchable');

  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — the two lists are tables, this device first and the newest after it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await open(browser);
  t.after(close);

  await page.goto(`${served.origin}/profile/sessions`);
  const sessions = page.getByRole('table', { name: 'Where you are signed in' });
  await sessions.waitFor({ timeout: 15_000 });
  const clients = await sessions.locator('tbody th').allTextContents();
  // This device first, then the newest of the rest: the row somebody looks for is the one that
  // appeared while they were not looking.
  assert.deepEqual(
    clients.map((row) => row.trim().replace(/\s+/g, ' ')),
    ['Chrome on macOS This device', 'Safari on iPhone', 'Firefox on Linux'],
  );
  // A column per fact, so a reader scans down rather than reading every row to its end.
  assert.deepEqual(
    (await sessions.locator('thead th').allTextContents()).map((head) => head.trim()),
    ['Client', 'Signed in', 'Signed in with', 'Last active', 'Network', 'End'],
  );

  await page.goto(`${served.origin}/profile/devices`);
  const devices = page.getByRole('table', { name: 'Devices that synchronise' });
  await devices.waitFor({ timeout: 10_000 });
  assert.equal(await devices.locator('tbody tr').count(), DEVICES.length);
  // A forgotten device is blocked rather than erased (N-03), and the row says which it is.
  assert.equal(await devices.getByRole('row').filter({ hasText: 'Old phone' }).getByText('Forgotten').count(), 1);

  assert.deepEqual(failures, []);
});

// UC-ID-03 checks 5 and 6: where the workspace requires a second factor of this person, the profile
// says so and offers no way to turn it off; where nothing requires it, turning it off is offered.
test('chromium: 1280 px — the profile does not offer to turn off a factor the workspace requires', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const required of [true, false]) {
    const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    await context.route('**/api/v1/**', async (route) => {
      const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
      if (path === '/accounts/me') {
        return route.fulfill({ json: { ...ACCOUNT, has_second_factor: true, recovery_codes_remaining: 8, second_factor_required: required } });
      }
      return answer(route);
    });
    await context.addInitScript(() => {
      sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
      sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
    });
    const page = await context.newPage();
    await page.goto(`${served.origin}/profile/security`);
    await page.getByText('Left: 8 of 10.').waitFor();
    const offered = await page.getByText('Turn the second factor off').count();
    const said = await page.getByText('Your workspace requires a second factor', { exact: false }).count();
    if (required) {
      assert.equal(offered, 0, 'turning off is offered although the workspace requires the factor');
      assert.ok(said > 0, 'the profile does not say why there is no way to turn it off');
    } else {
      assert.equal(offered, 1, 'turning off is not offered although nothing requires the factor');
      assert.equal(said, 0);
    }
    await context.close();
  }
});

// UC-ID-12 check 1: who needs a second factor is set in one place, the sign-in screen. The
// Workspace screen had its own switch for it, stored apart from the rule and able to disagree.
test('chromium: 1280 px — the workspace screen has no second-factor switch of its own', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/tenant') {
      return route.fulfill({ json: { id: 'w1', slug: 'acme', display_name: 'Acme', status: 'ACTIVE', default_locale: 'en', default_time_zone: 'Europe/Berlin', require_admin_totp: true, created_at: '2026-09-01T00:00:00Z', version: 3 } });
    }
    return answer(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/administration/workspace`);
  await page.waitForFunction(() => [...document.querySelectorAll('input')].some((input) => input.value === 'Acme'), null, { timeout: 15_000 });
  assert.equal(await page.getByText('Administrators need a second factor').count(), 0, 'the second place for the rule is still there');
  assert.equal(await page.getByRole('switch').count() + await page.locator('input[type="checkbox"]').count(), 0, 'a switch is left on the workspace screen');
});

// UC-ID-03 checks 1 and 2 on the profile: the same walk the forced setup during sign-in takes, and
// *Continue* ends it with the factor on and the codes gone from the screen.
test('chromium: 375 px — setting up a second factor on the profile confirms in the code field and shows the codes once', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 375, height: 812 }, permissions: ['clipboard-read', 'clipboard-write'] });
  t.after(() => context.close());
  const sent = {};
  let armed = false;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/mfa/totp:enroll') {
      sent.enroll = request.postDataJSON();
      return route.fulfill({ json: ENROLLMENT });
    }
    if (path === '/auth/mfa/totp:confirm') {
      sent.confirm = request.postDataJSON();
      armed = true;
      return route.fulfill({ json: { armed: true } });
    }
    if (path === '/accounts/me') {
      return route.fulfill({ json: { ...ACCOUNT, has_second_factor: armed, ...(armed ? { recovery_codes_remaining: 10 } : {}) } });
    }
    return answer(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));

  await page.goto(`${served.origin}/profile/security`);
  await walkSetup(page, sent);
  assert.deepEqual(sent.enroll, {}, 'a signed-in setup presents no pending credential');
  assert.equal(sent.confirm?.pending_token, undefined);

  await page.getByText('The second factor is on.', { exact: false }).first().waitFor();
  assert.equal(await page.getByText(ENROLLMENT.recovery_codes[0]).count(), 0, 'the codes outlived the panel');
  assert.deepEqual(failures, []);
});

// UC-ID-05 check 5: an account that signs in only through a provider has no password, so the profile
// offers no password change, and no proof it is asked for is a password - a step-up names nothing
// it could give and the dialog says so instead of drawing a field.
test('chromium: 1280 px — a provider-only account is offered no password and asked for none', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/accounts/me') {
      return route.fulfill({ json: { ...ACCOUNT, has_password: false, has_second_factor: true, recovery_codes_remaining: 4 } });
    }
    if (path === '/auth/mfa/recovery:regenerate') {
      return route.fulfill({ status: 403, json: { code: 'errors.forbidden', detail_code: 'auth.step_up_required', status: 403, params: { methods: 'TOTP' }, request_id: 'req_s' } });
    }
    return answer(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/profile/security`);
  await page.getByText('Left: 4 of 10.').waitFor();

  assert.equal(await page.getByRole('button', { name: 'Change the password' }).count(), 0, 'a password change is offered to an account without one');
  assert.equal(await page.locator('input[autocomplete="current-password"]').count(), 0, 'a password is asked for on the profile');
  assert.ok(await page.getByText('signs in through a provider', { exact: false }).first().isVisible(), 'the profile does not say why');

  await page.getByRole('button', { name: 'Make new codes' }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  assert.equal(await dialog.locator('input[type="password"]').count(), 0, 'the step-up asks a provider-only account for a password');
  // The account holds a factor and no password, so the server names the code alone and the dialog
  // asks for nothing else. (A refusal naming nothing at all is `stepup.test.ts`'s.)
  assert.equal(await dialog.locator('input[autocomplete="one-time-code"]').count(), 1, 'the dialog does not ask for the code');
});

// UC-ID-06 check 2: every session says how it was opened, in words, and the provider by its name.
test('chromium: 1280 px — every session says how it was opened', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const opened = [
    { id: 'm1', user_agent: 'Firefox on Linux', created_at: '2026-09-01T08:00:00Z', last_used_at: null, ip_class: null, current: true, signed_in_with: 'PASSWORD_TOTP' },
    { id: 'm2', user_agent: 'Safari on iPhone', created_at: '2026-09-02T08:00:00Z', last_used_at: null, ip_class: null, current: false, signed_in_with: 'OIDC', signed_in_provider: 'Contoso Entra ID' },
    { id: 'm3', user_agent: 'Chrome on macOS', created_at: '2026-09-03T08:00:00Z', last_used_at: null, ip_class: null, current: false, signed_in_with: 'PASSWORD_RECOVERY' },
    { id: 'm4', user_agent: 'Edge on Windows', created_at: '2026-09-04T08:00:00Z', last_used_at: null, ip_class: null, current: false, signed_in_with: null },
  ];
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions') return route.fulfill({ json: opened });
    return answer(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/profile/sessions`);
  await page.getByText('Safari on iPhone').waitFor();

  const row = async (client) => (await page.locator('tr', { hasText: client }).innerText()).replace(/\s+/g, ' ');
  assert.match(await row('Firefox on Linux'), /Password and second factor/);
  assert.match(await row('Safari on iPhone'), /Contoso Entra ID/);
  assert.match(await row('Chrome on macOS'), /Recovery code/);
  assert.doesNotMatch(await row('Edge on Windows'), /OIDC|PASSWORD|undefined|null/);
});

// UC-ID-06 check 4 (SC-23): *Sign out everywhere else* ends every other session and leaves this one -
// the reader stays on the screen, signed in, and the list shows only this device.
test('chromium: signing out everywhere else keeps this session and shows it alone', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  let listed = SESSIONS;
  const sent = [];
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/auth/sessions:revoke-others' && request.method() === 'POST') {
      sent.push(path);
      listed = SESSIONS.filter((one) => one.current);
      return route.fulfill({ status: 204 });
    }
    if (path === '/auth/sessions' && request.method() === 'DELETE') {
      sent.push('DELETE /auth/sessions');
      return route.fulfill({ status: 204 });
    }
    if (path === '/auth/sessions') return route.fulfill({ json: listed });
    return answer(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  await page.goto(`${served.origin}/profile/sessions`);
  await page.getByText('Safari on iPhone').waitFor();

  assert.match(await page.getByText(/You stay signed in here/).textContent() ?? '', /every other session/);
  await page.getByRole('button', { name: 'Sign out everywhere else' }).click();
  await page.getByText('Safari on iPhone').waitFor({ state: 'detached' });

  assert.deepEqual(sent, ['/auth/sessions:revoke-others'], 'the screen ended this session too');
  const rows = await page.getByRole('table', { name: 'Where you are signed in' }).locator('tbody tr').count();
  assert.equal(rows, 1, 'more than this session is still listed');
  assert.equal(new URL(page.url()).pathname, '/profile/sessions', 'the reader was signed out');
  // Nothing else to end: the control is gone with the sessions it ended.
  assert.equal(await page.getByRole('button', { name: 'Sign out everywhere else' }).count(), 0);
});
