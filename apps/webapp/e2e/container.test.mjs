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

import { sharedChromium } from './browser.mjs';

import { pressArea } from './press.mjs';
import { serve } from './serve.mjs';
import { BUCKETS, COLLECTION, HUB, ITEMS, LABELS, MANIFEST, PAGE, signedIn, openSettings, viewBar, chooseView, newInHead, openNewCollection, kindChoice } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');

const served = await serve(DIST);
test.after(() => served.close());

/** The items of a menu by their labels alone, without the reasons beside them. */
// A menu's item carries its label in `.label`; shown as a phone's sheet (D-1), the label is the item's first line.
const labelsOf = (menu) => menu.getByRole('menuitem').evaluateAll((items) => items.map((item) => (item.querySelector('.label')?.textContent ?? item.innerText.split('\n')[0] ?? '').trim().replace(/…$/, '')));

/** The collection's own menu (design-system.md §11.1): what is done to the place itself, trash last. */
// The place's one menu, the same from its page and its row in the navigation (placeMenu.ts).
const MENU = ['Settings', 'Move to another hub', 'Import', 'Change position', 'Archive', 'Move to the trash'];

async function openCollection(browser, width) {
  const { page, failures, context, unstubbed } = await signedIn(browser, width, 900);
  await page.goto(`${served.origin}/collections/${COLLECTION.id}`);
  await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).waitFor({ state: 'attached', timeout: 15_000 });
  return { page, failures, unstubbed, close: () => context.close() };
}

