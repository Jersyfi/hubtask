// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The detail pane in the collection (F9-09, ADR-0061 decision 4): a place, not a feature. From
// `large` a row opens the entry beside the list - the address gains `?item=`, the row stays
// current and keeps the focus, the pane holds the entry's own form, Escape closes it through the
// register and focus returns to the row, "open as a page" goes to the entry's address. Below
// `large` the same address is a redirect to the entry's page, replacing the one it left from.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { COLLECTION, ITEMS, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const ENTRY = ITEMS[1];

const served = await serve(DIST);
test.after(() => served.close());

test('chromium: 1280 px — a row opens beside the list, stays current and focused; Escape closes; the page is a link away', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  const row = page.getByRole('link', { name: ENTRY.title });
  await row.waitFor({ timeout: 15_000 });

  // A plain press opens beside: the address, the pane, the row current, focus still on the row.
  await row.focus();
  await page.keyboard.press('Enter');
  const pane = page.getByRole('complementary', { name: ENTRY.title });
  await pane.waitFor({ timeout: 5_000 });
  assert.equal(new URL(page.url()).search, `?item=${ENTRY.id}`);
  assert.equal(new URL(page.url()).pathname, `/collections/${COLLECTION.id}`);
  assert.equal(await row.getAttribute('aria-current'), 'true');
  assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), ENTRY.title, 'focus left the list');
  assert.equal(await page.getByRole('heading', { level: 1 }).count(), 1, 'the pane brought a second h1');
  // The link is still a link to the entry's page.
  assert.equal(await row.getAttribute('href'), `/items/${ENTRY.id}`);

  // The entry's own form is inside: the title in place, the chips, the details rows, the tabs.
  assert.equal(await pane.getByRole('textbox', { name: 'Title' }).inputValue(), ENTRY.title);
  assert.equal(await pane.locator('[data-detail="due"]').count(), 1);
  assert.equal(await pane.getByRole('tab', { name: /Comments/ }).count(), 1);
  await pane.locator('[data-detail="labels"]').click();
  await page.getByRole('dialog', { name: 'Labels' }).waitFor({ timeout: 5_000 });
  // Escape closes the editor first, then the pane - one layer at a time (the register).
  await page.keyboard.press('Escape');
  await page.getByRole('dialog', { name: 'Labels' }).waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(await pane.count(), 1, 'the first Escape closed the pane as well as the editor');
  await page.keyboard.press('Escape');
  await pane.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(new URL(page.url()).search, '');
  await page.waitForFunction((title) => document.activeElement?.textContent?.trim() === title, ENTRY.title, { timeout: 5_000 })
    .catch(async () => assert.fail(`focus did not return to the row but sits on ${await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80))}`));
  assert.equal(await row.getAttribute('aria-current'), null);

  // The pane's own controls: close, and open as a page.
  await row.click();
  await pane.waitFor({ timeout: 5_000 });
  await pane.getByRole('button', { name: 'Close the pane' }).click();
  await pane.waitFor({ state: 'hidden', timeout: 5_000 });
  await row.click();
  await pane.waitFor({ timeout: 5_000 });
  await pane.getByRole('link', { name: 'Open as a page' }).click();
  await page.waitForURL(`**/items/${ENTRY.id}`, { timeout: 5_000 });
  assert.equal(await page.getByRole('heading', { level: 1 }).count(), 1);
  // The back button returns to the collection with the pane, because opening was a navigation.
  await page.goBack();
  await page.waitForURL(`**/collections/${COLLECTION.id}?item=${ENTRY.id}`, { timeout: 5_000 });
  await page.getByRole('complementary', { name: ENTRY.title }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 905 px — the same address is a redirect to the entry’s page, and the back button skips it', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 905, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('link', { name: ENTRY.title }).waitFor({ timeout: 15_000 });
  // Below `large` a row is the link it always was: it goes to the entry's page.
  await page.getByRole('link', { name: ENTRY.title }).click();
  await page.waitForURL(`**/items/${ENTRY.id}`, { timeout: 5_000 });
  // The pane's address, arrived at directly, is replaced by the page's.
  await page.goto(`${served.origin}/collections/${COLLECTION.id}?item=${ENTRY.id}`);
  await page.waitForURL(`**/items/${ENTRY.id}`, { timeout: 10_000 });
  assert.equal(await page.getByRole('complementary', { name: ENTRY.title }).count(), 0);
  assert.equal(await page.getByRole('textbox', { name: 'Title' }).first().inputValue(), ENTRY.title);
  assert.deepEqual(failures, []);
});
