// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What takes effect at once offers *Undo* (UC-WRK-28 check 3): moving an entry to the trash and
// archiving it from its menu, taking a label off an entry, deleting a label from the collection and
// a bulk move to the trash. Each message still stands after eight seconds - the frame's dwell for a
// message that offers something - and *Undo* sends the inverse write. Offline, *Undo* is shown and
// says it waits. Chromium only; the clock is Playwright's, so eight seconds take none.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { CHILDREN, COLLECTION, ITEMS, LABELS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The collection's list with every write recorded, and the page's clock in the walk's hand. */
async function list(browser) {
  const written = [];
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  await page.clock.install();
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() !== 'GET' && !/(items:query|sync:pull|sync:snapshot|\/search)$/.test(path)) {
      written.push({ method: request.method(), path, body: request.postDataJSON() });
    }
    return stub(route);
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator(`[data-row="${ITEMS[2].id}"]`).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  return { page, context, failures, written, unstubbed };
}

/**
 * Does `act`, then presses *Undo* on the message that says `text` once it has stood for eight
 * seconds. The clock is paused before the act, so the eight seconds are exactly that from the
 * moment the message arrived; it runs again afterwards. The pointer is kept off the message: a
 * pointer over it holds it.
 */
async function undoAfterEightSeconds(page, act, text) {
  await page.mouse.move(1, 1);
  await page.clock.pauseAt(new Date((await page.evaluate(() => Date.now())) + 50));
  await act();
  const message = page.locator('.toast', { hasText: text });
  await message.waitFor({ timeout: 5_000 });
  await page.mouse.move(1, 1);
  await page.clock.runFor(8_000 - 100);
  assert.equal(await message.count(), 1, `"${text}" left before eight seconds`);
  await message.getByRole('button', { name: 'Undo' }).click();
  await page.clock.resume();
}

/** Lets the confirmation of an undo leave on its own, with nothing holding it. */
async function settle(page) {
  await page.mouse.move(1, 1);
  await page.clock.runFor(10_000);
  await page.locator('.toast').first().waitFor({ state: 'detached', timeout: 5_000 });
}