test('chromium: 1280 px — the head, the menu in three groups, and every dialog from it with its focus return', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close, unstubbed } = await openCollection(browser, 1280);
  t.after(close);

  // The head: no add - the bar's Capture and the list's own add are the two ways in -; the people
  // and the place's own menu, *Settings…* first in it - no ⚙ beside it.
  const head = page.locator('header.head');
  assert.equal(await head.getByRole('button', { name: 'Add an entry', exact: true }).count(), 0, 'a third add in the head');
  assert.equal(await page.getByRole('button', { name: 'More ways to add an entry' }).count(), 0, 'the add carries no split');
  assert.equal(await head.getByRole('button', { name: /^View/ }).count(), 0, 'the panel is the list\'s, not the head\'s');
  assert.equal(await head.getByRole('button', { name: `Settings of ${COLLECTION.name}` }).count(), 0, 'a ⚙ beside the menu');

  // The view bar under it (the navigation concept §3) holds the views; the one panel *View* and
  // selecting stand in words on the list's own line under it (work-surface concept §6.2, E-21).
  const toolbar = viewBar(page);
  assert.equal(await toolbar.count(), 1);
  assert.equal(await toolbar.getByRole('navigation', { name: COLLECTION.name }).count(), 1);
  const tools = page.getByRole('group', { name: 'The list’s tools' });
  for (const name of ['View', 'Select entries']) {
    assert.equal(await tools.getByRole('button', { name }).count(), 1, `${name} is not in the list's tools`);
    assert.ok((await tools.getByRole('button', { name }).innerText()).trim().length > 0, `${name} is a bare mark`);
  }
  assert.equal(await toolbar.evaluate((el) => getComputedStyle(el).position), 'sticky', 'the toolbar stays under the bar');

  const trigger = page.getByRole('button', { name: `Actions for ${COLLECTION.name}` });
  await trigger.click();
  const menu = page.getByRole('menu', { name: `Actions for ${COLLECTION.name}` });
  await menu.waitFor({ timeout: 5_000 });
  assert.deepEqual(await labelsOf(menu), MENU);

  await page.keyboard.press('Escape');

  // Every dialog of the place's menu opens from it, and Escape brings focus back.
  for (const [item, title] of [['Change position…', `Position of ${COLLECTION.name}`], ['Move to the trash', `Move ${COLLECTION.name} to the trash`]]) {
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

  // The daily ways: ＋ in the view bar saves a view, the faces open the people.
  await viewBar(page).getByRole('button', { name: 'Save this view…' }).click();
  await page.locator('dialog[open]').getByRole('heading', { name: /Save this view/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'People' }).click();
  await page.locator('dialog[open]').getByRole('heading', { name: /People/ }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  // The list's add opens capture pointed at this collection; the tour's last step finds it by its opener.
  // It says what it adds and where: "+ <kind> in “<group>”" (work-surface concept §8, L-6).
  const add = `Entry in “${COLLECTION.name}”`;
  assert.equal(await page.locator('[data-opener="add-entry"]').first().evaluate((el) => el.textContent.trim()), add);
  assert.equal(await head.getByRole('button', { name: add, exact: true }).count(), 0, 'the list\'s add in the head');
  await page.getByRole('main').getByRole('button', { name: add, exact: true }).first().click();
  const capture = page.getByRole('dialog', { name: 'Capture' });
  await capture.getByRole('textbox', { name: 'What do you want to remember?' }).waitFor({ timeout: 5_000 });
  assert.match(await capture.locator('[data-chip="place"]').innerText(), new RegExp(COLLECTION.name, 'u'));
  await page.keyboard.press('Escape');
  await capture.waitFor({ state: 'hidden', timeout: 5_000 });

  // The view bar: three, with "show what is inside" within the list.
  const switcher = viewBar(page).getByRole('navigation', { name: COLLECTION.name });
  assert.deepEqual((await switcher.getByRole('link').allTextContents()).map((each) => each.trim()), ['List', 'Board', 'Timeline']);
  const inside = page.getByRole('button', { name: 'Show what is inside', exact: true });
  assert.equal(await inside.count(), 1);
  await inside.click();
  assert.equal(await inside.getAttribute('aria-pressed'), 'true');
  await chooseView(page, 'Board');
  assert.equal(await inside.count(), 0, 'the toggle belongs to the list alone');
  await chooseView(page, 'List');
  assert.equal(await page.getByRole('button', { name: 'Show what is inside', exact: true }).getAttribute('aria-pressed'), 'true', 'the list came back as it was left');

  // The panel: a drawer at the inline end; the order is one of its sections, and the button says
  // what is changed against the tab's view.
  await tools.getByRole('button', { name: 'View' }).click();
  const panel = page.locator('[data-view-panel]');
  await panel.waitFor({ state: 'visible', timeout: 5_000 });
  await panel.getByLabel('Sort by').selectOption('title');
  await tools.getByRole('button', { name: 'View · 1 changed' }).waitFor({ timeout: 5_000 });
  await page.keyboard.press('Escape');

  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — *Settings…* opens the settings as a page, a section each at its own address', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await openCollection(browser, 1280);
  t.after(close);

  await openSettings(page, COLLECTION.name);
  await page.waitForURL(new RegExp(`/collections/${COLLECTION.id}/settings$`, 'u'), { timeout: 5_000 });
  await page.getByRole('heading', { name: 'Settings', level: 1 }).waitFor({ timeout: 5_000 });
  // No drawer over the collection: the settings are the page.
  assert.equal(await page.locator('dialog[open]').count(), 0);
  const sections = page.getByRole('navigation', { name: 'Parts of the structure' });
  // The sections arrive with the place's modules: read them once the last one stands.
  await sections.getByRole('link', { name: /^People( |$)/u }).waitFor({ timeout: 5_000 });
  assert.deepEqual((await sections.locator('a .label').allTextContents()).map((each) => each.trim()), ['General', 'Templates and presets', 'Types & forms', 'Fields', 'Labels', 'Entry templates', 'Workflow', 'Views', 'Rules', 'Completion and assignment', 'People']);
  assert.equal(await sections.getByRole('link', { name: /^General( |$)/u }).getAttribute('aria-current'), 'page');
  // *General* is the edit dialog's fields, in place.
  await page.locator('[data-place-general]').getByRole('textbox', { name: 'Collection name' }).waitFor({ timeout: 5_000 });

  for (const [section, part] of [['Labels', 'Labels in this collection'], ['Fields', 'Custom fields'], ['Entry templates', 'Templates'], ['Rules', 'Rules'], ['Completion and assignment', 'Completion and assignment'], ['People', 'People']]) {
    await sections.getByRole('link', { name: new RegExp(`^${section}( |$)`, 'u') }).click();
    // A table section is a named region; a section of setting rows is known by its heading.
    await page.getByRole('region', { name: part, exact: true }).or(page.getByRole('heading', { name: part, exact: true, level: 2 })).first().waitFor({ timeout: 5_000 }).catch(() => assert.fail(`${section} draws no ${part}`));
    assert.equal(await sections.getByRole('link', { name: new RegExp(`^${section}( |$)`, 'u') }).getAttribute('aria-current'), 'page');
  }
  assert.match(page.url(), /\/settings\/people$/u);
  // The section is an address: a reload comes back to it, and the trail leads back to the place.
  await page.reload();
  await page.getByRole('region', { name: 'People' }).waitFor({ timeout: 10_000 });
  await page.getByRole('navigation', { name: 'Where you are' }).getByRole('link', { name: COLLECTION.name }).click();
  await page.waitForURL(new RegExp(`/collections/${COLLECTION.id}$`, 'u'), { timeout: 5_000 });
  assert.deepEqual(failures, []);
});

