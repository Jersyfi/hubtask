// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The container screen on the page head (F9-07, ADR-0061 decision 4): what the toolbar held is
// in the menu with its focus return, the primary action opens the form the list owns, the
// switcher is the head's second row with the list toggle, the filter is a panel or a drawer by
// width, and the board on a phone is one column with a strip. Chromium only, as the shell walk.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, HUB, LABELS, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** The items of a menu by their labels alone, without the reasons beside them. */
const labelsOf = (menu) => menu.getByRole('menuitem').evaluateAll((items) => items.map((item) => item.querySelector('.label')?.textContent.trim().replace(/…$/, '') ?? ''));

/** Backlog decision 5's order: act, set up, trash. */
const MENU = ['Rename', 'Move to another hub', 'Archive', 'Move up', 'Move down', 'Labels', 'Custom fields', 'Saved views', 'Templates', 'Policies', 'People', 'Move to the trash'];

async function openCollection(browser, width) {
  const { page, failures, context } = await signedIn(browser, width, 900);
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { page, failures, close: () => context.close() };
}

test('chromium: 1280 px — the head, the menu in three groups, and every dialog from it with its focus return', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await openCollection(browser, 1280);
  t.after(close);

  // One primary, its split, the filter, the menu - and no toolbar of twelve.
  assert.equal(await page.getByRole('button', { name: 'Add an entry', exact: true }).count(), 2, 'the primary in the head and the button at the list end');
  assert.equal(await page.getByRole('button', { name: 'More ways to add an entry' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: 'Filter and sort' }).count(), 1);
  assert.equal(await page.getByRole('toolbar').count(), 0, 'the toolbar is gone');
  const trigger = page.getByRole('button', { name: `Actions for ${COLLECTION.name}` });
  await trigger.click();
  const menu = page.getByRole('menu', { name: `Actions for ${COLLECTION.name}` });
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual(await labelsOf(menu), MENU);
  // The reasons stay readable: the one collection is first and last in its level.
  assert.ok((await menu.getByRole('menuitem', { name: /Move up/ }).textContent()).includes('already first'));

  // Rename: the inline form takes focus, and cancelling returns it to the menu's trigger.
  await menu.getByRole('menuitem', { name: 'Rename' }).click();
  const field = page.getByRole('textbox', { name: 'Collection name' });
  await field.waitFor({ timeout: 5_000 });
  assert.equal(await page.evaluate(() => document.activeElement?.getAttribute('aria-label') ?? document.activeElement?.id ?? document.activeElement?.tagName), await field.evaluate((el) => el.getAttribute('aria-label') ?? el.id ?? el.tagName));
  await page.getByRole('button', { name: 'Cancel' }).click();
  await page.waitForFunction(() => document.activeElement?.getAttribute('data-opener') === 'container-menu', null, { timeout: 5_000 })
    .catch(async () => assert.fail(`focus did not return to the menu but sits on ${await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80))}`));

  // Every dialog the toolbar opened opens from the menu, and Escape brings focus back.
  for (const [item, title] of [['Labels', 'Labels'], ['Custom fields', 'Custom fields'], ['Saved views', 'Saved views'], ['Templates', 'Templates'], ['Policies', 'Policies'], ['People', 'People'], ['Move to the trash', `Move ${COLLECTION.name} to the trash`]]) {
    await trigger.focus();
    await page.keyboard.press('Enter');
    await page.getByRole('menuitem', { name: item }).click();
    const dialog = page.locator('dialog[open]');
    await dialog.waitFor({ timeout: 5_000 }).catch(() => assert.fail(`${item} opened no dialog`));
    const heading = await dialog.getByRole('heading').first().textContent();
    assert.ok(heading.includes(title), `${item} opened "${heading}"`);
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'hidden', timeout: 5_000 });
    await page.waitForFunction(() => document.activeElement?.getAttribute('data-opener') === 'container-menu', null, { timeout: 5_000 })
      .catch(async () => assert.fail(`${item}: focus did not return to the menu but sits on ${await page.evaluate(() => document.activeElement?.outerHTML.slice(0, 80))}`));
  }

  // The daily ways: the star opens the views, the faces open the people, the split opens the templates.
  await page.getByRole('button', { name: 'Saved views' }).click();
  await page.locator('dialog[open]').getByRole('heading', { name: /Saved views/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'People' }).click();
  await page.locator('dialog[open]').getByRole('heading', { name: /People/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'More ways to add an entry' }).click();
  await page.getByRole('menuitem', { name: 'From a template…' }).click();
  await page.locator('dialog[open]').getByRole('heading', { name: /Templates/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  // The primary opens the list's form and focuses into it; the tour's last step finds it by its opener.
  assert.equal(await page.locator('[data-opener="add-entry"]').first().evaluate((el) => el.textContent.trim()), 'Add an entry');
  await page.getByRole('button', { name: 'Add an entry', exact: true }).first().click();
  await page.getByRole('textbox', { name: 'Title' }).waitFor({ timeout: 5_000 });
  assert.equal(await page.evaluate(() => document.activeElement?.tagName), 'INPUT', 'the form did not take focus');

  // The switcher: three, with "show what is inside" within the list.
  const switcher = page.getByRole('radiogroup', { name: 'How these entries are shown' });
  assert.deepEqual((await switcher.getByRole('radio').allTextContents()).map((each) => each.trim()), ['List', 'Board', 'Timeline']);
  const inside = page.getByRole('checkbox', { name: 'Show what is inside' });
  assert.equal(await inside.count(), 1);
  await inside.click();
  assert.equal(await inside.isChecked(), true);
  await switcher.getByRole('radio', { name: 'Board' }).click();
  assert.equal(await inside.count(), 0, 'the toggle belongs to the list alone');
  await switcher.getByRole('radio', { name: 'List' }).click();
  assert.equal(await page.getByRole('checkbox', { name: 'Show what is inside' }).isChecked(), true, 'the list came back as it was left');

  // The filter: inline, and its count on the button.
  await page.getByRole('button', { name: 'Filter and sort' }).click();
  const panel = page.getByRole('group', { name: 'Which entries to show' });
  await panel.waitFor({ state: 'visible', timeout: 5_000 });
  assert.equal(await page.locator('dialog[open]').count(), 0, 'inline, not a drawer, from expanded');
  await page.getByRole('combobox', { name: 'Order by' }).selectOption('title');
  await page.getByRole('button', { name: 'Filter and sort (1)' }).waitFor({ timeout: 5_000 });

  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the folded head, the filter as a drawer, the board one column with a strip, the bulk bar above the bottom bar', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await openCollection(browser, 375);
  t.after(close);

  // The bar carries the title; the head reads its heading rather than drawing it.
  assert.equal(await page.getByRole('banner', { name: 'Application bar' }).locator('[data-bar="title"]').textContent(), COLLECTION.name);
  assert.equal(await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).evaluate((el) => el.getBoundingClientRect().width <= 1), true, 'the h1 is drawn as well as read');
  assert.equal(await page.getByRole('link', { name: HUB.name }).count(), 1, 'the parent as the way up');

  // Folded: the filter joined the menu, ahead of the twelve.
  await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).click();
  const menu = page.getByRole('menu');
  assert.deepEqual(await labelsOf(menu), ['From a template', 'Filter and sort', ...MENU]);
  await menu.getByRole('menuitem', { name: 'Filter and sort' }).click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  assert.equal(await drawer.getByRole('group', { name: 'Which entries to show' }).count(), 1, 'the filter is a drawer below expanded');
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

  // The board: one column, the strip above with every column and its count, a tap switches.
  await page.getByRole('radio', { name: 'Board' }).click();
  const strip = page.getByRole('radiogroup', { name: 'Which column is shown' });
  await strip.waitFor({ timeout: 10_000 });
  assert.deepEqual((await strip.getByRole('radio').allTextContents()).map((each) => each.trim()), ['To do · 2', 'Doing · 1', 'Done · 1', 'No column · 1']);
  assert.equal(await page.getByRole('region').filter({ has: page.getByRole('heading', { name: /To do|Doing|Done|No column/ }) }).count(), 1, 'more than one column drawn');
  await strip.getByRole('radio', { name: 'Doing · 1' }).click();
  assert.equal(await page.getByRole('heading', { name: 'Doing' }).count(), 1);
  assert.equal(await page.getByRole('heading', { name: 'To do' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: `Actions for ${BUCKETS[1].name}` }).count(), 1, 'the column keeps its own actions');
  assert.equal(await page.getByRole('button', { name: 'Move Book the electrician' }).count(), 1, 'the card keeps its menu');
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, 'the board widened the page');

  // The bulk bar: once something is picked it is fixed above the bottom bar, and the bottom bar stays.
  await page.getByRole('radio', { name: 'List' }).click();
  const pick = page.getByRole('checkbox', { name: /Select .*Order the tiles/ });
  await pick.waitFor({ timeout: 10_000 });
  await pick.click();
  const bar = page.locator('.bar[data-active]');
  await bar.waitFor({ timeout: 5_000 });
  const boxes = await page.evaluate(() => {
    const bulk = document.querySelector('.bar[data-active]').getBoundingClientRect();
    const bottom = document.querySelector('nav[aria-label="Sections"]').getBoundingClientRect();
    return { bulkBottom: Math.round(bulk.bottom), navTop: Math.round(bottom.top), position: getComputedStyle(document.querySelector('.bar[data-active]')).position, navOpacity: getComputedStyle(document.querySelector('nav[aria-label="Sections"]')).opacity };
  });
  assert.equal(boxes.position, 'fixed');
  assert.ok(boxes.bulkBottom <= boxes.navTop, `the bulk bar (${boxes.bulkBottom}) sits over the bottom bar (${boxes.navTop})`);
  assert.equal(boxes.navOpacity, '1', 'a ticked checkbox hid the bottom bar');

  // The timeline in its own frame.
  await page.getByRole('radio', { name: 'Timeline' }).click();
  await page.locator('.window').waitFor({ timeout: 10_000 });
  const framed = await page.evaluate(() => { const el = document.querySelector('.window'); return el ? getComputedStyle(el).borderTopWidth !== '0px' : false; });
  assert.equal(framed, true, 'the timeline has no edge of its own on a phone');
  const labelled = LABELS.length;
  assert.ok(labelled > 0);
  assert.deepEqual(failures, []);
});

