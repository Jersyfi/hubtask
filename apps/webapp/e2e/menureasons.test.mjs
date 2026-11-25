// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: a locked item of the entry's ⋯ menu says why as the design system's reason
// bubble (design-system.md §4, case 2), at 1280 × 800 in the anchored menu and at 390 × 844 in its
// sheet. Each locked item is one line, as tall as its own words, with an ⓘ mark at its end and
// none of the reason's words in it; it stays focusable and `aria-disabled`, and its reason is its
// `aria-describedby` whether or not the bubble shows. The bubble opens on hover, on focus, on a
// click and on a tap, inside the window; `Escape` closes the bubble first and the menu only on a
// second press, focus back on the ⋯. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';
import { serve } from './serve.mjs';
import { ITEMS, signedIn } from './fixture.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The fixture's task with a due date: its menu has items locked with a reason. */
const DATED = ITEMS.find((each) => each.due_at);

/** The entry's page at a size, its ⋯ menu open once the level is read; the menu and its opener. */
async function openMenu(browser, width, height) {
  const isPhone = width < 600;
  const { page, failures, context } = await signedIn(browser, width, height, isPhone ? { hasTouch: true, isMobile: true } : {});
  await page.goto(`${served.origin}/items/${DATED.id}`);
  await page.getByRole('heading', { name: DATED.title }).first().waitFor({ timeout: 15_000 });
  const opener = page.locator('[data-opener="entry-menu"]').filter({ visible: true }).first();
  await opener.waitFor({ timeout: 15_000 });
  if (isPhone) await opener.tap();
  else await opener.click();
  const menu = page.getByRole('menu', { name: `Actions for ${DATED.title}` });
  await menu.waitFor({ timeout: 5_000 });
  await page.waitForFunction(() => [...document.querySelectorAll('[role="menuitem"]')].some((each) => each.textContent?.includes('Move up') && each.getAttribute('aria-disabled') !== 'true'), null, { timeout: 5_000 }).catch(() => undefined);
  return { page, failures, context, menu, opener };
}

/** Each item: its name, whether it is locked, its height, its mark, and what describes it. */
function itemsOf(menu) {
  return menu.evaluate((list) => [...list.querySelectorAll('[role="menuitem"]')].map((item) => {
    const ids = (item.getAttribute('aria-describedby') ?? '').split(/\s+/).filter(Boolean);
    const described = ids.map((id) => document.getElementById(id)).filter(Boolean);
    const mark = item.querySelector('[data-reason-mark]');
    const style = getComputedStyle(item);
    const px = (value) => Number.parseFloat(value) || 0;
    const label = item.querySelector('.label') ?? item.querySelector('span:not([data-reason-mark]):not(:has(*))');
    // As tall as its own words and its padding, or its least height: no second line of a reason.
    const room = px(style.paddingTop) + px(style.paddingBottom) + px(style.borderTopWidth) + px(style.borderBottomWidth);
    const needs = Math.max(px(style.minBlockSize), (label?.getBoundingClientRect().height ?? 0) + room);
    return {
      name: item.innerText.trim(),
      isLocked: item.getAttribute('aria-disabled') === 'true',
      isFocusable: item.tabIndex >= -1 && !item.hasAttribute('disabled'),
      height: item.getBoundingClientRect().height,
      needs,
      hasMark: mark !== null && mark.getBoundingClientRect().width > 0 && getComputedStyle(mark).visibility === 'visible',
      description: described.map((each) => each.textContent.trim()).join(' '),
      bubbleIds: described.filter((each) => each.getAttribute('role') === 'tooltip').map((each) => each.id),
    };
  }));
}

/** The bubble with this id: whether it shows, and whether all of it is inside the window. */
function bubble(page, id) {
  return page.evaluate((bubbleId) => {
    const node = document.getElementById(bubbleId);
    if (!node || node.hidden) return { isShowing: false, isInside: false };
    const box = node.getBoundingClientRect();
    return {
      isShowing: box.width > 0 && box.height > 0,
      isInside: box.left >= 0 && box.top >= 0 && box.right <= innerWidth && box.bottom <= innerHeight,
      text: node.textContent.trim(),
    };
  }, id);
}

const showing = (page, id) => page.waitForFunction((bubbleId) => {
  const node = document.getElementById(bubbleId);
  return node !== null && !node.hidden && node.getBoundingClientRect().width > 0;
}, id, { timeout: 5_000 });

const hidden = (page, id) => page.waitForFunction((bubbleId) => {
  const node = document.getElementById(bubbleId);
  return node === null || node.hidden;
}, id, { timeout: 5_000 });

