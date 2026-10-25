// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Mail addresses for every place (ADR-0092, UC-JUM-09), walked against the shared fixture with the
// address routes answered here:
//
// * check 1 - the jumble shows its address in its head with *Copy*; a collection offers *Copy mail
//   address* in its menu and the address in *Settings → General*; an entry has a *By mail* line;
// * check 2 - *New address* replaces it, after a sentence that says what stops;
// * check 5 - who may send is chosen in the collection's settings, and a mail that waits in the
//   jumble for a place offers to sort it in there in one step;
// * check 8 - without a receiving domain no address is drawn, and the people who shape the place
//   read what is missing.
//
// Chromium only, as the other walks; large and compact.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, ITEMS, MANIFEST, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const MEANT = {
  id: '01a0e2e0-0000-7000-8000-0000000030a1', channel: 'EMAIL', sender: 'client@example.net',
  raw_subject: 'Offer for the tiles', raw_body: 'Please find our offer.', attachments: [], status: 'NEW',
  received_at: '2026-10-24T08:00:00Z', meant_for_kind: 'COLLECTION', meant_for_id: COLLECTION.id,
};
const SORTED_ITEM = '01a0e2e0-0000-7000-8000-0000000030a2';

async function open(browser, width, { available = true } = {}) {
  const { context, page, failures } = await signedIn(browser, width, 900);
  const writes = [];
  let code = 'aaaaaaaaaaaaaaaa';
  let policy = 'MEMBERS';
  const addressOf = (kind) => {
    if (!available) return { available: false };
    const label = kind === 'JUMBLE' ? 'jumble' : kind === 'COLLECTION' ? 'kitchen' : 'entry';
    return { available: true, address: `${label}.${code}@in.example.test`, target_kind: kind, sender_policy: policy };
  };
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    if (request.method() !== 'GET') writes.push({ call: `${request.method()} ${path}`, body: request.postDataJSON?.() ?? null });
    if (path === '/meta/capabilities') {
      return route.fulfill({ json: { ...MANIFEST, features: { ...MANIFEST.features, mail_addresses: available } } });
    }
    if (path === '/mail/addresses') return route.fulfill({ json: addressOf(url.searchParams.get('target_kind')) });
    if (path === '/mail/addresses:rotate') {
      code = 'bbbbbbbbbbbbbbbb';
      return route.fulfill({ json: addressOf(request.postDataJSON().target_kind) });
    }
    if (path === '/mail/addresses:set-sender-policy') {
      policy = request.postDataJSON().sender_policy;
      return route.fulfill({ json: addressOf(request.postDataJSON().target_kind) });
    }
    if (path === '/jumble/entries' && request.method() === 'GET') {
      return route.fulfill({ json: { data: [MEANT], page: { next_cursor: null } } });
    }
    if (path === `/jumble/entries/${MEANT.id}:convert`) {
      return route.fulfill({ json: { ...MEANT, status: 'PROCESSED', target_item_id: SORTED_ITEM } });
    }
    return stub(route);
  });
  return { context, page, failures, writes };
}

for (const [width, size] of [[1280, 'large'], [375, 'compact']]) {
  test(`chromium: ${size} — the jumble's address, and a mail meant for a collection sorted in at once`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    const { context, page, failures, writes } = await open(browser, width);
    try {
      await page.goto(`${served.origin}/jumble`);
      const line = page.locator('[data-mail-address]').first();
      await line.getByText('jumble.aaaaaaaaaaaaaaaa@in.example.test').waitFor({ timeout: 10_000 });
      await line.getByRole('button', { name: 'Copy' }).waitFor();

      await page.getByRole('button', { name: 'Sort into Kitchen' }).click();
      await page.waitForURL(`**/items/${SORTED_ITEM}`, { timeout: 10_000 });
      const sorted = writes.filter((each) => each.call === `POST /jumble/entries/${MEANT.id}:convert`);
      assert.equal(sorted.length, 1);
      assert.equal(sorted[0].body.collection_id, COLLECTION.id, 'sorted into the place it was meant for');
      assert.deepEqual(failures, []);
    } finally {
      await context.close();
      await browser.close();
    }
  });

  test(`chromium: ${size} — a collection's address: in its settings with who may send and a new address, in its menu, and an entry's line`, { timeout: 90_000 }, async () => {
    const browser = await chromium.launch();
    const { context, page, failures, writes } = await open(browser, width);
    try {
      await page.goto(`${served.origin}/collections/${COLLECTION.id}/settings/general`);
      const section = page.locator('[data-mail-settings]');
      await section.getByText('kitchen.aaaaaaaaaaaaaaaa@in.example.test').waitFor({ timeout: 10_000 });

      await section.getByText('Anyone with the address').click();
      await page.waitForTimeout(400);
      const set = writes.filter((each) => each.call === 'POST /mail/addresses:set-sender-policy');
      assert.equal(set.length, 1);
      assert.deepEqual(set[0].body, { target_kind: 'COLLECTION', target_id: COLLECTION.id, sender_policy: 'ANYONE' });

      await section.getByRole('button', { name: 'New address' }).click();
      await section.getByText('Mail sent to the current address stops arriving').waitFor();
      await section.getByRole('button', { name: 'Replace the address' }).click();
      await section.getByText('kitchen.bbbbbbbbbbbbbbbb@in.example.test').waitFor({ timeout: 5_000 });
      assert.equal(writes.filter((each) => each.call === 'POST /mail/addresses:rotate').length, 1);

      await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
      await page.getByRole('main').waitFor();
      await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).first().click();
      await page.getByRole('menuitem', { name: 'Copy mail address' }).waitFor({ timeout: 5_000 });
      await page.keyboard.press('Escape');

      await page.goto(`${served.origin}/items/${ITEMS[0].id}`);
      // In the details, which a phone keeps folded until asked: there, not necessarily on screen.
      await page.locator('[data-detail="mail"]').getByText('entry.bbbbbbbbbbbbbbbb@in.example.test').waitFor({ state: 'attached', timeout: 10_000 });
      assert.deepEqual(failures, []);
    } finally {
      await context.close();
      await browser.close();
    }
  });
}

test('chromium: without a receiving domain no address is drawn, and who shapes the place reads why', { timeout: 90_000 }, async () => {
  const browser = await chromium.launch();
  const { context, page, failures } = await open(browser, 1280, { available: false });
  try {
    await page.goto(`${served.origin}/collections/${COLLECTION.id}/settings/general`);
    await page.locator('[data-mail-settings] [data-mail-unavailable]').waitFor({ timeout: 10_000 });
    assert.equal(await page.locator('[data-mail-address]').count(), 0);

    await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
    await page.getByRole('main').waitFor();
    await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).first().click();
    await page.getByRole('menu').waitFor();
    assert.equal(await page.getByRole('menuitem', { name: 'Copy mail address' }).count(), 0);

    await page.goto(`${served.origin}/jumble`);
    await page.getByRole('heading', { name: 'Jumble', level: 1 }).waitFor().catch(() => {});
    await page.waitForTimeout(600);
    assert.equal(await page.locator('[data-mail-address]').count(), 0);
    assert.deepEqual(failures, []);
  } finally {
    await context.close();
    await browser.close();
  }
});
