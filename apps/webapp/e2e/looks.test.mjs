// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Hubs and collections in their own colour and symbol (UC-WRK-33 check 1): offered when creating
// and when editing, from the ten label colours and the symbol set, each option named in words; and
// drawn in the navigation, in the page head and next to entries in lists that mix collections.
// Chromium, over the shared fixture's places of their own, which read back what was written.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { HUB, LOOK_COLLECTION, LOOK_HUB, LOOK_PRIVATE_HUB, PLAIN_COLLECTION, showLooks, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
showLooks();

const LABEL_COLOURS = ['Slate', 'Blue', 'Teal', 'Green', 'Lime', 'Amber', 'Orange', 'Red', 'Magenta', 'Violet'];

/** A navigation row's tile colour, or `null` where it draws its mark without one. */
const tileOf = (page, id) => page.locator(`[data-node="${id}"] .mark .tile`).first().getAttribute('data-token', { timeout: 2_000 }).catch(() => null);

/** Presses an option by its face, the tile a person presses; the radio under it is what is named. */
async function choose(dialog, name) {
  const radio = dialog.getByRole('radio', { name, exact: true });
  await radio.locator('xpath=..').click();
  assert.ok(await radio.isChecked(), name);
}

async function at(browser, width, path) {
  const tab = await signedIn(browser, width, 900);
  await tab.page.goto(`${served.origin}${path}`);
  await tab.page.locator('nav[aria-label="Workspace"]').first().waitFor({ state: 'attached', timeout: 15_000 });
  return tab;
}

