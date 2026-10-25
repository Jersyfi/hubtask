// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The mail intake (ADR-0094, UC-JUM-09, UC-JUM-10), walked against the shared fixture:
//
// * the administrator switches the intake on, names the workspace on the receiving domain, chooses
//   members and allowed senders, allows a domain, adds a sender rule - saved in one write;
// * the page shows the routing order, every collection's handle and what the last messages became;
// * a collection's settings show its subject word and address and change its handle;
// * a jumble card says why the intake put a message there;
// * an entry shows its ticket key above its title.
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

    // The order, the handles, the log with its reasons.
    await page.locator('[data-mail-order]').getByText('[SHOP-42]').waitFor();
    await page.locator('[data-mail-handles] [data-handle="kitchen"] code').waitFor({ timeout: 10_000 });
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
