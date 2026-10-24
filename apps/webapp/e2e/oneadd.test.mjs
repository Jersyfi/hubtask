// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One add (design-system.md §11.8, UC-WRK-26 check 7): capture is the one way an entry is added,
// and every add control is a door into it that knows where it stands. At 390 px a collection pins
// no add of its own - the bottom bar's ＋ opens capture pointed at it - and a list keeps capture
// open for the next line. At 1280 px the head carries *Add an entry*, a collection keeps its add
// under the rows, and both open capture with the collection's chip.
//
// Chromium only: what is walked is layout, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, GROCERIES, resetGroceries, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
const browser = await chromium.launch();
test.after(async () => {
  await browser.close();
  await served.close();
});

async function open(width, path) {
  resetGroceries();
  const session = await signedIn(browser, width, width < 600 ? 844 : 900);
  await session.page.goto(`${served.origin}${path}`);
  await session.page.locator('[data-row]').first().waitFor();
  return session;
}

const adds = (page) => page.locator('main').getByRole('button', { name: 'Add an entry', exact: true });
const dialogOf = (page) => page.getByRole('dialog', { name: 'Capture' });

/** The place capture was opened for, and whether it stays open after Enter. */
async function opened(page) {
  const dialog = dialogOf(page);
  await dialog.getByRole('textbox', { name: 'What do you want to remember?' }).waitFor({ timeout: 5_000 });
  const place = (await dialog.locator('[data-chip="place"]').innerText()).replace(/\s+/gu, ' ');
  const keeps = await dialog.getByRole('checkbox', { name: 'Keep capturing' }).isChecked();
  await page.keyboard.press('Escape');
  await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
  return { place, keeps };
}

for (const [collection, keepsOpen] of [[GROCERIES, true], [COLLECTION, false]]) {
  test(`390 px: ${collection.name} pins no add of its own, and the bottom bar's ＋ opens capture pointed at it`, async () => {
    const { page, context, failures } = await open(390, `/collections/${collection.id}`);
    assert.equal(await adds(page).count(), 0, 'an add pinned over the bottom bar');
    await page.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: 'Capture' }).click();
    const { place, keeps } = await opened(page);
    assert.match(place, new RegExp(collection.name, 'u'));
    assert.equal(keeps, keepsOpen, 'keep capturing where a list is typed line after line');
    assert.deepEqual(failures, []);
    await context.close();
  });
}

test('1280 px: the head carries the add, the end of the list another, and each opens capture there', async () => {
  const list = await open(1280, `/collections/${GROCERIES.id}`);
  assert.equal(await adds(list.page).count(), 2);
  for (const index of [0, 1]) {
    await adds(list.page).nth(index).click();
    assert.match((await opened(list.page)).place, /Groceries/u);
  }
  assert.deepEqual(list.failures, []);
  await list.context.close();

  const collection = await open(1280, `/collections/${COLLECTION.id}`);
  assert.equal(await adds(collection.page).count(), 2);
  for (const index of [0, 1]) {
    await adds(collection.page).nth(index).click();
    assert.match((await opened(collection.page)).place, new RegExp(COLLECTION.name, 'u'));
  }
  assert.deepEqual(collection.failures, []);
  await collection.context.close();
});
