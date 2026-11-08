// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The mail intake (ADR-0094, UC-JUM-09, UC-JUM-10), walked against the shared fixture:
//
// * the administrator switches the intake on, names the workspace on the receiving domain, chooses
//   members and allowed senders, allows a domain, adds a sender rule - each section at its own
//   address with its own save, every write keeping what the others saved (page-system §7.3);
// * the page shows the routing order, every collection's handle and what the last messages became;
// * a collection's settings show its subject word and address and change its handle;
// * a jumble card says why the intake put a message there;
// * an entry shows its ticket key above its title;
// * a mailbox connects through its provider's sign-in: Google in one button with the installation's
//   app, Microsoft with an app of one's own, and the provider's answer comes back to the page.
//
// Chromium, large.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, ITEMS, JUMBLED, openSettings, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SETTINGS = {
  enabled: false, senders: { level: 'MEMBERS', allowed: [], blocked: [] }, sender_routes: [], domain: 'in.hubtask.test',
};
const RECEIPTS = [
  { id: '01a0e2e0-0000-7000-8000-00000000e001', received_at: '2026-10-25T09:00:00Z', sender: 'kunde@shop.test', subject: 'Angebot', outcome: 'JUMBLE', reason: 'SENDER_NOT_ALLOWED', entry_id: JUMBLED.id, door: 'inbound' },
  { id: '01a0e2e0-0000-7000-8000-00000000e002', received_at: '2026-10-25T08:00:00Z', sender: 'j@example.invalid', subject: '#kitchen Tiles', outcome: 'ENTRY', item_id: ITEMS[0].id, door: 'inbound' },
];