// UC-SVC-05 check 9: a desk's logo is part of its look - set in the collection's *General*, not
// on the portal it is drawn on (scene portal draws no upload there).
test('chromium: 1280 px — a desk sets its portal logo in its settings\' General', async (t) => {
  const browser = await sharedChromium(t);
  const { page, context, failures } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  const desk = { ...COLLECTION, structure: { ...COLLECTION.structure, settings: { ...COLLECTION.structure.settings, entry_types: { TASK: ['incident'] } } } };
  const incident = { key: 'incident', name: 'Incident', level: 'TASK', system: false, position: 0, archived: false, hub_id: null, form: { channels: [{ channel: 'PORTAL' }] } };
  await context.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname.replace(/^.*\/api\/v1/, '');
    // The fixture's copy of the collection is no desk; this one offers a type open to the portal.
    if (path === '/sync:snapshot') {
      const records = [HUB, desk].map((c) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c }));
      return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
    }
    if (route.request().method() !== 'GET') return route.fallback();
    if (path === '/containers' && new URL(route.request().url()).searchParams.get('parent_id') === HUB.id) return route.fulfill({ json: { ...PAGE, data: [desk] } });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: ['SERVICE_DESK', 'WORKFLOWS'] } });
    if (path === `/containers/${COLLECTION.id}/modules` || path === `/containers/${HUB.id}/modules`) return route.fulfill({ json: { modules_on: ['SERVICE_DESK', 'WORKFLOWS'] } });
    if (path === `/containers/${COLLECTION.id}`) return route.fulfill({ json: desk });
    if (path === '/entry-types') return route.fulfill({ json: { data: [incident] } });
    if (path === `/containers/${COLLECTION.id}/desk-logo`) return route.fulfill({ json: { collection_id: COLLECTION.id, logo_media_id: null } });
    return route.fallback();
  });
  await page.goto(`${served.origin}/collections/${COLLECTION.id}/settings/general`);
  await page.locator('[data-place-general]').getByRole('textbox', { name: 'Collection name' }).waitFor({ timeout: 15_000 });
  await page.getByText('Logo in the portal').first().waitFor({ timeout: 10_000 });
  assert.ok(await page.getByText('Choose a logo').count() > 0, 'the logo is offered in General');
  assert.deepEqual(failures, []);
});

