// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The frame as one system (design-system.md §11.2, §11.3, §11.6, §13): the navigation's width is
// the person's to drag and the device's to keep, a hub is managed from its own row, and the sync
// mark opens on a band that names the state with the lines on its text edge.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

async function home(page) {
  await page.goto(`${served.origin}/`);
  await page.getByRole('navigation', { name: 'Workspace' }).first().waitFor({ timeout: 10_000 });
}

const columnWidth = (page) => page.evaluate(() => Math.round(document.querySelector('aside.sidenav').getBoundingClientRect().width));

test('chromium: 1280 px — the navigation is as wide as the person drags it, within its bounds, and stays so', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await home(page);

  const edge = page.getByRole('separator', { name: 'Width of the navigation' });
  const start = await columnWidth(page);
  assert.equal(await edge.getAttribute('aria-valuenow'), String(start), 'the edge says the width it stands at');

  await edge.focus();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  assert.equal(await columnWidth(page), start + 32, 'two arrows widen the column by two steps');

  // Kept by the device: a reload draws the column as it was left.
  await page.reload();
  await page.getByRole('navigation', { name: 'Workspace' }).first().waitFor({ timeout: 10_000 });
  assert.equal(await columnWidth(page), start + 32, 'the width did not survive a reload');

  // The bounds are the tokens', and a double-click puts the default back.
  const kept = page.getByRole('separator', { name: 'Width of the navigation' });
  await kept.focus();
  await page.keyboard.press('End');
  assert.equal(await columnWidth(page), 400, 'End goes to the widest');
  await page.keyboard.press('Home');
  assert.equal(await columnWidth(page), 200, 'Home goes to the narrowest');
  await kept.dblclick();
  assert.equal(await columnWidth(page), start, 'a double-click puts the default back');

  // Dragged with the pointer as well.
  const box = await kept.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 60, box.y + box.height / 2, { steps: 4 });
  await page.mouse.up();
  assert.equal(await columnWidth(page), start + 60, 'a drag of 60 px widens it by 60');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — a hub is managed from its own row: the menu holds the hub and opens its dialog', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await home(page);

  const tree = page.getByRole('navigation', { name: 'Workspace' }).first();
  const row = tree.getByRole('treeitem', { name: /^House/ });
  await row.hover();
  await tree.getByRole('button', { name: 'Manage House' }).click();
  const menu = page.getByRole('menu', { name: 'Manage House' });
  await menu.waitFor({ timeout: 5_000 });
  const items = (await menu.getByRole('menuitem').allTextContents()).map((each) => each.trim());
  // The hub's own acts and nothing about entries (§13, G7).
  // The same menu as the hub's page has (placeMenu.ts): its settings first, where its name and
  // look are changed - there is no Edit beside them.
  for (const act of ['Settings…', 'Archive', 'Move to the trash']) assert.ok(items.some((each) => each.startsWith(act)), `the menu lacks ${act}: ${items}`);
  assert.equal(items.some((each) => each.startsWith('Edit')), false, `the menu holds Edit beside Settings: ${items}`);
  assert.equal(items.some((each) => /select|entries/i.test(each)), false, `the hub's menu holds an entry act: ${items}`);
  await page.keyboard.press('Escape');

  // The keyboard reaches the same menu from the row.
  await row.focus();
  await page.keyboard.press('Shift+F10');
  await page.getByRole('menu', { name: 'Manage House' }).waitFor({ timeout: 5_000 });
  // *Settings* is a page of its own, reached from where the place is listed.
  await page.getByRole('menuitem', { name: 'Settings' }).click();
  await page.waitForURL(/\/hubs\/[^/]+\/settings/u, { timeout: 5_000 });
  await page.getByRole('region', { name: 'General' }).getByRole('textbox', { name: 'Hub name' }).waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — the sync mark opens on a band naming the state, the lines on its text edge', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { context, page, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await home(page);

  await page.locator('header .trigger').click();
  const band = page.locator('p.band[data-tone]');
  await band.waitFor({ timeout: 5_000 });
  assert.equal((await band.innerText()).trim(), 'All synchronised', 'the band names the state, not only the connection');
  const geometry = await page.evaluate(() => {
    const state = document.querySelector('p.band[data-tone]');
    const word = state.querySelector('.word').getBoundingClientRect();
    const line = document.querySelector('p.band[data-tone] ~ .body .detail')?.getBoundingClientRect();
    const surface = state.closest('[popover], .surface')?.getBoundingClientRect() ?? state.parentElement.getBoundingClientRect();
    const box = state.getBoundingClientRect();
    const style = getComputedStyle(state);
    return {
      word: Math.round(word.left),
      line: line ? Math.round(line.left) : null,
      padding: Number.parseFloat(style.paddingInlineStart),
      edge: Math.round(box.left - surface.left),
    };
  });
  assert.ok(geometry.padding >= 8, `the band's words touch its edge: ${geometry.padding} px`);
  assert.ok(geometry.edge <= 1, `the band does not run edge to edge: ${geometry.edge} px in`);
  if (geometry.line !== null) assert.ok(Math.abs(geometry.line - geometry.word) <= 1, `the lines start ${geometry.line - geometry.word} px off the band's words`);

  // Offline is tinted like the other states, not a filled block.
  await context.setOffline(true);
  await page.waitForFunction(() => document.querySelector('header .trigger')?.getAttribute('data-connection') === 'offline', null, { timeout: 5_000 });
  const fill = await page.evaluate(() => {
    const trigger = getComputedStyle(document.querySelector('header .trigger'));
    return trigger.fontWeight;
  });
  assert.notEqual(fill, '600', 'offline is drawn as a bold filled block');
  await context.setOffline(false);
  assert.deepEqual(failures, []);
});