test('chromium: the intake is set up, read and used in the places it reaches', { timeout: 120_000 }, async () => {
  const browser = await chromium.launch();
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  const writes = [];
  let settings = SETTINGS;
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() !== 'GET') writes.push({ call: `${request.method()} ${path}`, body: request.postDataJSON?.() ?? null });
    if (path === '/mail/settings' && request.method() === 'GET') return route.fulfill({ json: settings });
    if (path === '/mail/settings' && request.method() === 'PUT') {
      const body = request.postDataJSON();
      settings = { ...body, domain: 'in.hubtask.test', address: `${body.name}@in.hubtask.test` };
      return route.fulfill({ json: settings });
    }
    if (path === '/mail/receipts') return route.fulfill({ json: { receipts: RECEIPTS } });
    if (path === '/mail/mailboxes' && request.method() === 'POST') {
      const body = request.postDataJSON();
      if (body.secret !== 'app-pw') {
        return route.fulfill({ status: 422, contentType: 'application/problem+json', json: { type: 'about:blank', title: 'validation', status: 422, code: 'validation', detail_code: 'mail.mailbox_refused' } });
      }
      return route.fulfill({ status: 201, json: {
        id: '01a0e2e0-0000-7000-8000-00000000b001', protocol: 'IMAP', address: body.address, host: 'imap.gmail.com', port: 993,
        username: body.address, folder: 'INBOX', done_folder: 'Hubtask', preset: 'gmail', plus_addresses: true, enabled: true,
        failing: false, checked_at: '2026-10-25T10:00:00Z', created_at: '2026-10-25T10:00:00Z',
      } });
    }
    if (path === `/collections/${COLLECTION.id}/handle`) return route.fulfill({ json: { handle: request.postDataJSON().handle, collection_id: COLLECTION.id } });
    if (path === '/jumble/entries') {
      return route.fulfill({ json: { data: [{ ...JUMBLED, channel: 'EMAIL', raw_subject: 'Angebot', sender: 'kunde@shop.test', meant_for_kind: 'COLLECTION', meant_for_id: COLLECTION.id, intake_reason: 'SENDER_NOT_ALLOWED' }], next_cursor: null } });
    }
    return stub(route);
  });
  try {
    // The administration page: on and a name, a level with its list, a rule - a write per section.
    await page.goto(`${served.origin}/administration/mail/intake`);
    const on = page.locator('[data-mail-on]');
    await on.waitFor({ timeout: 15_000 });
    await on.getByRole('switch', { name: 'Take mail' }).check();
    await on.getByLabel('Name on the receiving domain').fill('familie');
    await on.locator('[data-mail-address] code').getByText('familie@in.hubtask.test').waitFor();
    await on.getByRole('button', { name: 'Save', exact: true }).click();
    await waitFor(() => writes.filter((write) => write.call === 'PUT /mail/settings').length === 1);

    await section(page, 'Who may write');
    const senders = page.locator('[data-mail-senders]');
    await senders.getByText('Members and allowed senders').click();
    await senders.getByLabel('Allowed', { exact: true }).fill('@lieferant.test');
    await senders.getByRole('button', { name: 'Save', exact: true }).click();
    await waitFor(() => writes.filter((write) => write.call === 'PUT /mail/settings').length === 2);

    await section(page, 'Sender rules');
    await page.locator('[data-mail-routes]').getByRole('button', { name: 'Add a rule' }).click();
    const ruleDialog = page.locator('dialog[open]');
    await ruleDialog.getByLabel('Sender or @domain').fill('@lieferant.test');
    await ruleDialog.getByRole('button', { name: 'Add a rule' }).click();
    await page.locator('[data-mail-routes] [data-mail-rule="@lieferant.test"]').waitFor({ timeout: 5_000 });
    const saved = writes.filter((write) => write.call === 'PUT /mail/settings').at(-1);
    assert.equal(writes.filter((write) => write.call === 'PUT /mail/settings').length, 3, `writes ${JSON.stringify(writes)}`);
    assert.equal(saved.body.enabled, true);
    assert.equal(saved.body.name, 'familie');
    assert.deepEqual(saved.body.senders, { level: 'MEMBERS_AND_ALLOWED', allowed: ['@lieferant.test'], blocked: [] });
    assert.equal(saved.body.sender_routes[0].match, '@lieferant.test');

    // A mailbox: none yet, so the section says what one is for; the head's act opens the dialog,
    // the provider first and then the sign-in; a refused password is said in it, the right one
    // connects it and closes it, and the handles show their plus address through it.
    await section(page, 'Mailboxes');
    await page.locator('[data-mail-mailboxes]').getByText('No mailbox is connected').waitFor({ timeout: 10_000 });
    await page.getByRole('button', { name: 'Connect a mailbox' }).first().click();
    const dialog = page.locator('dialog[open]');
    await dialog.getByRole('button', { name: 'Continue', exact: true }).click();
    const connect = dialog.locator('[data-mailbox-connect]');
    await connect.locator('[data-mailbox-hint]').getByText('app password', { exact: false }).waitFor({ timeout: 10_000 });
    await connect.getByLabel('Mailbox address').fill('familie@gmail.com');
    await connect.getByLabel('Password or app password').fill('wrong');
    await dialog.getByRole('button', { name: 'Connect', exact: true }).click();
    await dialog.getByText('refused the sign-in', { exact: false }).waitFor({ timeout: 5_000 });
    await connect.getByLabel('Password or app password').fill('app-pw');
    await dialog.getByRole('button', { name: 'Connect', exact: true }).click();
    await page.locator('[data-mailbox="familie@gmail.com"]').waitFor({ timeout: 5_000 });
    await page.locator('dialog[open]').waitFor({ state: 'detached', timeout: 5_000 });
    await rowOf(page, 'familie@gmail.com').getByText('Reading', { exact: true }).waitFor();
    const connected = writes.filter((write) => write.call === 'POST /mail/mailboxes').at(-1);
    assert.equal(connected.body.preset, 'gmail');
    assert.equal(connected.body.secret, 'app-pw');
    await section(page, 'Handles');
    await page.locator('[data-mail-handles] [data-handle-through="kitchen"] [data-plus-address]').getByText('familie+kitchen@gmail.com').waitFor();

    // The order is the help of *Intake*, folded; the handles; the log with its reasons.
    await page.locator('[data-mail-handles] [data-handle="kitchen"] code').first().waitFor({ timeout: 10_000 });
    await section(page, 'Intake');
    await page.locator('[data-mail-on]').getByText('How Hubtask files mail').click();
    await page.locator('[data-mail-order]').getByText('[SHOP-42]').waitFor();
    await section(page, 'Recently arrived');
    const log = page.locator('[data-mail-log]');
    await log.getByText('In the inbox · Sender not allowed').waitFor();
    await log.getByRole('link', { name: 'Entry' }).waitFor();

    // A collection's settings: its subject word and address, and a new handle.
    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    await openSettings(page, COLLECTION.name);
    const mail = page.locator('[data-mail-settings]');
    await mail.locator('[data-mail-subject] code').getByText('#kitchen').waitFor({ timeout: 10_000 });
    await mail.locator('[data-mail-address] code').getByText('familie+kitchen@in.hubtask.test').waitFor();
    await mail.getByLabel('Handle').fill('Kueche');
    // The handle joins *General*'s one save (PS-10).
    await page.getByRole('region', { name: 'Unsaved changes' }).getByRole('button', { name: 'Save', exact: true }).click();
    await expectWrite(writes, `PUT /collections/${COLLECTION.id}/handle`, { handle: 'kueche' });

    // The jumble says why a message waits there.
    await page.goto(`${served.origin}/jumble`);
    await page.locator('[data-intake-reason]').getByText('Sender not allowed').waitFor({ timeout: 10_000 });

    // An entry's ticket key, above its title.
    await page.goto(`${served.origin}/items/${ITEMS[0].id}`);
    await page.getByRole('button', { name: 'Copy the key KITCHEN-31' }).waitFor({ timeout: 10_000 });

    assert.deepEqual(failures, []);
  } finally {
    await browser.close();
  }
});