test('chromium: 375 px — the folded head, the panel as a sheet of pages, the board one column with a strip, the bulk bar above the bottom bar', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close, unstubbed } = await openCollection(browser, 375);
  t.after(close);

  // The bar carries the title; the head reads its heading rather than drawing it.
  assert.equal(await page.getByRole('banner', { name: 'Application bar' }).locator('[data-bar="title"]').textContent(), COLLECTION.name);
  assert.equal(await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).evaluate((el) => el.getBoundingClientRect().width <= 1), true, 'the h1 is drawn as well as read');
  assert.equal(await page.getByRole('link', { name: HUB.name }).count(), 1, 'the parent as the way up');

  // Folded: the menu holds the place's own actions; the bottom bar's ＋ is the add, and the list's
  // own add at its end, never one pinned over the bottom bar.
  await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).click();
  const menu = page.getByRole('menu');
  // A phone's ⋯ leads with selecting (scene board 4).
  assert.deepEqual(await labelsOf(menu), ['Select entries', ...MENU]);
  await page.keyboard.press('Escape');
  assert.equal(await page.locator('header.head').getByRole('button', { name: 'Add an entry', exact: true }).count(), 0, 'an add in the head');

  // *Filter* on the view chip's line, whose sheet leads on to *View*, whose sheet holds its
  // sections as pages (scenes board 4, viewcfg 4).
  const toolbar = viewBar(page);
  assert.equal(await toolbar.getByRole('button', { name: 'Views' }).count(), 0);
  await page.locator('[data-opener="filter"]').click();
  await page.locator('[data-opener="view"]').click();
  const drawer = page.locator('dialog[open]');
  await drawer.waitFor({ timeout: 5_000 });
  assert.deepEqual(await drawer.locator('[data-view-page]').evaluateAll((pages) => pages.map((each) => each.dataset.viewPage)), ['layout', 'query', 'order', 'fields', 'save'], 'the sheet\'s pages');
  await page.keyboard.press('Escape');
  await drawer.waitFor({ state: 'hidden', timeout: 5_000 });

  // The board: one column, the strip above with every column and its count, a tap switches.
  await chooseView(page, 'Board');
  const strip = page.getByRole('radiogroup', { name: 'Which column is shown' });
  await strip.waitFor({ timeout: 10_000 });
  assert.deepEqual((await strip.getByRole('radio').allTextContents()).map((each) => each.trim()), ['To do · 2', 'Doing · 1', 'Done · 1', 'Without column · 1']);
  assert.equal(await page.getByRole('region').filter({ has: page.getByRole('heading', { name: /To do|Doing|Done|Without column/ }) }).count(), 1, 'more than one column drawn');
  for (const target of await strip.getByRole('radio').all()) assert.ok(((await target.boundingBox())?.height ?? 0) >= 44, 'a strip target is under 44 px (§7)');
  assert.equal(new Set(await strip.getByRole('radio').evaluateAll((all) => all.map((each) => Math.round(each.getBoundingClientRect().top)))).size, 1, 'the strip wraps onto a second row');
  await strip.getByRole('radio', { name: 'Doing · 1' }).click();
  assert.equal(await page.getByRole('heading', { name: 'Doing' }).count(), 1);
  assert.equal(await page.getByRole('heading', { name: 'To do' }).count(), 0);
  assert.equal(await page.getByRole('button', { name: `Actions for ${BUCKETS[1].name}` }).count(), 1, 'the column keeps its own actions');
  assert.equal(await page.getByRole('button', { name: 'Actions for Book the electrician' }).count(), 1, 'the card keeps its menu');
  const scroll = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: window.innerWidth }));
  assert.equal(scroll.width, scroll.viewport, 'the board widened the page');

  // Selecting is a mode, and its default is off (ADR-0063 decision 8): no row carries a pick and
  // no bar stands above the list until somebody says they are selecting. The way in on a phone is
  // the ⋯, because a modifier is what a mouse has and a long press is what a finger has.
  await chooseView(page, 'List');
  assert.equal(await page.getByRole('checkbox', { name: /Select .*Order the tiles/ }).count(), 0, 'a row carries a pick with nobody selecting');
  assert.equal(await page.locator('main .bar').count(), 0, 'the bulk bar stands above a list nobody is selecting in');
  await page.getByRole('button', { name: `Actions for ${COLLECTION.name}` }).click();
  await page.getByRole('menuitem', { name: 'Select entries' }).click();

  // The bulk bar: once something is picked it is fixed above the bottom bar, and the bottom bar stays.
  const pick = page.getByRole('checkbox', { name: /Select .*Order the tiles/ });
  await pick.waitFor({ timeout: 10_000 });
  await pick.click();
  const bar = page.locator('.bar[data-active]');
  await bar.waitFor({ timeout: 5_000 });
  // UC-WRK-08 check 1: while the mode is on, rows carry a checkbox and the bar the count and the
  // verbs; the ways in by a long press and by Ctrl or Cmd-click are walked in entrymenu.test.mjs.
  await bar.getByText('1 selected').waitFor({ timeout: 5_000 });
  assert.equal(await bar.getByRole('combobox', { name: 'Do this' }).count(), 1, 'the bar offers no verbs');
  const boxes = await page.evaluate(() => {
    const bulk = document.querySelector('.bar[data-active]').getBoundingClientRect();
    const bottom = document.querySelector('nav[aria-label="Sections"]').getBoundingClientRect();
    return { bulkBottom: Math.round(bulk.bottom), navTop: Math.round(bottom.top), position: getComputedStyle(document.querySelector('.bar[data-active]')).position, navOpacity: getComputedStyle(document.querySelector('nav[aria-label="Sections"]')).opacity };
  });
  assert.equal(boxes.position, 'fixed');
  assert.ok(boxes.bulkBottom <= boxes.navTop, `the bulk bar (${boxes.bulkBottom}) sits over the bottom bar (${boxes.navTop})`);
  assert.equal(boxes.navOpacity, '1', 'a ticked checkbox hid the bottom bar');
  // A thumb's target (§7): every control of the bar is at least 44 px tall on a phone.
  const short = await page.evaluate(() =>
    [...document.querySelectorAll('.bar[data-active] :is(button, select, [role="combobox"], .checkbox .row)')]
      .map((each) => ({ name: each.getAttribute('aria-label') ?? each.textContent.trim().slice(0, 30), height: Math.round(each.getBoundingClientRect().height) }))
      .filter((each) => each.height > 0 && each.height < 44),
  );
  assert.deepEqual(short, [], 'a control of the bulk bar is under 44 px on a phone');

  // And the way out: `Escape`, from anywhere on the screen. The picks and the bar go with it.
  await page.keyboard.press('Escape');
  await page.locator('main .bar').waitFor({ state: 'detached', timeout: 5_000 });
  assert.equal(await page.getByRole('checkbox', { name: /Select .*Order the tiles/ }).count(), 0, 'the mode outlived Escape');

  // The timeline in its own frame.
  await chooseView(page, 'Timeline');
  await page.locator('.timeline .scroller').waitFor({ timeout: 10_000 });
  const framed = await page.evaluate(() => { const el = document.querySelector('.timeline .scroller'); return el ? getComputedStyle(el).borderTopWidth !== '0px' : false; });
  assert.equal(framed, true, 'the timeline has no edge of its own on a phone');
  const labelled = LABELS.length;
  assert.ok(labelled > 0);
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

