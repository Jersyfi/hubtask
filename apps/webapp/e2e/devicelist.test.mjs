// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The device's list of workspaces (UC-ID-25 checks 1 to 6 and 8; ADR-0095 §7), walked across
// three origins of one installation: its own address (`localhost`) and two workspaces beneath it
// (`acme.localhost`, `north.localhost`). Signing in at a workspace hands its signed entry to the
// installation's address and comes back; the account menu's *Switch workspace* and the
// installation's address show the list; forgetting takes an entry off everywhere; an entry whose
// address changed says so and can only be forgotten; a forged one is never added; single mode
// shows nothing of it.
//
// Against a stubbed API: the stub signs an entry by a tag of its own and answers the check as the
// server does - an entry it did not sign is left out. Chromium only, as the other walks.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { ACCOUNT, MANIFEST, WORKSPACE, lookOf, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

const PORT = new URL(served.origin).port;
const INSTALLATION = `http://localhost:${PORT}`;
const at = (host) => `http://${host}:${PORT}`;
const NAMES = { 'acme.localhost': 'Acme Ltd', 'north.localhost': 'Northwind Studio', 'meier.localhost': 'Meier family' };
/** A workspace's square symbol, one pixel of PNG. */
const SYMBOL_PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64');
const NORTH_SYMBOL = '01a0e2e0-0000-7000-8000-00000000b0a1';

/** An entry as the installation signs it; `forged` carries a tag the stub does not recognise. */
function entryFor(host, { forged = false } = {}) {
  const payload = Buffer.from(JSON.stringify({ host, name: NAMES[host] ?? host })).toString('base64url');
  return `${payload}.${forged ? 'Zm9yZ2Vk' : 'c2lnbmVk'}`;
}

const MULTI = {
  ...MANIFEST, tenancy_mode: 'multi', workspace_hosts: 'subdomain', installation_url: INSTALLATION,
  features: { ...MANIFEST.features, sign_in_rules: true },
};

const RULES = (host) => ({
  workspace_host: host, methods: ['PASSWORD'], providers: [],
  password: {
    min_length: 12, min_lowercase: 0, min_uppercase: 0, min_digits: 0, min_symbols: 0,
    min_classes: 0, max_repeat: null, common_passwords: true, context_words: true,
    breach_check: false, history_count: 0, not_current: false,
  },
  legal: {},
});

const SIGNED_IN = {
  token_type: 'Bearer', access_token: 'e2e-access', refresh_token: 'e2e-refresh',
  access_token_expires_at: '2099-01-01T00:00:00Z', refresh_token_expires_at: '2099-01-01T00:00:00Z',
};

/**
 * A browser of the installation. `main` holds the hosts that are a workspace's main address now;
 * `asked` records the entry reads and the checks.
 */
async function device(browser, { main = ['acme.localhost', 'north.localhost'], manifest = MULTI, width = 1280, symbols = {} } = {}) {
  const context = await browser.newContext({ viewport: { width, height: 900 } });
  const asked = { entries: [], checks: [] };
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/meta/capabilities') return route.fulfill({ json: manifest });
    // A workspace with a symbol of its own (UC-INS-22 check 6): its answer, its record, its bytes.
    const symbol = symbols[url.hostname];
    const branded = { ...WORKSPACE, display_name: NAMES[url.hostname], branding: { header: 'PRODUCT', logo_light_media_id: null, logo_dark_media_id: null, symbol_media_id: symbol, allowed: true } };
    if (symbol && path === '/tenant' && request.method() === 'GET') return route.fulfill({ json: branded });
    if (symbol && path === '/tenant/look' && request.method() === 'GET') return route.fulfill({ json: lookOf(branded) });
    if (symbol && path === `/media/${symbol}`) {
      return route.fulfill({ json: {
        id: symbol, usage: 'WORKSPACE_LOGO', content_type: 'image/png', size: SYMBOL_PNG.length, ref_count: 1, status: 'READY', created_by: ACCOUNT.id, created_at: '2026-11-23T10:00:00Z',
        download: { url: `${at(url.hostname)}/api/v1/media/${symbol}:content`, method: 'GET', expires_at: '2099-01-01T00:00:00Z' },
      } });
    }
    if (symbol && path === `/media/${symbol}:content`) return route.fulfill({ status: 200, contentType: 'image/png', body: SYMBOL_PNG });
    if (path === '/auth/sign-in-rules') return route.fulfill({ json: RULES(url.hostname) });
    if (path === '/auth/sessions' && request.method() === 'POST') return route.fulfill({ status: 201, json: SIGNED_IN });
    if (path === '/tenant/device-list-entry') {
      asked.entries.push(url.hostname);
      const entry = entryFor(url.hostname);
      return route.fulfill({ json: { entry, host: url.hostname, display_name: NAMES[url.hostname] } });
    }
    if (path === '/auth/device-list:check') {
      const entries = request.postDataJSON().entries;
      asked.checks.push(entries);
      const answered = entries.filter((each) => each.endsWith('.c2lnbmVk')).map((each) => {
        const { host, name } = JSON.parse(Buffer.from(each.split('.')[0], 'base64url').toString());
        return { entry: each, host, display_name: name, answers: main.includes(host) };
      });
      return route.fulfill({ json: { entries: answered } });
    }
    return stub(route);
  });
  return { context, asked };
}

