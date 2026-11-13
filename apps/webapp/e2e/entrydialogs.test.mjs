// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The dialogs an entry's ⋯ opens hold to the page system's dialog ladder (page-system concept §5.1,
// work surface family 3.1): each takes its rung's size, holds no table, no tabs and no second
// dialog, and a Dialog S or M does not scroll at 1280×800. On a phone M and L are full screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';
import { chromium } from 'playwright';
import { serve } from './serve.mjs';
import { ITEMS, signedIn, stub } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const DATED = ITEMS.find((each) => each.due_at);

/** The ⋯ item and the rung its dialog takes. */
const RUNGS = [
  ['Change the due date', 'm'],
  ['Person', 'm'],
  ['Labels', 'm'],
  ['Move to another collection', 's'],
  ['Duplicate', 'm'],
  ['Split a part off', 'l'],
  ['Merge into another', 'l'],
  ['Share this entry', 'l'],
];

async function shapeOf(page) {
  const dialog = page.locator('dialog[open]').last();
  await dialog.waitFor({ timeout: 5_000 });
  await page.waitForTimeout(300);
  return dialog.evaluate((node) => {
    const body = node.querySelector('.body');
    const box = node.getBoundingClientRect();
    return {
      size: node.getAttribute('data-size'),
      inside: node.querySelectorAll('table, [role="table"], [role="grid"], [role="tablist"], dialog[open]').length,
      scrolls: body ? body.scrollHeight > body.clientHeight + 1 : false,
      width: Math.round(box.width),
    };
  });
}

for (const [width, height] of [[1280, 800], [390, 844]]) {
  test(`chromium ${width}×${height}: the entry's dialogs take their rung of the ladder`, { timeout: 120_000 }, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    const { page, context, failures } = await signedIn(browser, width, height);
    t.after(() => context.close());
    await context.unroute('**/api/v1/**');
    await context.route('**/api/v1/**', (route) => stub(route));
    await page.goto(`${served.origin}/items/${DATED.id}`);
    await page.getByRole('heading', { name: DATED.title }).first().waitFor({ timeout: 15_000 });
    await page.waitForTimeout(600);
    for (const [item, rung] of RUNGS) {
      await page.locator('[data-opener="entry-menu"]').first().click();
      await page.getByRole('menuitem', { name: item }).click();
      const shape = await shapeOf(page);
      assert.equal(shape.size, rung, `${item}: the dialog is ${shape.size}, not ${rung}`);
      assert.equal(shape.inside, 0, `${item}: the dialog holds a table, tabs or a second dialog`);
      if (width >= 1280 && rung !== 'l') assert.equal(shape.scrolls, false, `${item}: the dialog scrolls at 1280×800`);
      if (width < 600 && rung !== 's') assert.ok(shape.width >= width - 1, `${item}: a ${rung} dialog is not full screen on a phone (${shape.width})`);
      await page.keyboard.press('Escape');
      await page.locator('dialog[open]').waitFor({ state: 'detached', timeout: 5_000 }).catch(() => {});
      if (await page.locator('dialog[open]').count() > 0) {
        await page.locator('dialog[open]').last().getByRole('button', { name: /Cancel|Close|Discard/ }).first().click();
      }
    }
    assert.deepEqual(failures, []);
  });
}

// The dates dialog's two clears do one kind of thing, so they wear one style (finding of the
// family 3.1 review): the due date's and the start's, side by side on a phone.
test('chromium 390×844: the dates dialog clears the due date and the start in one button style', async (t) => {
  // The fixture's entry, with a start for this walk only: the shared fixture answers every read of it.
  DATED.start_at = DATED.due_at.replace(/T.*/, 'T07:00:00Z');
  t.after(() => { delete DATED.start_at; });
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 390, 844);
  t.after(() => context.close());
  await context.unroute('**/api/v1/**');
  await context.route('**/api/v1/**', (route) => stub(route));
  await page.goto(`${served.origin}/items/${DATED.id}`);
  await page.getByRole('heading', { name: DATED.title }).first().waitFor({ timeout: 15_000 });
  await page.waitForTimeout(600);
  await page.locator('[data-opener="entry-menu"]').first().click();
  await page.getByRole('menuitem', { name: 'Change the due date' }).click();
  const dialog = page.locator('dialog[open]').last();
  const clears = [dialog.getByRole('button', { name: 'Clear the due date' }), dialog.getByRole('button', { name: 'Clear the start' })];
  await clears[1].waitFor({ timeout: 5_000 });
  const looks = await Promise.all(clears.map((each) => each.evaluate((node) => `${node.dataset.tone}/${node.dataset.size}`)));
  assert.equal(looks[0], looks[1], `the clears wear two styles: ${looks.join(' and ')}`);
  assert.deepEqual(failures, []);
});