test('chromium: 1280 px — the hub: create a collection primary, import beside it, the shorter menu', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, context, unstubbed } = await signedIn(browser, 1280, 900);
  t.after(() => context.close());
  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  // The hub's tile before its name (scene hub).
  assert.equal(await page.locator('main .title-mark [data-hub-mark]').count(), 1, 'the hub head draws no tile');
  // Creating a collection is the head's *New*, which opens *Create*; importing is the
  // place's menu, as on a collection.
  assert.equal(await newInHead(page).count(), 1);
  // *New* opens *Create* preset to the hub - one way to create (§6.12, C-1, C-2) - and no menu.
  await newInHead(page).click();
  const made = page.getByRole('group', { name: 'Create' });
  await made.locator('[data-kind="collection"]').first().waitFor({ timeout: 5_000 });
  assert.equal(await page.getByRole('menu').count(), 0, 'a menu of its own beside Create');
  assert.equal(await made.getByRole('combobox', { name: 'Where it is made' }).inputValue(), `h:${HUB.id}`);
  await page.keyboard.press('Escape');
  await made.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(await page.getByRole('main').getByRole('button', { name: /Import/ }).count(), 0, 'import beside the head as well as in its menu');
  await page.getByRole('button', { name: `Actions for ${HUB.name}` }).click();
  // No *Change position* in a hub's own head: the hubs are ordered where they are all listed.
  assert.deepEqual(await labelsOf(page.getByRole('menu')), ['Settings', 'Import', 'Archive', 'Make private', 'Move to the trash']);
  await page.keyboard.press('Escape');
  // A hub's settings are a page as a collection's are: one section list (user-experience.md §5.7).
  await openSettings(page, HUB.name);
  await page.waitForURL(new RegExp(`/hubs/${HUB.id}/settings$`, 'u'), { timeout: 5_000 });
  // The page is drawn once its structure is read, a moment after the address lands.
  await page.getByRole('navigation', { name: 'Parts of the structure' }).getByRole('link').first().waitFor({ timeout: 5_000 });
  assert.deepEqual(
    (await page.getByRole('navigation', { name: 'Parts of the structure' }).locator('a .label').allTextContents()).map((each) => each.trim()),
    ['General', 'Products', 'Templates and presets', 'Types & forms', 'Workflow', 'Views', 'Rules', 'Code', 'Chat', 'People'],
  );
  await page.goBack();
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await page.keyboard.press('Escape');
  await openNewCollection(page);
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
  const browser = await sharedChromium(t);
  const { page, failures, close, unstubbed } = await openCollection(browser, 768);
  t.after(close);
  await chooseView(page, 'Board');
  await page.getByRole('region', { name: /To do/ }).waitFor({ timeout: 10_000 });
  const scroll = await page.evaluate(() => ({ page: document.documentElement.scrollWidth - window.innerWidth, board: document.querySelector('.board').scrollWidth - document.querySelector('.board').clientWidth }));
  assert.equal(scroll.page, 0, `the page scrolls sideways by ${scroll.page}px`);
  assert.ok(scroll.board > 0, 'the board has nothing to scroll, so the case is not exercised');
  assert.deepEqual(failures, []);
  // Nothing was answered by a guess: a shape this fixture never prepared is a shape the walk
  // cannot claim to have exercised (`fixture.mjs`).
  assert.deepEqual(unstubbed(), []);
});