/** The list as the installation's own address keeps it, read from the browser's storage. */
async function listAtInstallation(context) {
  const state = await context.storageState();
  const origin = state.origins.find((each) => each.origin === INSTALLATION);
  const kept = origin?.localStorage.find((each) => each.name === 'hubtask.workspaces');
  return kept ? JSON.parse(kept.value) : [];
}

/** Signs in on the card at a workspace's address, and waits until it is back there, signed in. */
async function signInAt(context, host) {
  const page = await context.newPage();
  const visited = [];
  page.on('framenavigated', (frame) => {
    if (frame === page.mainFrame()) visited.push(frame.url());
  });
  await page.goto(`${at(host)}/`);
  await page.locator('input[type="email"]').fill('lena@example.invalid');
  await page.locator('input[autocomplete="current-password"]').fill('the right password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await page.waitForURL((url) => url.host === `${host}:${PORT}` && visited.some((each) => each.startsWith(`${INSTALLATION}/workspaces`)), { timeout: 10_000 });
  await page.getByRole('button', { name: 'Lena Vogt' }).or(page.locator('button.account')).first().waitFor({ timeout: 10_000 });
  return { page, visited };
}

async function openSwitch(page) {
  await page.locator('button.account').click();
  await page.getByRole('menuitem', { name: 'Switch workspace' }).click();
  return page.getByRole('dialog', { name: 'Switch workspace' });
}

test('chromium: a sign-in adds the workspace, and Switch workspace opens another (checks 1, 2)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, asked } = await device(browser);
  t.after(() => context.close());

  const acme = await signInAt(context, 'acme.localhost');
  // Through the installation's address and back, once, with the entry; nothing else is kept.
  assert.ok(acme.visited.some((each) => each.startsWith(`${INSTALLATION}/workspaces#add=`)), acme.visited.join('\n'));
  assert.equal(new URL(acme.page.url()).hash, '', 'the handover stayed in the address');
  assert.deepEqual(asked.entries, ['acme.localhost']);
  const kept = await listAtInstallation(context);
  assert.deepEqual(kept, [entryFor('acme.localhost')]);
  const payload = JSON.parse(Buffer.from(kept[0].split('.')[0], 'base64url').toString());
  assert.deepEqual(Object.keys(payload).sort(), ['host', 'name'], 'the entry holds more than host and name');

  const north = await signInAt(context, 'north.localhost');
  assert.deepEqual(await listAtInstallation(context), [entryFor('north.localhost'), entryFor('acme.localhost')]);

  const dialog = await openSwitch(north.page);
  const rows = dialog.getByRole('listitem');
  await rows.filter({ hasText: 'Acme Ltd' }).waitFor();
  assert.match(await rows.filter({ hasText: 'Northwind Studio' }).innerText(), /This workspace/);
  assert.equal(await dialog.locator('input').count(), 0, 'the list offers a field');

  await rows.filter({ hasText: 'Acme Ltd' }).getByRole('button', { name: 'Open Acme Ltd' }).click();
  await north.page.waitForURL((url) => url.host === `acme.localhost:${PORT}`, { timeout: 10_000 });
});

