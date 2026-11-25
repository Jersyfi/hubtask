// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: where the reason bubble of a locked item of the entry's ⋯ menu stands
// (design-system.md §4, "Where a bubble stands"), walked with the classic scrollbars a mouse's
// system draws, which take room at the window's edge. At 1280 the ⋯ is at the right edge: the
// reason of "Move up" turns to the menu's left side, one or two lines as wide as its words, its tip
// on the row's middle and outside its rounded corners, with no border wider than a hairline - in
// English and German, light and dark. At the window's foot it lines up with the row's bottom. On a
// phone a locked row's reason stands above the row across the screen, its tip on the row's ⓘ.
// A resting mouse shows a reason after `--dur-intent`, and the next locked row's at once while one
// shows, one at a time. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';
import { ACCOUNT, ITEMS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { assertTip, measureTip, middleOf } from './tipmeasure.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The fixture's first task of its list: "Move up" is locked for it. */
const FIRST = ITEMS[0];

const WORDS = {
  en: { up: 'Move up', out: 'Move out one level' },
  de: { up: 'Nach oben', out: 'Eine Ebene nach außen verschieben' },
};

/** The entry's page in a tab with scrollbars, in a language and a theme, its ⋯ menu open. */
async function openMenu(t, { width, height, language, scheme }) {
  const browser = await sharedChromium(t, { hasScrollbars: true });
  const isPhone = width < 600;
  const context = await browser.newContext({ viewport: { width, height }, colorScheme: scheme, ...(isPhone ? { hasTouch: true, isMobile: true } : {}) });
  await context.route('**/api/v1/**', (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^.*\/api\/v1/, '');
    if (language === 'de' && path === '/accounts/me' && request.method() === 'GET') return route.fulfill({ json: { ...ACCOUNT, locale: 'de' } });
    return stub(route);
  });
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/items/${FIRST.id}`);
  await page.waitForFunction((lang) => document.documentElement.lang === lang, language, { timeout: 15_000 });
  await page.getByRole('heading', { name: FIRST.title }).first().waitFor({ timeout: 15_000 });
  const opener = page.locator('[data-opener="entry-menu"]').filter({ visible: true }).first();
  await opener.waitFor({ timeout: 15_000 });
  if (isPhone) await opener.tap();
  else await opener.click();
  const menu = page.getByRole('menu').first();
  await menu.waitFor({ timeout: 5_000 });
  const up = menu.getByRole('menuitem', { name: WORDS[language].up, exact: true });
  await page.waitForFunction((name) => [...document.querySelectorAll('[role="menuitem"][aria-disabled="true"]')].some((each) => each.textContent?.trim() === name), WORDS[language].up, { timeout: 5_000 });
  return { page, failures, menu, up };
}

/** Whether the bubble with this id shows. */
const isShown = (page, id) => page.evaluate((bubbleId) => {
  const node = document.getElementById(bubbleId);
  return node !== null && !node.hidden;
}, id);

/** The bubble that describes `item`, once it shows. */
async function bubbleOf(page, item) {
  const id = (await item.getAttribute('aria-describedby')).split(/\s+/)[0];
  await page.waitForFunction((bubbleId) => {
    const node = document.getElementById(bubbleId);
    return node !== null && !node.hidden && node.dataset.hbtTip !== undefined;
  }, id, { timeout: 5_000 });
  return page.locator(`[id="${id}"]`);
}

for (const language of ['en', 'de']) {
  for (const scheme of ['light', 'dark']) {
    test(`chromium: 1280 × 800, scrollbars, ${language}, ${scheme} — the reason of a locked item in a menu at the right edge turns left, one or two lines, its tip on the row`, async (t) => {
      const { page, failures, menu, up } = await openMenu(t, { width: 1280, height: 800, language, scheme });
      const scrollbar = await page.evaluate(() => innerWidth - document.documentElement.clientWidth);
      assert.ok(scrollbar > 0, 'the window draws no classic scrollbar: this walk would not see what it is for');

      await up.hover();
      const bubble = await bubbleOf(page, up);
      const measured = await measureTip(bubble);
      const surface = await menu.evaluate((list) => list.closest('[data-hbt-anchored]').getBoundingClientRect().left);
      assert.equal(measured.side, 'right', 'the bubble did not turn to the menu\'s left');
      assert.ok(measured.box.right <= surface, 'the bubble covers the menu');
      assert.ok(measured.lines >= 1 && measured.lines <= 2, `the reason takes ${measured.lines} lines`);
      assert.ok(measured.box.width >= 120, `the bubble is squeezed to ${measured.box.width} px`);
      const row = await middleOf(up);
      assertTip(measured, row, `${language} ${scheme}`);
      assert.deepEqual(failures, []);
    });
  }
}

test('chromium: 1280 × 300, scrollbars — at the window\'s foot a reason lines up with its row\'s bottom, inside the window, its tip on the row', async (t) => {
  const { page, failures, menu } = await openMenu(t, { width: 1280, height: 300, language: 'en', scheme: 'light' });
  const item = menu.getByRole('menuitem', { name: WORDS.en.out, exact: true });
  // The list scrolled by the wheel until the locked row is its last, at the window's foot.
  const list = await middleOf(menu);
  await page.mouse.move(list.x, list.y);
  const by = await item.evaluate((row) => row.getBoundingClientRect().bottom - row.closest('[role="menu"]').getBoundingClientRect().bottom);
  await page.mouse.wheel(0, by);
  await page.waitForFunction(() => {
    const row = [...document.querySelectorAll('[role="menuitem"]')].find((each) => each.textContent?.trim() === 'Move out one level');
    const box = row.getBoundingClientRect();
    return box.bottom <= row.closest('[role="menu"]').getBoundingClientRect().bottom + 1 && box.bottom > innerHeight - 80;
  });
  const row = await middleOf(item);
  await item.hover();
  const bubble = await bubbleOf(page, item);
  const measured = await measureTip(bubble);
  assert.ok(row.box.y + measured.box.height > measured.window.height - 16, 'the row is not near enough the foot for this walk');
  assert.ok(measured.box.bottom <= measured.window.height - 8, 'the bubble runs past the window\'s foot');
  assert.ok(measured.box.top < row.box.y, 'the bubble does not line up with the row\'s bottom');
  assertTip(measured, row, 'foot');
  assert.deepEqual(failures, []);
});

for (const scheme of ['light', 'dark']) {
  test(`chromium: 390 × 844, ${scheme} — a locked row's reason in the sheet stands above it across the screen, its tip on the row's ⓘ`, async (t) => {
    const { page, failures, up } = await openMenu(t, { width: 390, height: 844, language: 'de', scheme });
    await up.scrollIntoViewIfNeeded();
    const box = await up.boundingBox();
    await page.touchscreen.tap(box.x + box.width / 2, box.y + box.height / 2);
    const bubble = await bubbleOf(page, up);
    const measured = await measureTip(bubble);
    assert.equal(measured.side, 'bottom', 'the reason does not stand above its row');
    assert.ok(measured.box.bottom <= box.y, 'the reason covers its row');
    assert.ok(Math.abs(measured.box.width - (measured.window.width - 32)) <= 1, `the reason is ${measured.box.width} px wide, not the screen less its edges`);
    // The tip points at the row's ⓘ, an element without a role of its own. The ⓘ stands nearer
    // the screen's edge than sp-100, the corner and the tip together: the tip comes as near it as
    // its straight edge lets it.
    const mark = await middleOf(up.locator('[data-reason-mark]'));
    assertTip(measured, mark, `phone ${scheme}`, { mayStop: true });
    assert.ok(Math.abs(measured.centre.x - mark.x) <= 2, 'the tip is not at the row\'s ⓘ');
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 × 800 — a resting mouse shows a reason after dur-intent; the next shows at once, and only one shows', async (t) => {
  const { page, failures, menu, up } = await openMenu(t, { width: 1280, height: 800, language: 'en', scheme: 'light' });
  const top = menu.getByRole('menuitem', { name: 'Move to the top', exact: true });
  assert.equal(await top.getAttribute('aria-disabled'), 'true', 'the fixture\'s first task no longer has "Move to the top" locked');
  // In the page: when the pointer entered each row, and when its bubble stopped being hidden.
  const times = async (row) => row.evaluate((node) => {
    const bubble = document.getElementById(node.getAttribute('aria-describedby').split(/\s+/)[0]);
    const seen = { entered: undefined, shown: undefined };
    node.addEventListener('pointerenter', (event) => { seen.entered ??= event.timeStamp; }, { once: true });
    new MutationObserver((changes, observer) => {
      if (bubble.hidden) return;
      seen.shown = performance.now();
      observer.disconnect();
    }).observe(bubble, { attributes: true, attributeFilter: ['hidden'] });
    window.bubbleTimes ??= {};
    window.bubbleTimes[node.textContent.trim()] = seen;
  });
  const intent = await page.evaluate(() => {
    const value = getComputedStyle(document.documentElement).getPropertyValue('--dur-intent').trim();
    return value.endsWith('ms') ? Number.parseFloat(value) : Number.parseFloat(value) * 1000;
  });
  assert.ok(intent > 0, 'no --dur-intent');
  await times(up);
  await times(top);

  const at = await middleOf(up);
  await page.mouse.move(at.x, at.y);
  await bubbleOf(page, up);
  const first = await page.evaluate(() => window.bubbleTimes['Move up']);
  assert.ok(first.shown - first.entered >= intent - 1, `the reason showed ${first.shown - first.entered} ms after the pointer arrived, before dur-intent (${intent} ms)`);

  const next = await middleOf(top);
  await page.mouse.move(next.x, next.y);
  await bubbleOf(page, top);
  const second = await page.evaluate(() => window.bubbleTimes['Move to the top']);
  assert.ok(second.shown - second.entered < intent, `moving on from one reason, the next waited ${second.shown - second.entered} ms`);
  const upId = (await up.getAttribute('aria-describedby')).split(/\s+/)[0];
  assert.equal(await isShown(page, upId), false, 'two reasons show at once');
  assert.deepEqual(failures, []);
});