// UC-WRK-17 check 12: the filter as a line - typed, it becomes the conditions and the order the
// query sends; built from the controls, the line says them back.
test('chromium: 1280 px — the line in the panel is read into the query and the order, and spelled back', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close, unstubbed } = await openCollection(browser, 1280);
  t.after(close);
  const sent = [];
  page.on('request', (request) => {
    if (new URL(request.url()).pathname.endsWith('/items:query')) sent.push(request.postDataJSON());
  });

  await page.getByRole('group', { name: 'The list’s tools' }).getByRole('button', { name: 'View' }).click();
  const panel = page.locator('[data-view-panel]');
  await panel.waitFor({ state: 'visible', timeout: 5_000 });
  const line = panel.getByRole('combobox', { name: 'What is seen' });
  await line.click();
  await page.keyboard.type('title: milk sort: -title ');
  await line.press('Enter');
  for (let waited = 0; !sent.some((body) => JSON.stringify(body?.filter ?? {}).includes('milk')) && waited < 50; waited++) await page.waitForTimeout(100);

  const last = sent.at(-1);
  assert.match(JSON.stringify(last?.filter), /"title".*"milk"|"milk".*"title"/u, `the query sent ${JSON.stringify(last)}`);
  assert.deepEqual(last?.sort?.[0], { field: 'title', dir: 'DESC' });
  // The line is drawn as chips the reader can change, and spelled back as the text it sends.
  assert.equal(await panel.getByRole('button', { name: /^Change title/ }).count(), 1, 'the line became no chip');
  assert.match(await panel.locator('[data-view-line]').innerText(), /title: milk/u);
  // Over the list, each piece is a chip of its own that takes itself away.
  assert.ok((await page.locator('.query-chips li').count()) >= 1, 'the pieces are not over the list');

  // A line that cannot be read says so at its chip and changes nothing.
  const asked = sent.length;
  await page.keyboard.type('(is: open ');
  await page.waitForTimeout(600);
  assert.equal(sent.slice(asked).some((body) => !JSON.stringify(body?.filter ?? {}).includes('milk')), false, 'an unreadable line was sent');

  assert.deepEqual(failures, []);
  assert.deepEqual(unstubbed(), []);
});

// A thumb's target (user-experience.md §7): on a phone, while entries are picked for the bulk bar,
// a row's label chips are marks, not targets of their own - a press on one lands on the row's link
// or on the bar drawn over it, each taking layout.target (44 px) each way.
test('chromium: 375 px — while picking, a press on a row\'s label chip lands on a 44 px target', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, close } = await openCollection(browser, 375);
  t.after(close);
  await page.locator('.task-row').first().waitFor({ timeout: 15_000 });
  await page.locator('[data-bar="menu"] button').click();
  await page.getByRole('menuitem', { name: 'Select entries' }).click();
  await page.getByRole('checkbox', { name: `Select ${ITEMS[0].title}` }).check();
  const chips = page.locator('.task-row .chip');
  assert.ok((await chips.count()) > 0, 'no label chip on a row');
  for (const chip of await chips.all()) {
    await chip.scrollIntoViewIfNeeded();
    const marker = await chip.evaluate((element) => {
      const box = element.getBoundingClientRect();
      const target = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2)?.closest('a, button, label, [role="toolbar"], .bar');
      if (!target) return null;
      target.setAttribute('data-pressed-probe', '');
      return target.tagName;
    });
    assert.notEqual(marker, null, 'a press on a label chip lands on nothing');
    const target = page.locator('[data-pressed-probe]');
    const area = await pressArea(target);
    await target.evaluate((element) => element.removeAttribute('data-pressed-probe'));
    assert.ok(area.width >= 44 && area.height >= 44, `a press on a label chip lands on ${marker} of ${JSON.stringify(area)}`);
  }
  assert.deepEqual(failures, []);
});