test('chromium: on a phone the account sheet lists the workspaces, and one tapped opens (the navigation concept §9)', async (t) => {
  const browser = await sharedChromium(t);
  const { context } = await device(browser, { width: 390 });
  t.after(() => context.close());

  await signInAt(context, 'acme.localhost');
  const north = await signInAt(context, 'north.localhost');
  await north.page.locator('[data-phone-bar] button.account').click();
  const sheet = north.page.getByRole('dialog', { name: 'You' });
  const listed = sheet.locator('[data-workspaces]');
  await listed.getByRole('link', { name: 'Acme Ltd' }).or(listed.getByRole('button', { name: 'Acme Ltd' })).first().waitFor({ timeout: 10_000 });
  assert.equal(await listed.locator('[aria-current="page"]').innerText().then((text) => text.trim()), 'Northwind Studio', 'this workspace is not the checked one');
  assert.equal(await sheet.getByRole('button', { name: 'Switch workspace' }).count(), 0, 'the switch is a row besides the list');
  await listed.getByText('Acme Ltd').click();
  await north.page.waitForURL((url) => url.host === `acme.localhost:${PORT}`, { timeout: 10_000 });
});

// UC-INS-22 check 6: the current workspace's row shows its own symbol - in *Switch workspace* and in
// the phone's sheet, in the check's place there; the others keep their letters, since the device's
// list holds only each workspace's address and name (O4).
test('chromium: the current workspace\'s row shows its symbol, the others their letters (UC-INS-22 check 6)', async (t) => {
  const browser = await sharedChromium(t);
  const wide = await device(browser, { symbols: { 'north.localhost': NORTH_SYMBOL } });
  t.after(() => wide.context.close());
  await signInAt(wide.context, 'acme.localhost');
  const north = await signInAt(wide.context, 'north.localhost');
  const dialog = await openSwitch(north.page);
  const here = dialog.getByRole('listitem').filter({ hasText: 'Northwind Studio' });
  await here.locator('img').waitFor({ timeout: 10_000 });
  await north.page.waitForFunction(() => [...document.querySelectorAll('dialog[open] li img')].every((each) => each.naturalWidth > 0));
  assert.match((await here.locator('img').getAttribute('src')) ?? '', new RegExp(`/media/${NORTH_SYMBOL}:content`));
  const other = dialog.getByRole('listitem').filter({ hasText: 'Acme Ltd' });
  assert.equal(await other.locator('img').count(), 0, 'another workspace drew a picture');
  assert.match(await other.innerText(), /^AL\b/);

  const phone = await device(browser, { width: 390, symbols: { 'north.localhost': NORTH_SYMBOL } });
  t.after(() => phone.context.close());
  await signInAt(phone.context, 'acme.localhost');
  const small = await signInAt(phone.context, 'north.localhost');
  await small.page.locator('[data-phone-bar] button.account').click();
  const listed = small.page.getByRole('dialog', { name: 'You' }).locator('[data-workspaces]');
  const current = listed.locator('[aria-current="page"]');
  await current.locator('img').waitFor({ timeout: 10_000 });
  assert.equal((await current.innerText()).trim(), 'Northwind Studio');
  assert.equal(await listed.locator('img').count(), 1, 'another row drew a picture');
});

