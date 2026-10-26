// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The mail intake (ADR-0094, UC-JUM-09, UC-JUM-10), walked against the shared fixture:
//
// * the administrator switches the intake on, names the workspace on the receiving domain, chooses
//   members and allowed senders, allows a domain, adds a sender rule - saved in one write;
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
    // The administration page: on, a name, a level with its list, a rule - one write.
    await page.goto(`${served.origin}/administration/mail`);
    const on = page.locator('[data-mail-on]');
    await on.waitFor({ timeout: 15_000 });
    await on.getByText('Take mail').click();
    await on.getByLabel('Name on the receiving domain').fill('familie');
    await on.locator('[data-mail-address] code').getByText('familie@in.hubtask.test').waitFor();
    const senders = page.locator('[data-mail-senders]');
    await senders.getByText('Members and allowed senders').click();
    await senders.getByLabel('Allowed', { exact: true }).fill('@lieferant.test');
    const rules = page.locator('[data-mail-routes]');
    await rules.getByRole('button', { name: 'Add a rule' }).click();
    await rules.getByLabel('Sender or @domain').fill('@lieferant.test');
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await page.getByText('Mail settings saved.').waitFor({ timeout: 5_000 }).catch(() => undefined);
    const saved = writes.find((write) => write.call === 'PUT /mail/settings');
    assert.ok(saved, `writes ${JSON.stringify(writes)}`);
    assert.equal(saved.body.enabled, true);
    assert.equal(saved.body.name, 'familie');
    assert.deepEqual(saved.body.senders, { level: 'MEMBERS_AND_ALLOWED', allowed: ['@lieferant.test'], blocked: [] });
    assert.equal(saved.body.sender_routes[0].match, '@lieferant.test');

    // A mailbox: a refused password is said in the form, the right one connects it, and the handles
    // show their plus address through it.
    const connect = page.locator('[data-mailbox-connect]');
    await connect.locator('[data-mailbox-hint]').getByText('app password', { exact: false }).waitFor({ timeout: 10_000 });
    await connect.getByLabel('Mailbox address').fill('familie@gmail.com');
    await connect.getByLabel('Password or app password').fill('wrong');
    await connect.getByRole('button', { name: 'Connect' }).click();
    await connect.getByText('refused the sign-in', { exact: false }).waitFor({ timeout: 5_000 });
    await connect.getByLabel('Password or app password').fill('app-pw');
    await connect.getByRole('button', { name: 'Connect' }).click();
    await page.locator('[data-mailbox="familie@gmail.com"]').waitFor({ timeout: 5_000 });
    const connected = writes.filter((write) => write.call === 'POST /mail/mailboxes').at(-1);
    assert.equal(connected.body.preset, 'gmail');
    assert.equal(connected.body.secret, 'app-pw');
    await page.locator('[data-mail-handles] [data-handle="kitchen"] [data-plus-address]').getByText('familie+kitchen@gmail.com').waitFor();

    // The order, the handles, the log with its reasons.
    await page.locator('[data-mail-order]').getByText('[SHOP-42]').waitFor();
    await page.locator('[data-mail-handles] [data-handle="kitchen"] code').first().waitFor({ timeout: 10_000 });
    const log = page.locator('[data-mail-log]');
    await log.getByText('In the jumble · Sender not allowed').waitFor();
    await log.getByRole('link', { name: 'Entry' }).waitFor();

    // A collection's settings: its subject word and address, and a new handle.
    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    await openSettings(page, COLLECTION.name);
    const mail = page.locator('[data-mail-settings]');
    await mail.locator('[data-mail-subject] code').getByText('#kitchen').waitFor({ timeout: 10_000 });
    await mail.locator('[data-mail-address] code').getByText('familie+kitchen@in.hubtask.test').waitFor();
    await mail.getByLabel('Handle').fill('Kueche');
    await mail.getByRole('button', { name: 'Change the handle' }).click();
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
    const oauth = page.locator('[data-mailbox-oauth]');
    await oauth.waitFor({ timeout: 15_000 });

    // Microsoft without the installation's app: the form for one's own, with the redirect URI to
    // register; nothing leaves before a client ID is there.
    await oauth.getByRole('button', { name: 'Connect with Microsoft' }).click();
    const own = page.locator('[data-mailbox-own-app="microsoft"]');
    assert.equal(await own.getByLabel('Redirect URI', { exact: false }).inputValue(), `${served.origin}/administration/mail/oauth`);
    await own.getByRole('button', { name: 'Cancel' }).click();

    // Google with the installation's app: one button, to the provider and back, connected.
    await oauth.getByRole('button', { name: 'Connect with Google' }).click();
    await page.waitForURL(`${served.origin}/administration/mail`, { timeout: 15_000 });
    const row = page.locator('[data-mailbox="familie@gmail.com"]');
    await row.getByText('signed in with Google', { exact: false }).waitFor({ timeout: 10_000 });
    assert.deepEqual(writes.find((write) => write.call === 'POST /mail/mailboxes:authorize')?.body, { provider: 'google' });
    assert.deepEqual(writes.find((write) => write.call === 'POST /mail/mailboxes:complete')?.body, { state: 's-1', code: 'c-1' });
    assert.deepEqual(failures, []);
  } finally {
    await browser.close();
  }
});

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
