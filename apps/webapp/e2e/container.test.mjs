// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The collection screen (design-system.md §11.1): the head is the place - its people, its ⚙
// settings, its own menu with focus return, and the one add, which opens capture - and the
// toolbar under it is the entries - the layouts with the list toggle, the filter as a panel or a
// drawer by width, the views and selecting. The settings are a page with a section each, and the
// board on a phone is one column with a strip. Chromium only, as the shell walk.

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

/** The collection's own menu (design-system.md §11.1): what is done to the place itself, trash last. */
const MENU = ['Edit', 'Move to another hub', 'Archive', 'Move up', 'Move down', 'Move to the trash'];

async function openCollection(browser, width) {
  const { page, failures, context, unstubbed } = await signedIn(browser, width, 900);
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { page, failures, unstubbed, close: () => context.close() };
}

test('chromium: 1280 px — the head, the menu in three groups, and every dialog from it with its focus return', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await openCollection(browser, 1280);
  t.after(close);

  // The head: one add, no split; the people and the ⚙; the place's own menu.
  const head = page.locator('header.head');
  assert.equal(await head.getByRole('button', { name: 'Add an entry', exact: true }).count(), 1, 'one add in the head');
  assert.equal(await page.getByRole('button', { name: 'More ways to add an entry' }).count(), 0, 'the add carries no split');
  assert.equal(await head.getByRole('button', { name: 'Filter and sort' }).count(), 0, 'the filter is the toolbar\'s, not the head\'s');
  const settings = head.getByRole('button', { name: `Settings of ${COLLECTION.name}` });
  assert.equal(await settings.count(), 1, 'the ⚙ to the settings stands in the head');

  // The toolbar under it: the layouts at the start; the filter, the views and selecting at the end.
  const toolbar = page.getByRole('toolbar', { name: 'What is shown of the entries' });
  assert.equal(await toolbar.count(), 1);
  assert.equal(await toolbar.getByRole('radiogroup', { name: 'How these entries are shown' }).count(), 1);
  for (const name of ['Filter and sort', 'Views', 'Select entries']) {
    assert.equal(await toolbar.getByRole('button', { name }).count(), 1, `${name} is not in the toolbar`);
  }
  assert.equal(await toolbar.evaluate((el) => getComputedStyle(el).position), 'sticky', 'the toolbar stays under the bar');

  const trigger = page.getByRole('button', { name: `Actions for ${COLLECTION.name}` });
  await trigger.click();
  const menu = page.getByRole('menu', { name: `Actions for ${COLLECTION.name}` });
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual(await labelsOf(menu), MENU);
  // The reasons stay readable: the one collection is first and last in its level.
  assert.ok((await menu.getByRole('menuitem', { name: /Move up/ }).textContent()).includes('already first'));

  await page.keyboard.press('Escape');

  // Every dialog of the place's menu opens from it, and Escape brings focus back.
  for (const [item, title] of [['Edit', 'Edit collection'], ['Move to the trash', `Move ${COLLECTION.name} to the trash`]]) {
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

  // The daily ways: the *Views* menu saves a view, the faces open the people.
  await toolbar.getByRole('button', { name: 'Views' }).click();
  await page.getByRole('menuitem', { name: 'Save this view…' }).click();
  await page.locator('dialog[open]').getByRole('heading', { name: /Save this view/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'People' }).click();
  await page.locator('dialog[open]').getByRole('heading', { name: /People/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  // The add opens capture pointed at this collection; the tour's last step finds it by its opener.
  assert.equal(await page.locator('[data-opener="add-entry"]').first().evaluate((el) => el.textContent.trim()), 'Add an entry');
  await head.getByRole('button', { name: 'Add an entry', exact: true }).click();
  const capture = page.getByRole('dialog', { name: 'Capture' });
  await capture.getByRole('textbox', { name: 'What do you want to remember?' }).waitFor({ timeout: 5_000 });
  assert.match(await capture.locator('[data-chip="place"]').innerText(), new RegExp(COLLECTION.name, 'u'));
  await page.keyboard.press('Escape');
  await capture.waitFor({ state: 'hidden', timeout: 5_000 });

  // The switcher: three, with "show what is inside" within the list.
  const switcher = page.getByRole('radiogroup', { name: 'How these entries are shown' });
  assert.deepEqual((await switcher.getByRole('radio').allTextContents()).map((each) => each.trim()), ['List', 'Board', 'Timeline']);
  const inside = page.getByRole('button', { name: 'Show what is inside', exact: true });
  assert.equal(await inside.count(), 1);
  await inside.click();
  assert.equal(await inside.getAttribute('aria-pressed'), 'true');
  await switcher.getByRole('radio', { name: 'Board' }).click();
  assert.equal(await inside.count(), 0, 'the toggle belongs to the list alone');
  await switcher.getByRole('radio', { name: 'List' }).click();
  assert.equal(await page.getByRole('button', { name: 'Show what is inside', exact: true }).getAttribute('aria-pressed'), 'true', 'the list came back as it was left');

  // The filter: inline, and its count on the button.
  await toolbar.getByRole('button', { name: 'Filter and sort' }).click();
  const panel = page.getByRole('group', { name: 'Which entries to show' });
  await panel.waitFor({ state: 'visible', timeout: 5_000 });
  assert.equal(await page.locator('dialog[open]').count(), 0, 'inline, not a drawer, from expanded');
  await page.getByRole('combobox', { name: 'Order by' }).selectOption('title');
  await page.getByRole('button', { name: 'Filter and sort (1)' }).waitFor({ timeout: 5_000 });

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — the ⚙ opens the settings as a page, a section each at its own address', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close } = await openCollection(browser, 1280);
  t.after(close);

  await page.locator('header.head').getByRole('button', { name: `Settings of ${COLLECTION.name}` }).click();
  await page.waitForURL(new RegExp(`/collections/${COLLECTION.id}/settings/general$`, 'u'), { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Settings', level: 1 }).waitFor({ timeout: 5_000 });
  // No drawer over the collection: the settings are the page.
  assert.equal(await page.locator('dialog[open]').count(), 0);
  const sections = page.getByRole('navigation', { name: 'Parts of the structure' });
  assert.deepEqual((await sections.getByRole('link').allTextContents()).map((each) => each.trim()), ['General', 'Structure', 'Labels', 'Fields', 'Templates', 'Automation', 'People']);
  assert.equal(await sections.getByRole('link', { name: 'General' }).getAttribute('aria-current'), 'page');
  // *General* is the edit dialog's fields, in place.
  await page.getByRole('region', { name: 'General' }).getByRole('textbox', { name: 'Collection name' }).waitFor({ timeout: 5_000 });

  for (const [section, part] of [['Labels', 'Labels in this collection'], ['Fields', 'Custom fields'], ['Templates', 'Templates'], ['Automation', 'Policies'], ['People', 'People']]) {
    await sections.getByRole('link', { name: section }).click();
    await page.getByRole('region', { name: part }).waitFor({ timeout: 5_000 }).catch(() => assert.fail(`${section} draws no ${part}`));
    assert.equal(await sections.getByRole('link', { name: section }).getAttribute('aria-current'), 'page');
  }
  assert.match(page.url(), /\/settings\/people$/u);
  // The section is an address: a reload comes back to it, and the trail leads back to the place.
  await page.reload();
  await page.getByRole('region', { name: 'People' }).waitFor({ timeout: 10_000 });
  await page.getByRole('navigation', { name: 'Where you are' }).getByRole('link', { name: COLLECTION.name }).click();
  await page.waitForURL(new RegExp(`/collections/${COLLECTION.id}$`, 'u'), { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the folded head, the filter as a drawer, the board one column with a strip, the bulk bar above the bottom bar', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await openCollection(browser, 375);
  t.after(close);

  // The bar carries the title; the head reads its heading rather than drawing it.
  assert.equal(await page.getByRole('banner', { name: 'Application bar' }).locator('[data-bar="title"]').textContent(), COLLECTION.name);
  assert.equal(await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).evaluate((el) => el.getBoundingClientRect().width <= 1), true, 'the h1 is drawn as well as read');
  assert.equal(await page.getByRole('link', { name: HUB.name }).count(), 1, 'the parent as the way up');

  // Folded: the menu holds the place's own actions; the bottom bar's ＋ is the add, pinned nowhere else.
  await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).click();
  const menu = page.getByRole('menu');
  assert.deepEqual(await labelsOf(menu), MENU);
  await page.keyboard.press('Escape');
  assert.equal(await page.getByRole('main').getByRole('button', { name: 'Add an entry', exact: true }).count(), 0, 'a second add pinned over the bottom bar');

  // The toolbar: the layouts and one *Filter*, whose sheet holds the views and selecting too.
  const toolbar = page.getByRole('toolbar', { name: 'What is shown of the entries' });
  assert.equal(await toolbar.getByRole('button', { name: 'Views' }).count(), 0);
  await toolbar.getByRole('button', { name: 'Filter and sort' }).click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  assert.equal(await drawer.getByRole('group', { name: 'Which entries to show' }).count(), 1, 'the filter is a drawer below expanded');
  assert.equal(await drawer.getByRole('button', { name: 'Views' }).count(), 1, 'the sheet holds the views');
  assert.equal(await drawer.getByRole('button', { name: 'Select entries' }).count(), 1, 'the sheet holds selecting');
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

  // The board: one column, the strip above with every column and its count, a tap switches.
  await page.getByRole('radio', { name: 'Board' }).click();
  const strip = page.getByRole('radiogroup', { name: 'Which column is shown' });
  await strip.waitFor({ timeout: 10_000 });
  assert.deepEqual((await strip.getByRole('radio').allTextContents()).map((each) => each.trim()), ['No column · 1', 'To do · 2', 'Doing · 1', 'Done · 1']);
  assert.equal(await page.getByRole('region').filter({ has: page.getByRole('heading', { name: /To do|Doing|Done|No column/ }) }).count(), 1, 'more than one column drawn');
  await strip.getByRole('radio', { name: 'Doing · 1' }).click();
  assert.equal(await page.getByRole('heading', { name: 'Doing' }).count(), 1);
  assert.equal(await page.getByRole('heading', { name: 'To do' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: `Actions for ${BUCKETS[1].name}` }).count(), 1, 'the column keeps its own actions');
  assert.equal(await page.getByRole('button', { name: 'Actions for Book the electrician' }).count(), 1, 'the card keeps its menu');
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, 'the board widened the page');

  // Selecting is a mode, and its default is off (ADR-0063 decision 8): no row carries a pick and
  // no bar stands above the list until somebody says they are selecting. The way in on a phone is
  // the toolbar's sheet, because a modifier is what a mouse has and a long press is what a finger has.
  await page.getByRole('radio', { name: 'List' }).click();
  assert.equal(await page.getByRole('checkbox', { name: /Select .*Order the tiles/ }).count(), 0, 'a row carries a pick with nobody selecting');
  assert.equal(await page.locator('main .bar').count(), 0, 'the bulk bar stands above a list nobody is selecting in');
  await toolbar.getByRole('button', { name: 'Filter and sort' }).click();
  await page.locator('dialog[open]').getByRole('button', { name: 'Select entries' }).click();

  // The bulk bar: once something is picked it is fixed above the bottom bar, and the bottom bar stays.
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

  // And the way out: `Escape`, from anywhere on the screen. The picks and the bar go with it.
  await page.keyboard.press('Escape');
  await page.locator('main .bar').waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(await page.getByRole('checkbox', { name: /Select .*Order the tiles/ }).count(), 0, 'the mode outlived Escape');

  // The timeline in its own frame.
  await page.getByRole('radio', { name: 'Timeline' }).click();
  await page.locator('.window').waitFor({ timeout: 10_000 });
  const framed = await page.evaluate(() => { const el = document.querySelector('.window'); return el ? getComputedStyle(el).borderTopWidth !== '0px' : false; });
  assert.equal(framed, true, 'the timeline has no edge of its own on a phone');
  const labelled = LABELS.length;
  assert.ok(labelled > 0);
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — the hub: create a collection primary, import beside it, the shorter menu', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  assert.equal(await page.getByRole('button', { name: 'Create collection' }).count(), 1);
  assert.equal(await page.getByRole('button', { name: /Import/ }).count(), 1);
  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  assert.deepEqual(await labelsOf(page.getByRole('menu')), ['Edit', 'Archive', 'Move up', 'Move down', 'Make private', 'Move to the trash']);
  await page.keyboard.press('Escape');
  // A hub's settings are a page as a collection's are: its general fields, its default, its people.
  await page.getByRole('button', { name: `Settings of ${HUB.name}` }).click();
  await page.waitForURL(new RegExp(`/hubs/${HUB.id}/settings/general$`, 'u'), { timeout: 5_000 });
  assert.deepEqual(
    (await page.getByRole('navigation', { name: 'Parts of the structure' }).getByRole('link').allTextContents()).map((each) => each.trim()),
    ['General', 'Structure', 'People'],
  );
  await page.goBack();
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Create collection' }).click();
  await page.locator('dialog[open]').waitFor({ timeout: 5_000 });
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 768 px — the board scrolls inside itself and does not widen the page', async (t) => {
  // A card's hidden checkbox input is absolutely positioned, and an absolute box inside an
  // unpositioned scroller overflows the scroller's ancestor - the page grows by four hundred
  // pixels on a tablet. The board is positioned; this holds the page to its viewport.
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, failures, close, unstubbed } = await openCollection(browser, 768);
  t.after(close);
  await page.getByRole('radio', { name: 'Board' }).click();
  await page.getByRole('region', { name: /To do/ }).waitFor({ timeout: 10_000 });
  const scroll = await page.evaluate(() => ({ page: document.documentElement.scrollWidth - window.innerWidth, board: document.querySelector('.board').scrollWidth - document.querySelector('.board').clientWidth }));
  assert.equal(scroll.page, 0, `the page scrolls sideways by ${scroll.page}px`);
  assert.ok(scroll.board > 0, 'the board has nothing to scroll, so the case is not exercised');
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});
