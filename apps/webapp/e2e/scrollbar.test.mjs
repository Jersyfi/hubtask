// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the product's one scrollbar (design-system.md §6, "One scrollbar"), walked
// with the classic scrollbars a mouse's system draws. The page, the entry's ⋯ menu at 1280 and the
// *Help and shortcuts* dialog draw it alike, light and dark: a thumb narrower than the hit zone of
// `layout.nav.scrollbar`, round, inset from both edges, on a track that shows the surface behind
// it; faint at rest, `border.default` while its surface is hovered, holds focus or scrolls - the
// page's only while it scrolls - and wider under the pointer. The page and a menu keep their bar's
// room before they scroll. A system that overlays its scrollbars keeps its own (the probe, asked
// again on a resize or focus), and so do forced colours. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';
import { ITEMS, stub } from './fixture.mjs';
import { serve } from './serve.mjs';
import { PAGE, distance, measureBar, tokenColour } from './scrollbarmeasure.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const ENTRY = ITEMS[0];

/** The entry's page, signed in, in a tab with or without classic scrollbars. */
async function openEntry(t, { width = 1280, height = 800, scheme = 'light', hasScrollbars = true, forcedColors } = {}) {
  const browser = await sharedChromium(t, { hasScrollbars });
  const context = await browser.newContext({ viewport: { width, height }, colorScheme: scheme, ...(forcedColors ? { forcedColors } : {}) });
  await context.route('**/api/v1/**', (route) => stub(route));
  await context.addInitScript(() => {
    sessionStorage.setItem('hubtask.bearer', 'e2e-bearer');
    sessionStorage.setItem('hubtask.refresh', 'e2e-refresh');
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  await page.goto(`${served.origin}/items/${ENTRY.id}`);
  await page.getByRole('heading', { name: ENTRY.title }).first().waitFor({ timeout: 15_000 });
  return { browser, page, failures };
}

/** Waits until nothing on the page moves: an overlay's entrance fades it in. */
const still = (page) => page.waitForFunction(() => document.getAnimations().every((each) => each.playState !== 'running'));

/** The entry's ⋯ menu, open and still. Its opener's name is `app.workspace.actions`. */
async function openMenu(page) {
  await page.getByRole('button', { name: `Actions for ${ENTRY.title}`, exact: true }).filter({ visible: true }).first().click();
  const menu = page.getByRole('menu').first();
  await menu.waitFor({ timeout: 5_000 });
  await still(page);
  return menu;
}

/**
 * Runs `measure` while `target` (an element handle, or `PAGE`) goes on scrolling: a scroll event
 * each frame keeps its `data-scrolling` from settling until the measurement is done.
 */
async function whileScrolling(page, target, measure) {
  const surface = target === PAGE ? await page.evaluateHandle(() => document) : target;
  await surface.evaluate((node) => {
    window.hbtScrolling = true;
    const tick = () => {
      if (!window.hbtScrolling) return;
      node.dispatchEvent(new Event('scroll'));
      requestAnimationFrame(tick);
    };
    tick();
  });
  try {
    return await measure();
  } finally {
    await page.evaluate(() => { window.hbtScrolling = false; });
  }
}

/** The hit zone the bar takes, from the token. */
const hitZone = (page) => page.evaluate(() => Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--layout-nav-scrollbar')));

/** The product's bar: in the hit zone, round, inset from both edges, its track transparent. */
function assertDrawn(bar, zone, where) {
  assert.equal(bar.gutter, zone, `${where}: the bar takes ${bar.gutter} px, not the hit zone`);
  assert.ok(bar.drawn > 0 && bar.drawn < zone, `${where}: the thumb paints ${bar.drawn} columns`);
  assert.ok(bar.fromStart > 0 && bar.fromEnd > 0, `${where}: the thumb touches an edge of its zone`);
  assert.ok(bar.corner < bar.body / 2, `${where}: the thumb's top corner shows as much as its middle - it is square`);
  if (bar.track) assert.ok(distance(bar.track.colour, bar.track.beside) <= 1, `${where}: the track is ${bar.track.colour}, the surface ${bar.track.beside}`);
}

for (const scheme of ['light', 'dark']) {
  test(`chromium: 1280 × 800, scrollbars, ${scheme} — the page's bar is thin, round and faint with the pointer in the window, border.default while it scrolls, wider under the pointer`, async (t) => {
    const { browser, page, failures } = await openEntry(t, { scheme });
    assert.equal(await page.evaluate(() => document.documentElement.dataset.scrollbars), undefined, 'the probe took classic scrollbars for overlay ones');
    assert.ok(await page.evaluate(() => document.documentElement.scrollHeight > innerHeight), 'the page does not scroll at this height');
    const zone = await hitZone(page);
    const strong = await tokenColour(page, PAGE, '--border-default');

    // The page is hovered while the pointer is anywhere in the window: that is rest for its bar.
    await page.mouse.move(600, 400);
    const rest = await measureBar(browser, page, PAGE);
    assertDrawn(rest, zone, 'the page at rest');
    assert.ok(rest.track, 'the thumb fills the track: no track to see');
    assert.ok(distance(rest.thumb, strong) > 20 && distance(rest.thumb, rest.track.beside) > 20, `at rest the thumb is ${rest.thumb}: not faint between ${rest.track.beside} and ${strong}`);

    await page.mouse.wheel(0, 40);
    await page.waitForFunction(() => document.documentElement.hasAttribute('data-scrolling'));
    const scrolling = await whileScrolling(page, PAGE, () => measureBar(browser, page, PAGE));
    assertDrawn(scrolling, zone, 'the page scrolling');
    assert.ok(distance(scrolling.thumb, strong) <= 2, `while it scrolls the thumb is ${scrolling.thumb}, not border.default ${strong}`);
    await page.waitForFunction(() => !document.documentElement.hasAttribute('data-scrolling'));

    // The pointer on the thumb itself: it grows, still inside its zone.
    const right = await page.evaluate(() => document.documentElement.clientWidth);
    await page.mouse.move(right + zone / 2, 60);
    const grabbed = await measureBar(browser, page, PAGE);
    assert.ok(grabbed.drawn > rest.drawn && grabbed.drawn < zone, `under the pointer the thumb paints ${grabbed.drawn} columns, at rest ${rest.drawn}`);
    assert.ok(distance(grabbed.thumb, rest.thumb) > 20 && distance(grabbed.thumb, scrolling.thumb) > 6, `under the pointer the thumb is ${grabbed.thumb}: not the strong boundary`);
    assert.deepEqual(failures, []);
  });

  test(`chromium: 1280 × 800, scrollbars, ${scheme} — the entry's ⋯ menu draws the same bar, border.default while it holds the focus`, async (t) => {
    const { browser, page, failures } = await openEntry(t, { scheme });
    const menu = await openMenu(page);
    assert.ok(await menu.evaluate((node) => node.scrollHeight > node.clientHeight && node.contains(document.activeElement)), 'the menu does not scroll, or holds no focus');
    const bar = await measureBar(browser, page, menu);
    assertDrawn(bar, await hitZone(page), 'the menu');
    const strong = await tokenColour(page, menu, '--border-default');
    assert.ok(distance(bar.thumb, strong) <= 2, `with the focus inside, the thumb is ${bar.thumb}, not border.default ${strong}`);
    assert.deepEqual(failures, []);
  });

  test(`chromium: 1280 × 800, scrollbars, ${scheme} — a dialog's body draws the same bar, faint at rest and border.default while hovered`, async (t) => {
    const { browser, page, failures } = await openEntry(t, { scheme });
    await page.keyboard.press('?');
    const dialog = page.getByRole('dialog', { name: 'Help and shortcuts' });
    await dialog.waitFor({ timeout: 5_000 });
    await still(page);
    // The box that scrolls: the nearest ancestor of a row that scrolls.
    const body = await dialog.getByText('Fold or unfold the sidebar').evaluateHandle((row) => {
      let node = row.parentElement;
      while (node && !(node.scrollHeight > node.clientHeight && /auto|scroll/.test(getComputedStyle(node).overflowY))) node = node.parentElement;
      return node;
    });
    assert.ok(await body.evaluate((node) => node !== null && node !== document.documentElement), 'the dialog does not scroll at this height');
    const zone = await hitZone(page);
    const strong = await tokenColour(page, body, '--border-default');
    const rest = await measureBar(browser, page, body);
    assertDrawn(rest, zone, 'the dialog at rest');
    assert.ok(distance(rest.thumb, strong) > 20, `at rest the thumb is already border.default: ${rest.thumb}`);
    // While it scrolls the thumb is drawn in full, and fades back once the scroll has settled.
    await body.evaluate((node) => { node.scrollTop = 40; });
    assert.equal(await body.evaluate((node) => new Promise((resolve) => requestAnimationFrame(() => resolve(node.hasAttribute('data-scrolling'))))), true, 'the scroll did not mark its box');
    const scrolling = await whileScrolling(page, body, () => measureBar(browser, page, body));
    assert.ok(distance(scrolling.thumb, strong) <= 2, `while it scrolls the thumb is ${scrolling.thumb}, not border.default ${strong}`);
    await body.evaluate((node) => new Promise((resolve) => {
      const done = () => (node.hasAttribute('data-scrolling') ? requestAnimationFrame(done) : resolve());
      done();
    }));
    const settled = await measureBar(browser, page, body);
    assert.ok(distance(settled.thumb, rest.thumb) <= 2, `after the scroll the thumb stays ${settled.thumb}, not ${rest.thumb} as at rest`);
    await body.hover();
    const hovered = await measureBar(browser, page, body);
    assertDrawn(hovered, zone, 'the dialog hovered');
    assert.ok(distance(hovered.thumb, strong) <= 2, `hovered the thumb is ${hovered.thumb}, not border.default ${strong}`);
    assert.deepEqual(failures, []);
  });
}

test('chromium: scrollbars — the page and a menu keep their bar\'s room before they scroll, so nothing shifts once they do', async (t) => {
  const widths = {};
  for (const height of [800, 2400]) {
    const { page, failures } = await openEntry(t, { height });
    const menu = await openMenu(page);
    widths[height] = await page.evaluate((list) => ({
      pageScrolls: document.documentElement.scrollHeight > innerHeight,
      page: document.documentElement.clientWidth,
      menuScrolls: list.scrollHeight > list.clientHeight,
      menu: list.clientWidth,
      menuOuter: list.offsetWidth,
    }), await menu.elementHandle());
    assert.deepEqual(failures, []);
  }
  assert.deepEqual([widths[800].pageScrolls, widths[800].menuScrolls, widths[2400].pageScrolls, widths[2400].menuScrolls], [true, true, false, false], 'this walk needs both to scroll at 800 and neither at 2400');
  assert.equal(widths[2400].page, widths[800].page, 'the page is wider before it scrolls');
  assert.equal(widths[2400].menu, widths[800].menu, 'the menu\'s rows are wider before it scrolls');
  assert.equal(widths[2400].menuOuter, widths[800].menuOuter, 'the menu is wider once it scrolls');
});

test('chromium: the probe — overlay scrollbars set data-scrollbars="overlay" and keep the system\'s bar; classic ones take the product\'s; a focus or a resize asks again', async (t) => {
  const overlay = await openEntry(t, { hasScrollbars: false });
  assert.equal(await overlay.page.evaluate(() => document.documentElement.dataset.scrollbars), 'overlay', 'a system that overlays its scrollbars was taken for a classic one');
  assert.equal(await overlay.page.evaluate(() => innerWidth - document.documentElement.clientWidth), 0, 'an overlay bar took room from the page');
  // Told the bars were classic, the window asks again when it takes the focus.
  await overlay.page.evaluate(() => { delete document.documentElement.dataset.scrollbars; dispatchEvent(new Event('focus')); });
  assert.equal(await overlay.page.evaluate(() => document.documentElement.dataset.scrollbars), 'overlay', 'the focus did not ask the system again');
  assert.deepEqual(overlay.failures, []);

  const classic = await openEntry(t);
  const zone = await hitZone(classic.page);
  assert.equal(await classic.page.evaluate(() => innerWidth - document.documentElement.clientWidth), zone);
  // The same window told its scrollbars overlay: the rule is gone, the system's own bar is back.
  await classic.page.evaluate(() => { document.documentElement.dataset.scrollbars = 'overlay'; });
  const system = await measureBar(classic.browser, classic.page, PAGE);
  assert.ok(system.gutter > zone, `with the rule gone the bar is ${system.gutter} px, the product's width`);
  assert.ok(system.drawn >= zone, `with the rule gone the thumb paints ${system.drawn} columns, the product's thin one`);
  // The system's answer is asked again: a resize finds the classic bars, and the rule is back.
  await classic.page.evaluate(() => dispatchEvent(new Event('resize')));
  assert.equal(await classic.page.evaluate(() => document.documentElement.dataset.scrollbars), undefined, 'a resize did not ask the system again');
  assert.equal(await classic.page.evaluate(() => innerWidth - document.documentElement.clientWidth), zone);
  assert.deepEqual(classic.failures, []);
});

test('chromium: scrollbars, forced colours — the system\'s bar stays', async (t) => {
  const { browser, page, failures } = await openEntry(t, { forcedColors: 'active' });
  assert.equal(await page.evaluate(() => matchMedia('(forced-colors: active)').matches), true);
  assert.equal(await page.evaluate(() => getComputedStyle(document.documentElement).scrollbarColor), 'auto');
  const bar = await measureBar(browser, page, PAGE);
  assert.ok(bar.gutter > await hitZone(page), `in forced colours the bar is ${bar.gutter} px, the product's width`);
  assert.deepEqual(failures, []);
});
