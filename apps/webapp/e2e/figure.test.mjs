// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What counts in a collection (UC-WRK-29 checks 1-3): the figure in the head and in the
// navigation row, per figure, at three widths and in both modes; the figure moves nothing it
// stands beside; the control offers what the structure allows; offline there is no figure.
// Chromium, over the shared fixture's figure hub.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { FIGURES, showFigures, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
showFigures();

/** What each collection's head says, and what its row says - nothing where it says nothing. */
const EXPECTED = [
  [FIGURES.sum, '5 open · about 38.1', '5 open'],
  [FIGURES.due, '3 due', '3 due'],
  [FIGURES.progress, '38% done · 5 of 13', '38%'],
  [FIGURES.rhythm, 'Something done on 2 days this week', '2 days'],
  [FIGURES.newWeek, 'A new week', undefined],
  [FIGURES.open, '7 open', '7 open'],
  [FIGURES.none, undefined, undefined],
  [FIGURES.unavailable, 'Could not be counted', undefined],
];

async function open(browser, width, collection, options = {}) {
  const tab = await signedIn(browser, width, 900, options);
  await tab.page.goto(`${served.origin}/collections/${collection.id}`);
  await tab.page.getByRole('heading', { name: collection.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return tab;
}

const headText = (page) => page.locator('[data-figure] .text').first().textContent({ timeout: 5_000 }).catch(() => undefined);

for (const width of [390, 700, 1280]) {
  test(`chromium: ${width} px — every figure in the head, and no percentage but progress's`, async (t) => {
    const browser = await chromium.launch();
    t.after(() => browser.close());
    for (const [collection, head] of EXPECTED) {
      const { page, context, failures } = await open(browser, width, collection);
      if (head === undefined) {
        assert.equal(await page.locator('[data-figure]').count(), 0, `${collection.name} draws a figure`);
      } else {
        await page.locator('[data-figure]').first().waitFor({ timeout: 5_000 });
        assert.equal((await headText(page))?.trim(), head, collection.name);
      }
      const text = await page.getByRole('heading', { name: collection.name, level: 1 }).evaluate((el) => el.closest('header')?.textContent ?? '');
      assert.equal(text.includes('%'), collection === FIGURES.progress, `${collection.name}: ${text}`);
      // Rhythm: one mark per day with a completion, none for the others.
      if (collection === FIGURES.rhythm) assert.equal(await page.locator('[data-figure] .mark').count(), 2);
      if (collection === FIGURES.newWeek) {
        assert.equal(await page.locator('[data-figure] .mark').count(), 0);
        assert.ok(!/\d/.test((await headText(page)) ?? ''), 'a new week shows a number');
      }
      assert.deepEqual(failures, []);
      await context.close();
    }
  });
}

test('chromium: 1280 px — each row says its figure as part of its name, light and dark', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  for (const colorScheme of ['light', 'dark']) {
    const { page, context } = await open(browser, 1280, FIGURES.none);
    await page.emulateMedia({ colorScheme });
    for (const [collection, , row] of EXPECTED) {
      const node = page.locator(`[data-node="${collection.id}"]`);
      await node.waitFor({ timeout: 5_000 });
      const meta = node.locator('.meta');
      if (row === undefined) assert.equal(await meta.count(), 0, `${collection.name} row draws a figure`);
      else assert.equal((await meta.textContent())?.trim(), row, collection.name);
    }
    await context.close();
  }
});

test('chromium: 1280 px — a figure moves neither the title nor the row it stands in', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const measure = async (collection) => {
    const { page, context } = await open(browser, 1280, collection);
    if (collection !== FIGURES.none) await page.locator('[data-figure]').first().waitFor({ timeout: 5_000 });
    const box = async (locator) => locator.evaluate((el) => {
      const r = el.getBoundingClientRect();
      return { x: Math.round(r.x), y: Math.round(r.y), h: Math.round(r.height) };
    });
    const title = await box(page.getByRole('heading', { name: collection.name, level: 1 }));
    const primary = await box(page.getByRole('button', { name: 'Add an entry', exact: true }).first());
    const plain = await box(page.locator(`[data-node="${FIGURES.none.id}"]`));
    const figured = await box(page.locator(`[data-node="${FIGURES.sum.id}"]`));
    const label = await box(page.locator(`[data-node="${FIGURES.sum.id}"] .label`));
    const plainLabel = await box(page.locator(`[data-node="${FIGURES.none.id}"] .label`));
    await context.close();
    return { title, primary, plain, figured, label, plainLabel };
  };
  const without = await measure(FIGURES.none);
  const withFigure = await measure(FIGURES.sum);
  assert.deepEqual(withFigure.title, without.title, 'the title moved');
  assert.deepEqual(withFigure.primary, without.primary, 'the primary action moved');
  assert.equal(without.figured.h, without.plain.h, 'a row with a figure is taller than one without');
  assert.equal(without.label.x, without.plainLabel.x, 'the label moved');
});

test('chromium: 1280 px — the control offers what the structure allows, and writes the choice', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const choicesOf = async (collection) => {
    const { page, context } = await open(browser, 1280, collection);
    await page.getByRole('button', { name: `Actions for ${collection.name}` }).click();
    await page.getByRole('menuitem', { name: 'What counts' }).click();
    const dialog = page.locator('dialog[open]');
    await dialog.waitFor({ timeout: 5_000 });
    const options = await dialog.getByRole('combobox', { name: 'Show' }).locator('option').allTextContents();
    return { page, context, dialog, options };
  };

  // A list: no progress (check 2).
  const list = await choicesOf(FIGURES.sum);
  assert.deepEqual(list.options, ['Open entries', 'The sum of a number field', 'What is due', "The week's rhythm", 'Flow', 'Nothing']);
  const written = new Promise((resolve) => list.page.on('request', (request) => {
    if (request.method() === 'PATCH' && request.url().endsWith('/structure')) resolve(request.postDataJSON());
  }));
  await list.dialog.getByRole('combobox', { name: 'Show' }).selectOption('DUE');
  await list.dialog.getByRole('button', { name: 'Save' }).click();
  assert.deepEqual(await written, { settings: { counts: { figure: 'DUE' } } });
  await list.context.close();

  // A project: progress offered.
  const project = await choicesOf(FIGURES.progress);
  assert.ok(project.options.includes('Progress'));
  await project.context.close();

  // A progress the hub set, shown as open entries: said so.
  const clipped = await choicesOf(FIGURES.open);
  assert.ok((await clipped.dialog.locator('[data-figure-clipped]').textContent()).includes('set by the hub'));
  assert.equal(await clipped.dialog.getByRole('combobox', { name: 'Show' }).inputValue(), 'OPEN', 'the choice starts empty');
  await clipped.context.close();
});

test('chromium: 1280 px — offline, the replica answers and no figure is drawn', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, context } = await open(browser, 1280, FIGURES.sum);
  await page.locator('[data-figure]').first().waitFor({ timeout: 5_000 });
  // The server goes away: every read of a container fails to reach it.
  await context.route('**/api/v1/containers**', (route) => route.abort('internetdisconnected'));
  await page.reload();
  await page.getByRole('heading', { name: FIGURES.sum.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  assert.equal(await page.locator('[data-figure]').count(), 0, 'a figure was drawn from the replica');
  await context.close();
});
