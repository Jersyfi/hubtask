// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Adding inside an entry from its row, and the entry beside the list. ＋ on a row opens its form
// indented under that row, where the rows it adds will stand, and while it is open the page draws
// no top-level add under it that would read as the form's second button. In the pane, the entry's
// menu stands at the end of its title's line rather than alone above it.
//
// Chromium only: what is walked is layout, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, ITEMS, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
const browser = await chromium.launch();
test.after(async () => {
  await browser.close();
  await served.close();
});

async function collection(width) {
  const session = await signedIn(browser, width, width < 600 ? 844 : 900);
  await session.page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await session.page.locator('[data-row]').first().waitFor();
  return session;
}

for (const width of [390, 1280]) {
  test(`${width} px: a row's add form stands under that row, with no top-level add under it`, async () => {
    const { page, context, failures } = await collection(width);
    const parent = ITEMS.find((each) => each.type === 'TASK' && !each.parent_id);
    const row = page.locator(`[data-row="${parent.id}"]`);
    await row.getByRole('button', { name: `Add something inside ${parent.title}` }).click();
    const form = page.locator(`[data-under="${parent.id}"]`);
    await form.getByRole('textbox').first().waitFor();
    const box = await row.locator('input[type="checkbox"]').boundingBox();
    const field = await form.getByRole('textbox').first().boundingBox();
    assert.ok(field.x > box.x, `the form starts at ${field.x}, its row's box at ${box.x}`);
    // The head's own add stays (pinned on a phone); nothing else adds at the top level.
    assert.equal(await page.locator('main').getByRole('button', { name: 'Add an entry', exact: true }).count(), 1);
    assert.deepEqual(failures, []);
    await context.close();
  });
}

test('1280 px: in the pane, the entry menu stands at the end of the title line', async () => {
  const { page, context, failures } = await collection(1280);
  const entry = ITEMS.find((each) => !each.parent_id);
  await page.locator(`[data-row="${entry.id}"] a`).first().click();
  const pane = page.getByRole('complementary');
  const menu = pane.locator('[data-opener="entry-menu"]');
  await menu.waitFor();
  const title = await pane.getByRole('textbox', { name: 'Title' }).boundingBox();
  const opener = await menu.boundingBox();
  const middle = (each) => each.y + each.height / 2;
  assert.ok(Math.abs(middle(title) - middle(opener)) <= 12, `the menu at ${middle(opener)}, the title at ${middle(title)}`);
  assert.deepEqual(failures, []);
  await context.close();
});