test('chromium: 1280 px — the hub: create a collection primary, import beside it, the shorter menu', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('button', { name: 'Create collection' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: /Import/ }).count(), 1);
  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  assert.deepEqual(await labelsOf(page.getByRole('menu')), ['Rename', 'Move to another hub', 'Archive', 'Move up', 'Move down', 'People', 'Move to the trash']);
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Create collection' }).click();
  await page.locator('dialog[open]').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 768 px — the board scrolls inside itself and does not widen the page', async (t) => {
  // Issue 874: a card's hidden checkbox input is absolutely positioned, and an absolute box inside
  // an unpositioned scroller overflows the scroller's ancestor - the page grew by four hundred
  // pixels on a tablet. The board is positioned now; this holds the page to its viewport.
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await openCollection(browser, 768);
  t.after(close);
  await page.getByRole('radio', { name: 'Board' }).click();
  await page.getByRole('region', { name: /To do/ }).waitFor({ timeout: 10_000 });
  const scroll = await page.evaluate(() => ({ page: document.documentElement.scrollWidth - window.innerWidth, board: document.querySelector('.board').scrollWidth - document.querySelector('.board').clientWidth }));
  assert.equal(scroll.page, 0, `the page scrolls sideways by ${scroll.page}px`);
  assert.ok(scroll.board > 0, 'the board has nothing to scroll, so the case is not exercised');
  assert.deepEqual(failures, []);
});
