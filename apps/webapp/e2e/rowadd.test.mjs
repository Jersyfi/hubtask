// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Adding inside an entry from its row, and the entry beside the list. ＋ on a row opens capture
// pointed under that row - its chip names the entry, and its × takes the new entry out from under
// it - and creates the entry by its parent. In the pane, the entry's menu stands at the end of its
// title's line rather than alone above it.
//
// Chromium only: what is walked is layout, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { COLLECTION, ITEMS, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';
import { reveal } from './reveal.mjs';

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
  test(`${width} px: a row's ＋ opens capture under that row, and the entry is made by its parent`, async () => {
    const { page, context, failures } = await collection(width);
    const parent = ITEMS.find((each) => each.type === 'TASK' && !each.parent_id);
    const created = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/api/v1/items')) created.push(request.postDataJSON());
    });
    const row = page.locator(`[data-row="${parent.id}"]`);
    await reveal(row.getByRole('button', { name: `Add something inside ${parent.title}` }));
    const dialog = page.getByRole('dialog', { name: 'Capture' });
    const field = dialog.getByRole('textbox', { name: 'What do you want to remember?' });
    await field.waitFor({ timeout: 5_000 });
    assert.match(await dialog.locator('[data-chip="parent"]').innerText(), new RegExp(parent.title, 'u'));
    assert.equal(await dialog.getByRole('button', { name: `Not under ${parent.title}` }).count(), 1, 'no way out from under it');
    await field.fill('Grout the corner');
    await field.press('Enter');
    for (let waited = 0; waited < 30 && created.length === 0; waited++) await page.waitForTimeout(100);
    assert.equal(created[0]?.parent_id, parent.id, `created ${JSON.stringify(created)}`);
    assert.equal('collection_id' in (created[0] ?? {}), false);
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
