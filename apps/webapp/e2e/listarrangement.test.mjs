// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One list for everything (concept round 2, §2): the fields as columns under their heads, *Fields*
// and *Group* in the toolbar, what is done gathered at the end - and the device keeping it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, ITEMS, signedIn } from './fixture.mjs';

const served = await serve(join(dirname(fileURLToPath(import.meta.url)), '..', 'dist'));
test.after(() => served.close());

test('chromium: 1280 px - fields stand in columns, Group groups by column, done goes to the end, and it is kept', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator('[data-row]').first().waitFor({ timeout: 15_000 });

  // The template's fields, each under its head, at one inline position in every row.
  const heads = (await page.locator('.fields-head .col-head').allInnerTexts()).map((each) => each.trim().toLowerCase());
  assert.deepEqual(heads, ['title', 'due', 'people', 'labels', 'parts']);
  const starts = await page.locator('[data-row] .cell[data-field="labels"]').evaluateAll((cells) => cells.map((cell) => Math.round(cell.getBoundingClientRect().left)));
  assert.equal(new Set(starts).size, 1, `the labels column is not straight: ${starts}`);

  // *Fields* takes one out.
  await page.getByRole('button', { name: 'Fields' }).click();
  await page.getByRole('menuitem', { name: 'People' }).click();
  assert.equal(await page.locator('[data-row] .cell[data-field="people"]').count(), 0, 'the column stayed');

  // *Group* by column: one head per column with its count, the column-less last.
  await page.getByRole('button', { name: 'Group' }).click();
  await page.getByRole('menuitem', { name: 'Column' }).click();
  const groups = await page.locator('.level h2.group-head > span:first-child').allInnerTexts();
  assert.deepEqual(groups, [...BUCKETS.map((bucket) => bucket.name).filter((name) => groups.includes(name)), 'No column']);
  assert.equal(await page.locator('[data-grip]').count(), 0, 'a grouped list is dragged as if it were its manual order');

  // What is done gathered at the end, folded.
  await page.getByRole('button', { name: /^Grouped/ }).click();
  await page.getByRole('menuitem', { name: 'Done entries at the end' }).click();
  const done = ITEMS.find((item) => item.completion?.is_completed);
  assert.equal(await page.locator(`[data-row="${done.id}"]`).count(), 0, 'a done entry stands among the open ones');
  await page.locator('.done-head').click();
  await page.locator(`[data-row="${done.id}"]`).waitFor({ timeout: 5_000 });

  // The device keeps it.
  await page.reload();
  await page.locator('[data-row]').first().waitFor({ timeout: 15_000 });
  assert.match(await page.getByRole('button', { name: /^Grouped/ }).innerText(), /Column/);
  assert.equal(await page.locator('[data-row] .cell[data-field="people"]').count(), 0, 'the fields were not kept');
  assert.deepEqual(failures, []);
});
