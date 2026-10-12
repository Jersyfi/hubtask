// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The reminder's screen for somebody without a mailbox (UC-ID-20 check 6): a reminder by mail
// whose recipient is a managed account says, under the choice, that mail does not reach them -
// rather than saving a reminder whose mail goes nowhere. A recipient with an address gets no line.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ITEMS, OTHER, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[0];

/** Lena, as `GET /accounts/{id}` answers a managed account: no address, and saying so. */
const LENA = { id: '01a0e2e0-0000-7000-8000-000000000013', kind: 'USER', display_name: 'Lena', status: 'ACTIVE', has_address: false, managed: true };
const MARA = { ...OTHER, has_address: true };

const served = await serve(DIST);
test.after(() => served.close());

const NO_MAILBOX = 'Lena has no mailbox: a reminder by mail does not reach them.';

async function composeFor(browser, assignee) {
  const { page, failures, context } = await signedIn(browser, 1280, 1000);
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (route.request().method() === 'GET' && path.endsWith(`/api/v1/items/${ENTRY.id}`)) {
      return route.fulfill({ json: { ...ENTRY, assignee_id: assignee.id } });
    }
    if (path.endsWith(`/api/v1/accounts/${LENA.id}`)) return route.fulfill({ json: LENA });
    if (path.endsWith(`/api/v1/accounts/${MARA.id}`)) return route.fulfill({ json: MARA });
    return stub(route);
  });
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });
  await page.locator('[data-detail="reminders"]').click();
  const editor = page.getByRole('dialog', { name: 'Reminders' });
  await editor.waitFor({ timeout: 5_000 });
  await editor.getByRole('button', { name: 'Add a reminder' }).click();
  await editor.getByText('Who is reminded').first().waitFor({ timeout: 5_000 });
  return { page, editor, failures, close: () => context.close() };
}

test('a reminder by mail to an assignee without a mailbox says it does not reach them', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { editor, failures, close } = await composeFor(browser, LENA);
  t.after(close);

  await editor.getByText(NO_MAILBOX).waitFor({ timeout: 5_000 })
    .catch(() => assert.fail('no line says the reminder by mail does not reach Lena'));
  assert.deepEqual(failures, []);
});

test('a reminder by mail to an assignee with an address says nothing of a mailbox', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { editor, failures, close } = await composeFor(browser, MARA);
  t.after(close);

  // Her account is read before the line could appear; only then is its absence an answer.
  await editor.getByText(MARA.display_name).first().waitFor({ timeout: 5_000 });
  assert.equal(await editor.getByText('no mailbox', { exact: false }).count(), 0);
  assert.deepEqual(failures, []);
});