// UC-WRK-01 check 12: *Create collection* offers its kinds as one choice, *Like its hub* chosen
// when the dialog opens, the chosen kind marked by its edge and a tick - the hub page's tile.

/** Opens the hub, answers the collection's creation with `made`, and opens the dialog. */
async function openCreateCollection(browser, width, height, made) {
  const { page, failures, context } = await signedIn(browser, width, height, width < 600 ? { hasTouch: true } : {});
  const sent = [];
  await page.route('**/api/v1/containers', (route, request) => {
    if (request.method() !== 'POST') return route.fallback();
    sent.push(request.postDataJSON());
    return route.fulfill({ status: 201, json: made });
  });
  await page.goto(`${served.origin}/hubs/${HUB.id}`);
  await page.getByRole('heading', { name: HUB.name, level: 1 }).waitFor({ timeout: 15_000 });
  await openNewCollection(page);
  const dialog = page.locator('dialog[open]');
  await kindChoice(dialog, 'Like its hub').waitFor({ timeout: 10_000 });
  // The kinds are data: wait for the presets the installation answers.
  await kindChoice(dialog, 'Scrum').waitFor({ timeout: 10_000 });
  return { page, failures, dialog, sent, close: () => context.close() };
}

/** What the focused element is: its type and the start of its label's words. */
const focused = (page) => page.evaluate(() => {
  const element = document.activeElement;
  const label = element?.closest('label')?.textContent?.trim() ?? '';
  return `${element?.getAttribute('type') ?? element?.tagName}:${label.slice(0, 24)}`;
});

test('chromium: 1280 px — Create collection: Like its hub chosen at opening, a kind chosen by the keyboard, the chosen tile ticked, Desk refused with its reason (UC-WRK-01 check 12)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, dialog, sent, close } = await openCreateCollection(browser, 1280, 800, COLLECTION);
  t.after(close);

  // One radio group, the hub's own structure first and chosen.
  const kinds = dialog.getByRole('radiogroup', { name: 'How much structure' });
  const names = await kinds.getByRole('radio').evaluateAll((radios) => radios.map((radio) => radio.closest('label')?.querySelector('.title')?.textContent?.trim()));
  assert.deepEqual(names, ['Like its hub', 'List', 'Tasks', 'Project', 'Flow', 'Scrum', 'Desk']);
  assert.equal(await kindChoice(dialog, 'Like its hub').isChecked(), true, 'Like its hub is not chosen at opening');
  assert.equal(await kinds.getByRole('radio', { checked: true }).count(), 1);

  // Without a portal type in the hub, Desk cannot be chosen and its reason is its description.
  const desk = kindChoice(dialog, 'Desk');
  assert.equal(await desk.isDisabled(), true, 'a desk without a portal type can be chosen');
  const reason = await desk.evaluate((radio) => (radio.getAttribute('aria-describedby') ?? '').split(' ').map((id) => document.getElementById(id)?.textContent ?? '').join(' '));
  assert.match(reason, /Needs a portal type in the hub/u);

  // The focus starts in the name; Tab enters the kinds at the chosen one; the arrows walk and
  // choose, and pass over the refused Desk.
  assert.match(await focused(page), /^text:Name:/u, 'the focus does not start in the name');
  await page.keyboard.press('Tab');
  assert.match(await focused(page), /^radio:Like its hub/u, 'Tab does not enter the kinds at the chosen one');
  await page.keyboard.press('ArrowRight');
  assert.equal(await kindChoice(dialog, 'List').isChecked(), true, 'an arrow does not choose');
  assert.equal(await kindChoice(dialog, 'Like its hub').isChecked(), false, 'two kinds chosen at once');
  await page.keyboard.press('ArrowLeft');
  await page.keyboard.press('ArrowLeft');
  assert.match(await focused(page), /^radio:Scrum/u, 'the arrows do not pass over the refused Desk');
  for (let press = 0; press < 3; press += 1) await page.keyboard.press('ArrowLeft');
  assert.equal(await kindChoice(dialog, 'Tasks').isChecked(), true);

  // The chosen tile: its edge doubled and the tick filled in the edge's colour; another one not.
  const tile = (name) => kindChoice(dialog, name).locator('xpath=ancestor::label[1]');
  const handle = await tile('Tasks').elementHandle();
  await page.waitForFunction((element) => getComputedStyle(element.querySelector('.tick')).backgroundColor === getComputedStyle(element).borderTopColor, handle, { timeout: 5_000 });
  const look = (name) => tile(name).evaluate((element) => {
    const style = getComputedStyle(element);
    return { edge: style.borderTopColor, shadow: style.boxShadow, tick: getComputedStyle(element.querySelector('.tick')).backgroundColor };
  });
  const chosen = await look('Tasks');
  const other = await look('List');
  assert.notEqual(chosen.shadow, 'none', 'the chosen edge is not doubled');
  assert.equal(chosen.tick, chosen.edge, 'the tick is not filled in the edge\'s colour');
  assert.equal(other.shadow, 'none', 'a kind not chosen is drawn as chosen');
  assert.notEqual(other.tick, chosen.tick, 'a kind not chosen is ticked');

  // The look stands open; choosing a colour keeps the focus in the dialog.
  await dialog.getByRole('radio', { name: 'Teal' }).check();
  assert.notEqual(await page.evaluate(() => document.activeElement?.tagName), 'BODY', 'the focus fell to the page');

  // Enter in the name creates: one request with the kind chosen, then the collection's page.
  await dialog.getByRole('textbox', { name: 'Name' }).fill(COLLECTION.name);
  await page.keyboard.press('Enter');
  await page.waitForURL(new RegExp(`/collections/${COLLECTION.id}$`, 'u'), { timeout: 10_000 });
  await page.getByRole('heading', { name: COLLECTION.name, level: 1 }).waitFor({ timeout: 15_000 });
  assert.equal(sent.length, 1);
  assert.deepEqual(sent[0], { type: 'COLLECTION', parent_id: HUB.id, name: COLLECTION.name, description: null, icon: null, color_token: 'teal', preset: 'TASKS' });
  assert.deepEqual(failures, []);
});