/** Asserts what every locked item is: one line, an ⓘ at its end, the reason its description and not its text. */
function assertLockedItems(items, where) {
  const locked = items.filter((item) => item.isLocked);
  assert.ok(locked.length >= 1, `${where}: the fixture's task has no locked item in its menu`);
  for (const item of locked) {
    assert.ok(item.isFocusable, `${where}: “${item.name}” cannot be reached`);
    assert.ok(item.description.length > 0, `${where}: “${item.name}” has no reason for a screen reader`);
    assert.equal(item.bubbleIds.length, 1, `${where}: “${item.name}” is not described by one reason bubble`);
    assert.ok(!item.name.includes(item.description), `${where}: “${item.name}” draws its reason as a line`);
    assert.ok(item.hasMark, `${where}: “${item.name}” has no ⓘ at its end`);
    assert.ok(item.height <= item.needs + 1, `${where}: “${item.name}” is ${item.height} px tall, its own words need ${item.needs} px`);
  }
  return locked;
}

test('chromium: 1280 × 800 — a locked item of the ⋯ menu is one line with an ⓘ, its reason a bubble on hover, focus and click, closed by Escape before the menu', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, menu, opener } = await openMenu(browser, 1280, 800);
  const items = await itemsOf(menu);
  const [first] = assertLockedItems(items, '1280');
  const id = first.bubbleIds[0];
  const item = menu.getByRole('menuitem', { name: first.name, exact: true });
  assert.equal((await bubble(page, id)).isShowing, false, 'a reason shows before anybody reached for its item');

  // Hover: the pointer over the item shows it; moved onto another item, it goes.
  await item.scrollIntoViewIfNeeded();
  await item.hover();
  await showing(page, id);
  const shown = await bubble(page, id);
  assert.equal(shown.text, first.description, 'the bubble says something other than the item\'s description');
  assert.ok(shown.isInside, 'the bubble stands outside the window');
  const other = menu.getByRole('menuitem').filter({ hasNot: page.locator('[data-reason-mark]:not([data-off])') }).first();
  await other.hover();
  await hidden(page, id);

  // Focus: the arrows reach the locked item and its bubble shows; Escape closes the bubble, not the menu.
  await item.focus();
  await showing(page, id);
  await page.keyboard.press('Escape');
  await hidden(page, id);
  assert.equal(await menu.isVisible(), true, 'the first Escape closed the menu, not the bubble');

  // Click: with the pointer resting on the item and the bubble closed, a click opens it again and
  // runs nothing - the menu stays open.
  const box = await item.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await showing(page, id);
  await page.keyboard.press('Escape');
  await hidden(page, id);
  await page.mouse.down();
  await page.mouse.up();
  await showing(page, id);
  assert.equal(await menu.isVisible(), true, 'a click on a locked item closed the menu');

  // Enter on the focused locked item shows the reason too, and runs nothing.
  await page.keyboard.press('Escape');
  await hidden(page, id);
  await page.keyboard.press('Enter');
  await showing(page, id);

  // Escape twice: the bubble, then the menu, focus back on the ⋯.
  await page.keyboard.press('Escape');
  await hidden(page, id);
  assert.equal(await menu.isVisible(), true, 'Escape closed the menu with its bubble');
  await page.keyboard.press('Escape');
  await menu.waitFor({ state: 'hidden' });
  assert.equal(await opener.evaluate((node) => node === document.activeElement), true, 'focus did not return to the ⋯');
  assert.deepEqual(failures, []);
});

test('chromium: 390 × 844 — a locked row of the ⋯ menu\'s sheet is one line with an ⓘ, its reason a bubble on a tap and on focus, closed by Escape before the sheet', async (t) => {
  const browser = await sharedChromium(t);
  const { page, failures, menu } = await openMenu(browser, 390, 844);
  const items = await itemsOf(menu);
  const [first] = assertLockedItems(items, '390');
  const id = first.bubbleIds[0];
  const item = menu.getByRole('menuitem', { name: first.name, exact: true });
  const sheet = page.getByRole('dialog', { name: `Actions for ${DATED.title}` });

  // A tap shows it and runs nothing; a tap elsewhere in the sheet closes it.
  // The screen's own tap at the row: Playwright's would wait for an `aria-disabled` row to be enabled.
  await item.scrollIntoViewIfNeeded();
  const box = await item.boundingBox();
  await page.touchscreen.tap(box.x + box.width / 2, box.y + box.height / 2);
  await showing(page, id);
  const shown = await bubble(page, id);
  assert.equal(shown.text, first.description, 'the bubble says something other than the row\'s description');
  assert.ok(shown.isInside, 'the bubble stands outside the screen');
  assert.equal(await sheet.isVisible(), true, 'a tap on a locked row closed the sheet');
  await sheet.getByRole('heading', { name: `Actions for ${DATED.title}` }).tap();
  await hidden(page, id);

  // Focus and Escape: the bubble first, then the sheet.
  await item.focus();
  await showing(page, id);
  await page.keyboard.press('Escape');
  await hidden(page, id);
  assert.equal(await sheet.isVisible(), true, 'the first Escape closed the sheet, not the bubble');
  await page.keyboard.press('Escape');
  await sheet.waitFor({ state: 'hidden' });
  assert.deepEqual(failures, []);
});