test('chromium: a mailbox connects through its provider and comes back connected', { timeout: 120_000 }, async () => {
  const browser = await chromium.launch();
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  const writes = [];
  let mailboxes = [];
  await context.route('https://accounts.provider.test/**', (route) => {
    // The provider: the administrator says yes, and the browser goes back with a code.
    const state = new URL(route.request().url()).searchParams.get('state');
    return route.fulfill({ status: 302, headers: { location: `${served.origin}/administration/mail/oauth?state=${state}&code=c-1` } });
  });
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() !== 'GET') writes.push({ call: `${request.method()} ${path}`, body: request.postDataJSON?.() ?? null });
    if (path === '/mail/settings' && request.method() === 'GET') return route.fulfill({ json: SETTINGS });
    if (path === '/mail/receipts') return route.fulfill({ json: { receipts: [] } });
    if (path === '/mail/mailboxes' && request.method() === 'GET') {
      return route.fulfill({ json: {
        mailboxes, presets: [{ key: 'gmail', protocol: 'IMAP', plus_addresses: true, hint: 'mail.preset_gmail' }],
        oauth_providers: [
          { key: 'google', installation: true, hint: 'mail.oauth_google' },
          { key: 'microsoft', installation: false, hint: 'mail.oauth_microsoft' },
        ],
        oauth_redirect_url: `${served.origin}/administration/mail/oauth`,
      } });
    }
    if (path === '/mail/mailboxes:authorize') {
      return route.fulfill({ json: { authorize_url: 'https://accounts.provider.test/auth?state=s-1', expires_at: '2026-10-25T10:10:00Z' } });
    }
    if (path === '/mail/mailboxes:complete') {
      const connected = {
        id: '01a0e2e0-0000-7000-8000-00000000b002', protocol: 'IMAP', address: 'familie@gmail.com', host: 'imap.gmail.com', port: 993,
        username: 'familie@gmail.com', folder: 'INBOX', done_folder: 'Hubtask', preset: 'gmail', plus_addresses: true, enabled: true,
        failing: false, checked_at: '2026-10-25T10:00:00Z', created_at: '2026-10-25T10:00:00Z', auth: 'OAUTH', oauth_provider: 'google',
      };
      mailboxes = [connected];
      return route.fulfill({ status: 201, json: connected });
    }
    return stub(route);
  });
  try {
    await page.goto(`${served.origin}/administration/mail`);
    await page.getByRole('button', { name: 'Connect a mailbox' }).first().click();
    const dialog = page.locator('dialog[open]');
    const oauth = dialog.locator('[data-mailbox-oauth]');
    await oauth.waitFor({ timeout: 15_000 });

    // Microsoft without the installation's app: the provider, then the form for one's own, with the
    // redirect URI to register; nothing leaves before a client ID is there.
    await oauth.getByRole('radio', { name: 'Connect with Microsoft' }).check();
    await dialog.getByRole('button', { name: 'Continue', exact: true }).click();
    const own = dialog.locator('[data-mailbox-own-app="microsoft"]');
    assert.equal(await own.getByLabel('Redirect URI', { exact: false }).inputValue(), `${served.origin}/administration/mail/oauth`);
    await dialog.getByRole('button', { name: 'Back', exact: true }).click();

    // Google with the installation's app: one button, to the provider and back, connected.
    // The page is on /administration/mail before it leaves, so its URL proves nothing: the
    // completion's request does, after the way to the provider and back.
    const completed = page.waitForRequest((request) => request.url().endsWith('/mail/mailboxes:complete'), { timeout: 30_000 });
    await oauth.getByRole('radio', { name: 'Connect with Google' }).check();
    await dialog.getByRole('button', { name: 'Continue to the provider' }).click();
    await completed;
    await page.waitForURL(`${served.origin}/administration/mail/mailboxes`, { timeout: 15_000 });
    const row = rowOf(page, 'familie@gmail.com');
    await row.getByText('signed in with Google', { exact: false }).waitFor({ timeout: 10_000 });
    assert.deepEqual(writes.find((write) => write.call === 'POST /mail/mailboxes:authorize')?.body, { provider: 'google' });
    assert.deepEqual(writes.find((write) => write.call === 'POST /mail/mailboxes:complete')?.body, { state: 's-1', code: 'c-1' });
    assert.deepEqual(failures, []);
  } finally {
    await browser.close();
  }
});

