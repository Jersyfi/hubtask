// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-SYN-01 check 6 in the browser: a reminder set without a connection is kept, counted in the
// bar, and pushed as a REMINDER_CREATE under an identifier the browser minted once the connection
// returns - where the web app used to answer `sync.needs_connection`.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { ITEMS, pushed, signedIn } from './fixture.mjs';
import { unfoldFields } from './reveal.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const DATED = ITEMS.find((each) => each.due_at) ?? ITEMS[1];

const served = await serve(DIST);
const browser = await chromium.launch();
test.after(async () => {
  await browser.close();
  await served.close();
});

test('offline, a reminder is kept and sent when the network returns (check 6)', async () => {
  const { page, context } = await signedIn(browser, 1280, 1000);
  pushed.length = 0;
  await page.goto(`${served.origin}/items/${DATED.id}`);
  await page.getByRole('textbox', { name: 'Title' }).first().waitFor({ timeout: 15_000 });
  await unfoldFields(page);
  const row = page.locator('button[data-detail="reminders"]');
  await row.waitFor({ timeout: 15_000 });

  let isOffline = false;
  await context.route('**/api/v1/**', (route) => (isOffline ? route.abort('internetdisconnected') : route.fallback()));
  isOffline = true;
  await context.setOffline(true);
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') === 'offline', null, { timeout: 5_000 });

  await row.click();
  await page.getByRole('button', { name: 'Add a reminder' }).click();
  const save = page.getByRole('button', { name: 'Save the reminder' });
  await save.waitFor();
  await save.focus();
  await page.keyboard.press('Enter');
  // The mark counts it, and says so to a screen reader (SyncStatus): kept, not refused.
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('aria-label')?.includes('1 change is waiting to be sent'), null, { timeout: 5_000 })
    .catch(() => assert.fail('the reminder was not kept as a waiting change'));
  assert.equal(await page.getByText('needs a connection', { exact: false }).count(), 0, 'the reminder was refused offline');

  isOffline = false;
  await context.setOffline(false);
  await page.waitForFunction(() => document.querySelector('.trigger')?.getAttribute('data-connection') !== 'offline', null, { timeout: 10_000 });
  await page.waitForFunction(() => !document.querySelector('.trigger')?.getAttribute('aria-label')?.includes('waiting to be sent'), null, { timeout: 10_000 })
    .catch(() => assert.fail('the waiting reminder was never sent'));
  const mutations = pushed.flatMap((push) => push.mutations ?? []);
  const reminder = mutations.find((mutation) => mutation.kind === 'REMINDER_CREATE');
  assert.ok(reminder, `no reminder was pushed: ${JSON.stringify(mutations)}`);
  assert.equal(reminder.item_id, DATED.id);
  assert.match(String(reminder.entity_id), /^[0-9a-f]{8}-[0-9a-f]{4}-7/, 'the browser minted a UUIDv7 for it');
  assert.ok(typeof reminder.payload?.offset_spec === 'string' && reminder.hlc, `pushed ${JSON.stringify(reminder)}`);
  await context.close();
});