test('chromium: 1280 px — the navigation draws each place in its own colour, and one nobody chose for plainly', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await at(browser, 1280, `/collections/${LOOK_COLLECTION.id}`);
  t.after(() => context.close());

  await page.locator(`[data-node="${LOOK_COLLECTION.id}"]`).waitFor({ timeout: 10_000 });
  assert.equal(await tileOf(page, LOOK_HUB.id), 'violet');
  assert.equal(await tileOf(page, LOOK_COLLECTION.id), 'amber');
  assert.equal(await tileOf(page, PLAIN_COLLECTION.id), null, 'a collection nobody chose a colour for');
  assert.equal(await tileOf(page, HUB.id), null, 'a hub nobody chose a colour for');
  // A private hub with a symbol of its own keeps saying that it is private - in words.
  assert.equal((await page.locator(`[data-node="${LOOK_PRIVATE_HUB.id}"] .meta`).textContent()).trim(), 'Private');
  // The colour never names the row: its name does.
  assert.equal(await page.locator(`[data-node="${LOOK_COLLECTION.id}"]`).getAttribute('aria-current'), 'page');
  assert.match(await page.locator(`[data-node="${LOOK_COLLECTION.id}"]`).textContent(), /Clients/);

  // The page head: the same mark, before the title, decorative beside it.
  const head = page.locator('header .title-mark');
  assert.equal(await head.getAttribute('aria-hidden'), 'true');
  assert.equal(await head.locator('[data-token]').getAttribute('data-token'), 'amber');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — creating a hub offers the ten colours and the symbols by name, and sends the choice', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await at(browser, 1280, '/');
  t.after(() => context.close());

  const sent = [];
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith('/api/v1/containers')) sent.push(request.postDataJSON());
  });

  await page.getByRole('button', { name: 'Create hub' }).first().click();
  const dialog = page.getByRole('dialog', { name: 'Create hub' });
  await dialog.waitFor({ timeout: 5_000 });

  // Rule 3: every colour has a name in words, and "none" comes first.
  const colours = dialog.getByRole('group', { name: 'Colour' }).getByRole('radio');
  assert.deepEqual(await colours.evaluateAll((radios) => radios.map((radio) => radio.getAttribute('aria-label'))), ['No colour', ...LABEL_COLOURS]);
  const symbols = dialog.getByRole('group', { name: 'Symbol' }).getByRole('radio');
  assert.equal(await symbols.first().getAttribute('aria-label'), 'Default symbol');
  assert.ok((await symbols.count()) > 20, 'a set to choose from');
  assert.ok(await dialog.getByRole('radio', { name: 'No colour' }).isChecked(), 'nothing is chosen for the person');

  await dialog.getByRole('textbox', { name: 'Hub name' }).fill('Garden');
  await choose(dialog, 'Teal');
  await choose(dialog, 'Leaf');
  await dialog.getByRole('button', { name: 'Create hub' }).click();
  await dialog.waitFor({ state: 'detached', timeout: 5_000 });

  assert.equal(sent.length, 1);
  assert.equal(sent[0].name, 'Garden');
  assert.equal(sent[0].color_token, 'teal');
  assert.equal(sent[0].icon, 'leaf');

  // Read back: the new hub's row and its page head carry the choice.
  await page.getByRole('heading', { name: 'Garden', level: 1 }).waitFor({ timeout: 10_000 });
  assert.equal(await page.locator('header .title-mark [data-token]').getAttribute('data-token'), 'teal');
  const row = page.locator('nav[aria-label="Workspace"] [role="treeitem"]', { hasText: 'Garden' });
  await row.waitFor({ timeout: 5_000 });
  assert.equal(await row.locator('.tile').getAttribute('data-token'), 'teal');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — editing changes the colour and the symbol, and "none" clears the colour', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await at(browser, 1280, `/hubs/${LOOK_HUB.id}`);
  t.after(() => context.close());
  await page.getByRole('heading', { name: LOOK_HUB.name, level: 1 }).waitFor({ timeout: 10_000 });

  const sent = [];
  page.on('request', (request) => {
    if (request.method() === 'PATCH' && request.url().endsWith(`/api/v1/containers/${LOOK_HUB.id}`)) sent.push({ ...request.postDataJSON(), ifMatch: request.headers()['if-match'] });
  });

  await page.getByRole('button', { name: `Actions for ${LOOK_HUB.name}` }).click();
  await page.getByRole('menuitem', { name: 'Edit' }).click();
  const dialog = page.getByRole('dialog', { name: 'Edit hub' });
  await dialog.waitFor({ timeout: 5_000 });
  // It opens on what the hub has now.
  assert.ok(await dialog.getByRole('radio', { name: 'Violet' }).isChecked());
  assert.ok(await dialog.getByRole('radio', { name: 'Briefcase' }).isChecked());

  await choose(dialog, 'No colour');
  await choose(dialog, 'House');
  await dialog.getByRole('button', { name: 'Save' }).click();
  await dialog.waitFor({ state: 'detached', timeout: 5_000 });

  // With the version the dialog opened on, so a change that lost a race is refused (ADR-0025).
  assert.equal(sent.length, 1);
  assert.deepEqual({ icon: sent[0].icon, color_token: sent[0].color_token }, { icon: 'house', color_token: null });
  assert.ok(sent[0].ifMatch?.includes(String(LOOK_HUB.version)), sent[0].ifMatch);
  await page.waitForFunction((id) => !document.querySelector(`[data-node="${id}"] .tile`), LOOK_HUB.id, { timeout: 5_000 });
  assert.equal(await page.locator('header .title-mark [data-token]').count(), 0, 'no colour in the head');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — Today and search say where each entry lives, in the place’s colour', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await at(browser, 1280, '/');
  t.after(() => context.close());

  // Today names the hub first, in the hub's colour, then the collection in its own
  // (UC-WRK-34 check 1, UC-WRK-33 check 1).
  const mine = page.locator('section[aria-labelledby="today-mine"]');
  const tokens = (place) => place.locator('[data-token]').evaluateAll((marks) => marks.map((mark) => mark.getAttribute('data-token')));
  const clients = mine.locator('.place', { hasText: LOOK_COLLECTION.name });
  await clients.first().waitFor({ timeout: 10_000 });
  assert.deepEqual(await tokens(clients.first()), ['violet', 'amber']);
  const invoices = mine.locator('.place', { hasText: PLAIN_COLLECTION.name });
  await invoices.first().waitFor({ timeout: 5_000 });
  assert.deepEqual(await tokens(invoices.first()), ['violet'], 'a collection without a colour draws one');

  await page.goto(`${served.origin}/search`);
  await page.getByRole('searchbox', { name: 'Search the entries you can see' }).or(page.getByRole('textbox', { name: 'Search the entries you can see' })).first().fill('offer');
  const hit = page.locator('.place', { hasText: LOOK_COLLECTION.name }).first();
  await hit.waitFor({ timeout: 10_000 });
  assert.equal(await hit.locator('[data-token]').getAttribute('data-token'), 'amber');
  assert.deepEqual(failures, []);
});