test('chromium: 1280 px — the trash and the archive from an entry\'s menu take effect at once and offer Undo', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written, unstubbed } = await list(browser);
  t.after(() => context.close());
  const entry = ITEMS[2];
  const menu = page.getByRole('button', { name: `Actions for ${entry.title}`, exact: true }).first();

  await menu.click();
  await undoAfterEightSeconds(page, () => page.getByRole('menuitem', { name: 'Move to the trash' }).click(), `${entry.title} is in the trash.`);
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'DELETE' && w.path.endsWith(`/items/${entry.id}`)), 'the trash was not at once');
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/items/${entry.id}:restore`)), 'Undo sent no :restore');
  await settle(page);

  await menu.click();
  await undoAfterEightSeconds(page, () => page.getByRole('menuitem', { name: 'Archive' }).click(), `${entry.title} is archived.`);
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/items/${entry.id}:unarchive`)), 'Undo sent no :unarchive');
  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — a label taken off an entry and a label deleted from the collection come back by Undo', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await list(browser);
  t.after(() => context.close());
  const tagged = ITEMS[0];
  const label = LABELS[0];

  // A chip taken off a row.
  await undoAfterEightSeconds(
    page,
    () => page.locator(`[data-row="${tagged.id}"]`).first().getByRole('button', { name: `Take ${label.name} off ${tagged.title}` }).click(),
    `The label is off ${tagged.title}.`,
  );
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'DELETE' && w.path.endsWith(`/items/${tagged.id}/labels/${label.id}`)), 'the label was not taken off at once');
  assert.ok(written.some((w) => w.method === 'PUT' && w.path.endsWith(`/items/${tagged.id}/labels/${label.id}`)), 'Undo did not put it back');
  await settle(page);

  // A label deleted from the collection, from *Settings → Labels*: a page, not a modal, so the
  // message stands with its Undo at once beside the list of deleted labels.
  // The labels are a section of the settings page (design-system.md §11.8).
  await page.getByRole('button', { name: `Settings of ${COLLECTION.name}` }).click();
  await page.getByRole('link', { name: 'Labels' }).click();
  await undoAfterEightSeconds(page, async () => {
    await page.getByRole('button', { name: `Delete ${label.name}` }).click();
  }, `${label.name} is deleted.`);
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'DELETE' && w.path.endsWith(`/labels/${label.id}`)), 'the label was not deleted at once');
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/labels/${label.id}:restore`)), 'Undo sent no :restore');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — *Settings → Labels* lists a deleted label with the way back', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await list(browser);
  t.after(() => context.close());
  const gone = { id: '01a0e2e0-0000-7000-8000-0000000000e9', collection_id: COLLECTION.id, name: 'Waiting', color_token: 'amber', description: null, version: 2, deleted_at: '2026-10-20T09:00:00Z' };
  await context.route('**/api/v1/containers/*/labels?include_deleted=true', (route) => route.fulfill({ json: [...LABELS, gone] }));

  await page.getByRole('button', { name: `Settings of ${COLLECTION.name}` }).click();
  await page.getByRole('link', { name: 'Labels' }).click();
  await page.getByRole('heading', { name: 'Deleted labels' }).waitFor({ timeout: 5_000 });
  await page.getByRole('button', { name: `Bring ${gone.name} back` }).click();
  await page.waitForTimeout(300);
  assert.ok(written.some((w) => w.method === 'POST' && w.path.endsWith(`/labels/${gone.id}:restore`)), 'Bring back sent no :restore');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — a selection moved to the trash comes back as one atomic bulk, the parent before its child', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures, written } = await list(browser);
  t.after(() => context.close());
  const parent = ITEMS[0];
  const child = CHILDREN[parent.id][0];

  await page.getByRole('button', { name: 'Show what is inside', exact: true }).click();
  await page.locator(`[data-row="${child.id}"] a`).first().waitFor({ timeout: 5_000 });
  // Picked child first, so the order back has to be the hierarchy's and not the selection's.
  await page.locator(`[data-row="${child.id}"] a`).first().click({ modifiers: ['ControlOrMeta'] });
  await page.locator(`[data-row="${parent.id}"] a`).first().click({ modifiers: ['ControlOrMeta'] });
  await page.getByRole('combobox', { name: 'Do this' }).selectOption({ label: 'Move to the trash' });
  await undoAfterEightSeconds(page, async () => {
    await page.getByRole('button', { name: 'Apply' }).click();
    assert.equal(await page.getByRole('dialog').count(), 0, 'the trash asked first');
  }, '2 entries are in the trash.');
  await page.waitForTimeout(300);
  const bulks = written.filter((w) => w.method === 'POST' && w.path.endsWith('/items:bulk'));
  const back = bulks.at(-1)?.body;
  assert.equal(back?.atomic, true, 'the way back is not all or nothing');
  assert.deepEqual(back?.operations?.map((each) => [each.op, each.item_id]), [['RESTORE_ITEM', parent.id], ['RESTORE_ITEM', child.id]]);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — offline, Undo is shown and says it waits for the network', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await list(browser);
  t.after(() => context.close());
  const entry = ITEMS[3];
  await page.getByRole('button', { name: `Actions for ${entry.title}`, exact: true }).first().click();
  await page.getByRole('menuitem', { name: 'Archive' }).click();
  const message = page.locator('.toast', { hasText: `${entry.title} is archived.` });
  await message.waitFor({ timeout: 5_000 });
  await context.setOffline(true);
  await page.evaluate(() => window.dispatchEvent(new Event('offline')));
  await page.waitForFunction(() => document.querySelector('.toast button[aria-disabled="true"]') !== null, undefined, { timeout: 10_000 });
  assert.equal(await message.getByRole('button', { name: 'Undo' }).getAttribute('aria-disabled'), 'true');
  await context.setOffline(false);
  assert.deepEqual(failures, []);
});