test('chromium: 390 px — Create collection is the whole screen, one tile per row, Like its hub chosen; Create without a name marks the field (UC-WRK-01 check 12)', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, dialog, sent, close } = await openCreateCollection(browser, 390, 844, COLLECTION);
  t.after(close);

  // *Dialog M* below `medium`: the whole screen, the primary in its bar - once it has risen.
  await page.waitForFunction(() => {
    const box = document.querySelector('dialog[open]')?.getBoundingClientRect();
    return box?.x === 0 && box.y === 0 && box.width === innerWidth && box.height === innerHeight;
  }, null, { timeout: 5_000 }).catch(() => assert.fail('the dialog is not the whole screen'));
  const tiles = await dialog.getByRole('radiogroup', { name: 'How much structure' }).locator('label').evaluateAll((labels) => labels.map((label) => Math.round(label.getBoundingClientRect().x)));
  assert.equal(new Set(tiles).size, 1, `the tiles are not one per row: ${tiles}`);
  assert.equal(await kindChoice(dialog, 'Like its hub').isChecked(), true, 'Like its hub is not chosen at opening');
  assert.equal(await kindChoice(dialog, 'Desk').isDisabled(), true);
  assert.equal(await page.evaluate(() => document.scrollingElement.scrollWidth), 390, 'the page scrolls sideways');

  // A kind chosen by a press; Create without a name marks the name and puts the focus there.
  await kindChoice(dialog, 'Flow').check();
  await dialog.getByRole('button', { name: 'Create', exact: true }).first().click();
  await page.waitForFunction(() => document.activeElement?.getAttribute('aria-invalid') === 'true', null, { timeout: 5_000 });
  assert.match(await dialog.innerText(), /Give it a name first\./u);
  assert.equal(sent.length, 0, 'a collection was sent without a name');
  await page.keyboard.type('Garden');
  await dialog.getByRole('button', { name: 'Create', exact: true }).first().click();
  await page.waitForURL(new RegExp(`/collections/${COLLECTION.id}$`, 'u'), { timeout: 10_000 });
  assert.equal(sent.length, 1);
  assert.equal(sent[0].preset, 'FLOW');
  assert.equal(sent[0].name, 'Garden');
  assert.deepEqual(failures, []);
});