// A mailbox whose reads fail for an hour: its row says so with the failure beside *Check now*,
// the head counts it, and the column counts it on every screen of the administration
// (user-experience.md §7.10); a single failed read is said but not counted as an outage.
test('chromium: a failing mailbox is said beside Check now and counted in the column', { timeout: 120_000 }, async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  const box = (id, address, failing, failure) => ({
    id, protocol: 'IMAP', address, host: 'imap.gmail.com', port: 993, username: address, folder: 'INBOX', done_folder: 'Hubtask',
    preset: 'gmail', enabled: true, failing, ...(failure ? { failure } : {}), checked_at: '2026-11-03T10:00:00Z', created_at: '2026-10-25T10:00:00Z', auth: 'PASSWORD',
  });
  const checks = [];
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (path === '/mail/settings') return route.fulfill({ json: { enabled: true, senders: { level: 'MEMBERS', allowed: [], blocked: [] }, sender_routes: [] } });
    if (path === '/mail/receipts') return route.fulfill({ json: { receipts: [] } });
    if (path === '/mail/mailboxes' && request.method() === 'GET') {
      return route.fulfill({ json: {
        mailboxes: [box('b1', 'familie@gmail.com', true, 'mail.mailbox_refused'), box('b2', 'büro@gmail.com', false, 'mail.mailbox_unreachable'), box('b3', 'info@gmail.com', false)],
        presets: [{ key: 'gmail', protocol: 'IMAP', plus_addresses: true, hint: 'mail.preset_gmail' }],
      } });
    }
    if (path.endsWith(':check')) {
      checks.push(path);
      return route.fulfill({ json: box('b1', 'familie@gmail.com', false) });
    }
    return stub(route);
  });

  await page.goto(`${served.origin}/administration/people`);
  const column = page.getByRole('navigation', { name: 'Administration', exact: true });
  await column.getByText('1 failing').waitFor({ timeout: 10_000 });
  assert.equal(await column.getByRole('treeitem', { name: /1 failing/ }).count(), 1, 'the figure is not heard with the row');

  await page.goto(`${served.origin}/administration/mail`);
  await page.locator('[data-mail-figure]').getByText('2 reading · 1 failing').waitFor({ timeout: 10_000 });
  const failing = rowOf(page, 'familie@gmail.com');
  await failing.getByText('Failing for an hour or longer').waitFor();
  await failing.locator('[data-mailbox-failure]').getByText('refused the sign-in', { exact: false }).waitFor();
  await rowOf(page, 'büro@gmail.com').getByText('Last read failed').waitFor();
  await rowOf(page, 'info@gmail.com').getByText('Reading', { exact: true }).waitFor();

  await failing.getByRole('button', { name: 'Check now' }).click();
  await failing.getByText('Reading', { exact: true }).waitFor({ timeout: 5_000 });
  assert.deepEqual(checks, ['/mail/mailboxes/b1:check']);
  assert.deepEqual(failures, []);
});

/** Moves to a section of the page through its list, as a press does. */
async function section(page, name) {
  await page.getByRole('navigation', { name: 'Sections of E-mail' }).getByRole('link', { name: new RegExp(`^${name}`) }).click();
}

/** A mailbox's row of the table. */
function rowOf(page, address) {
  return page.getByRole('row').filter({ has: page.locator(`[data-mailbox="${address}"]`) });
}

async function waitFor(condition) {
  for (let tries = 0; tries < 50 && !condition(); tries++) await new Promise((resolve) => setTimeout(resolve, 100));
  assert.ok(condition(), 'the write did not arrive');
}

async function expectWrite(writes, call, body) {
  for (let tries = 0; tries < 50; tries++) {
    const found = writes.find((write) => write.call === call);
    if (found) {
      assert.deepEqual(found.body, body);
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  assert.fail(`no ${call} in ${JSON.stringify(writes)}`);
}
