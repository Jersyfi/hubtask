// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A list's screen as UC-WRK-25 draws it, at a phone's width and a desk's. A line of a list is one
// box and its words - no twist and no type mark beside the box - and it starts near the page's
// edge rather than two empty columns in. The toolbar stands on the heading's edge. *Recently* ends
// the list; while capture holds words in a list, what they match of it is offered right under the
// field, so one press brings an entry back where its name was being typed (check 4).
//
// Chromium only: what is walked is layout, with no engine-specific part.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { GROCERIES, GROCERY_ITEMS, resetGroceries, signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
const browser = await chromium.launch();
test.after(async () => {
  await browser.close();
  await served.close();
});

async function list(width) {
  resetGroceries();
  const session = await signedIn(browser, width, width < 600 ? 844 : 900);
  await session.page.goto(`${served.origin}/collections/${GROCERIES.id}`);
  await session.page.locator('[data-row]').first().waitFor();
  return session;
}

for (const width of [390, 1280]) {
  test(`${width} px: a line is one box and its words, near the edge`, async () => {
    const { page, context, failures } = await list(width);
    const rows = await page.locator('[data-row]').evaluateAll((each) =>
      each.map((row) => ({
        boxes: row.querySelectorAll('input[type="checkbox"]').length,
        marks: row.querySelectorAll('.mark, .twist').length,
        inset: row.querySelector('input[type="checkbox"]').getBoundingClientRect().x - row.getBoundingClientRect().x,
      })),
    );
    assert.equal(rows.length, GROCERY_ITEMS.length);
    for (const row of rows) {
      assert.equal(row.boxes, 1);
      assert.equal(row.marks, 0);
      // The grip, where there is one, and the row's own padding - not two empty columns more.
      assert.ok(row.inset <= 40, `the box starts ${row.inset} px into its row`);
    }
    assert.deepEqual(failures, []);
    await context.close();
  });

  test(`${width} px: the toolbar stands on the heading's edge`, async () => {
    const { page, context, failures } = await list(width);
    const tools = await page.getByRole('toolbar', { name: 'What is shown of the entries' }).boundingBox();
    const edge = await page.locator('main header').first().boundingBox();
    assert.ok(Math.abs(tools.x - edge.x) <= 1, `the toolbar at ${tools.x}, the head at ${edge.x}`);
    assert.deepEqual(failures, []);
    await context.close();
  });

  test(`${width} px: what capture asks for of *Recently* is offered right under the field (check 4)`, async () => {
    const { page, context, failures } = await list(width);
    const heads = () => page.locator('h2.section-head').allTextContents();
    assert.equal((await heads()).at(-1)?.trim(), 'Recently', 'Recently ends the list');
    await page.locator('body').click({ position: { x: 1, y: 300 } });
    await page.keyboard.press('q');
    const dialog = page.getByRole('dialog', { name: 'Capture' });
    await dialog.getByRole('textbox', { name: 'What do you want to remember?' }).fill('oat');
    const before = dialog.getByRole('region', { name: 'Ticked before' });
    await before.waitFor({ timeout: 5_000 });
    const offered = (await before.getByRole('button').allTextContents()).map((each) => each.trim());
    assert.ok(offered.length > 0 && offered.every((each) => /oat/iu.test(each)), `offered ${offered}`);
    assert.deepEqual(failures, []);
    await context.close();
  });
}
