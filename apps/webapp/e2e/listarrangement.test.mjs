// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// One list for everything (concept round 2, §2): the fields as columns under their heads, the
// fields and the grouping in the panel *View* (work-surface §6.2), what is done gathered at the
// end - and the device keeping it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';

import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, ITEMS, signedIn } from './fixture.mjs';

const served = await serve(join(dirname(fileURLToPath(import.meta.url)), '..', 'dist'));
test.after(() => served.close());

test('chromium: 1280 px - fields stand in columns, the panel groups by column, done goes to the end, and it is kept', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.locator('[data-row]').first().waitFor({ timeout: 15_000 });

  // The template's fields, each under its head, at one inline position in every row.
  const heads = (await page.locator('.fields-head .col-head').allInnerTexts()).map((each) => each.trim().toLowerCase());
  assert.deepEqual(heads, ['title', 'due', 'people', 'labels', 'parts']);
  const starts = await page.locator('[data-row] .cell[data-field="labels"]').evaluateAll((cells) => cells.map((cell) => Math.round(cell.getBoundingClientRect().left)));
  assert.equal(new Set(starts).size, 1, `the labels column is not straight: ${starts}`);

  // The panel's *Fields* takes one out.
  const panel = page.locator('[data-view-panel]');
  await page.locator('[data-opener="view"]').click();
  await panel.waitFor({ timeout: 5_000 });
  await panel.locator('[data-view-field="people"]').getByRole('checkbox').uncheck();
  assert.equal(await page.locator('[data-row] .cell[data-field="people"]').count(), 0, 'the column stayed');

  // Grouped by column: one head per column with its count, the column-less last.
  await panel.locator('[data-grouping="column"]').click();
  const groups = await page.locator('.level h2.group-head > span:first-child').allInnerTexts();
  assert.deepEqual(groups, [...BUCKETS.map((bucket) => bucket.name).filter((name) => groups.includes(name)), 'No column']);
  assert.equal(await page.locator('[data-grip]').count(), 0, 'a grouped list is dragged as if it were its manual order');

  // What is done gathered at the end, folded.
  await panel.getByRole('button', { name: 'Done entries at the end' }).click();
  await page.keyboard.press('Escape');
  await panel.waitFor({ state: 'hidden', timeout: 5_000 });
  const done = ITEMS.find((item) => item.completion?.is_completed);
  assert.equal(await page.locator(`[data-row="${done.id}"]`).count(), 0, 'a done entry stands among the open ones');
  await page.locator('.done-head').click();
  await page.locator(`[data-row="${done.id}"]`).waitFor({ timeout: 5_000 });

  // The device keeps it.
  await page.reload();
  await page.locator('[data-row]').first().waitFor({ timeout: 15_000 });
  assert.ok((await page.locator('.level h2.group-head > span:first-child').allInnerTexts()).includes('No column'), 'the grouping was not kept');
  assert.equal(await page.locator('[data-row] .cell[data-field="people"]').count(), 0, 'the fields were not kept');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px - the density switch makes the rows compact, and this device keeps it (E-5, UC-WRK-45 check 4, UC-WRK-41 check 6)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  const row = page.locator('[data-row]').first();
  await row.waitFor({ timeout: 15_000 });
  const comfortable = (await row.boundingBox()).height;
  const density = page.getByRole('radiogroup', { name: 'Density' });
  await density.getByRole('radio', { name: 'Compact' }).click();
  await page.waitForFunction((before) => document.querySelector('[data-row]').getBoundingClientRect().height < before, comfortable, { timeout: 5_000 });
  await page.reload();
  await row.waitFor({ timeout: 15_000 });
  assert.ok((await row.boundingBox()).height < comfortable, 'the device did not keep compact');
  assert.equal(await density.getByRole('radio', { name: 'Compact' }).isChecked(), true);
  assert.deepEqual(failures, []);
});