test('chromium: forgetting takes the workspace off the menu at once and off the installation\'s address (checks 3, 4)', async (t) => {
  const browser = await sharedChromium(t);
  const { context } = await device(browser);
  t.after(() => context.close());

  await signInAt(context, 'acme.localhost');
  const north = await signInAt(context, 'north.localhost');

  const dialog = await openSwitch(north.page);
  const acmeRow = dialog.getByRole('listitem').filter({ hasText: 'Acme Ltd' });
  await acmeRow.getByRole('button', { name: 'Actions for Acme Ltd' }).click();
  await north.page.getByRole('menuitem', { name: 'Forget this workspace on this device' }).click();
  // Gone from the menu at once; then the redirect there and back takes it off the installation's.
  await acmeRow.waitFor({ state: 'detached' });
  await north.page.waitForURL(
    (url) => url.host === `north.localhost:${PORT}` && url.hash === '' && north.visited.some((each) => each.startsWith(`${INSTALLATION}/workspaces#forget=`)),
    { timeout: 10_000 },
  );
  assert.deepEqual(await listAtInstallation(context), [entryFor('north.localhost')]);

  const home = await context.newPage();
  await home.goto(`${INSTALLATION}/`);
  await home.getByRole('heading', { name: 'Your workspaces' }).waitFor();
  assert.equal(await home.getByText('Acme Ltd').count(), 0);
  await home.getByRole('button', { name: 'Forget Northwind Studio on this device' }).click();
  await home.getByRole('heading', { name: 'No workspace on this device yet' }).waitFor();
  assert.equal(await home.locator('input').count(), 0, 'the installation\'s address offers a field');
  assert.deepEqual(await listAtInstallation(context), []);
});

test('chromium: an entry whose address changed says so and is only forgotten; a forged one is never added (checks 5, 6)', async (t) => {
  const browser = await sharedChromium(t);
  const { context } = await device(browser, { main: ['acme.localhost'] });
  t.after(() => context.close());
  const forged = entryFor('north.localhost', { forged: true });

  const home = await context.newPage();
  await home.goto(`${INSTALLATION}/`);
  await home.getByRole('heading', { name: 'No workspace on this device yet' }).waitFor();
  await home.evaluate((list) => localStorage.setItem('hubtask.workspaces', JSON.stringify(list)),
    [entryFor('acme.localhost'), entryFor('meier.localhost'), forged]);
  await home.reload();
  await home.getByRole('heading', { name: 'Your workspaces' }).waitFor();

  const gone = home.getByRole('listitem').filter({ hasText: 'Meier family' });
  await gone.getByText('No longer answers at this address.').waitFor();
  assert.equal(await gone.getByRole('button', { name: /^Open/ }).count(), 0, 'a stale entry still opens');
  assert.equal(await home.getByText('Northwind Studio').count(), 0, 'a forged entry is listed');
  assert.deepEqual(await listAtInstallation(context), [entryFor('acme.localhost'), entryFor('meier.localhost')]);
  await gone.getByRole('button', { name: 'Forget Meier family on this device' }).click();
  await gone.waitFor({ state: 'detached' });
  assert.deepEqual(await listAtInstallation(context), [entryFor('acme.localhost')]);

  // A link that carries a forged entry adds nothing and sends nobody anywhere.
  await home.goto(`${INSTALLATION}/workspaces#add=${forged}&path=%2F`);
  await home.getByRole('heading', { name: 'Your workspaces' }).waitFor();
  assert.equal(new URL(home.url()).host, `localhost:${PORT}`);
  assert.deepEqual(await listAtInstallation(context), [entryFor('acme.localhost')]);
  // Nor does one that asks to open a host the list does not hold.
  await home.goto(`${INSTALLATION}/workspaces#open=north.localhost`);
  await home.getByRole('heading', { name: 'Your workspaces' }).waitFor();
  assert.equal(new URL(home.url()).host, `localhost:${PORT}`);
});

test('chromium: in single mode there is no list and no Switch workspace (check 8)', async (t) => {
  const browser = await sharedChromium(t);
  const { context, page } = await signedIn(browser, 1280);
  t.after(() => context.close());
  const asked = [];
  page.on('request', (request) => {
    if (request.url().includes('device-list')) asked.push(request.url());
  });
  await page.goto(served.origin);
  await page.locator('button.account').click();
  await page.getByRole('menuitem', { name: 'Sign out' }).waitFor();
  assert.equal(await page.getByRole('menuitem', { name: 'Switch workspace' }).count(), 0);
  assert.deepEqual(asked, []);
});
