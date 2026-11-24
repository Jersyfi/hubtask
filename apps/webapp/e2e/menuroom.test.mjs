// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the entry's ⋯ menu on its own page at 1280 × 800, 768 and 390 px (the
// design system's Menu, its sheet on a phone) - no item's box overlaps another and no item's words
// run out of its box, and a menu taller than its room scrolls inside it instead of squeezing its
// items. Its scroll shades (scrollEdges) show only at an edge with more beyond it - at its top, in
// its middle, at its end, and none where the window holds the whole menu - painted over the items
// and inside the surface's rounded corners. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';
import { serve } from './serve.mjs';
import { ITEMS, signedIn } from './fixture.mjs';
import { assertShades, frameBox, readEdges, scrollToShare, shadeChanges } from './scrolledges.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

/** The fixture's task with a due date: its menu has every item, four of them with a reason. */
const DATED = ITEMS.find((each) => each.due_at);

/** The entry's page at a size, with its ⋯ menu open; the menu's list of items. */
async function openMenu(browser, width, height) {
  const { page, failures } = await signedIn(browser, width, height, width < 600 ? { hasTouch: true, isMobile: true } : {});
  await page.goto(`${served.origin}/items/${DATED.id}`);
  await page.getByRole('heading', { name: DATED.title }).first().waitFor({ timeout: 15_000 });
  const opener = page.locator('[data-opener="entry-menu"]').filter({ visible: true }).first();
  await opener.waitFor({ timeout: 15_000 });
  await opener.click();
  const menu = page.getByRole('menu', { name: `Actions for ${DATED.title}` });
  await menu.waitFor({ timeout: 5_000 });
  // The level is read when the menu is reached for; its answer turns *Move up* on.
  await page.waitForFunction(() => [...document.querySelectorAll('[role="menuitem"]')].some((each) => each.textContent?.includes('Move up') && each.getAttribute('aria-disabled') !== 'true'), null, { timeout: 5_000 }).catch(() => undefined);
  return { page, failures, menu };
}

/** Each item's box, the boxes of what is drawn inside it, and the box that scrolls the items. */
function measure(menu) {
  return menu.evaluate((list) => {
    const round = (value) => Math.round(value * 10) / 10;
    const items = [...list.querySelectorAll('[role="menuitem"]')].map((item) => {
      const box = item.getBoundingClientRect();
      const inner = [...item.querySelectorAll('span')].filter((each) => each.getClientRects().length > 0).map((each) => each.getBoundingClientRect());
      return {
        name: (item.querySelector('span')?.textContent ?? '').trim(),
        top: round(box.top),
        bottom: round(box.bottom),
        innerTop: round(Math.min(box.top, ...inner.map((each) => each.top))),
        innerBottom: round(Math.max(box.bottom, ...inner.map((each) => each.bottom))),
      };
    });
    let scroller = list;
    while (scroller && !/auto|scroll/.test(getComputedStyle(scroller).overflowY)) scroller = scroller.parentElement;
    return { items, scrolls: scroller !== null && scroller.scrollHeight > scroller.clientHeight + 1 };
  });
}

for (const [width, height] of [[1280, 800], [768, 1024], [390, 844]]) {
  test(`chromium: ${width} × ${height} — the entry's ⋯ menu keeps every item in its own box and scrolls when taller than its room`, async (t) => {
    const browser = await sharedChromium(t);
    const { menu, failures } = await openMenu(browser, width, height);
    const { items, scrolls } = await measure(menu);
    assert.ok(items.length >= 18, `the menu lists ${items.length} items, not the fixture's eighteen`);
    for (const item of items) {
      assert.ok(item.innerBottom <= item.bottom + 0.5 && item.innerTop >= item.top - 0.5,
        `“${item.name}” runs out of its box: its words reach ${item.innerBottom}, its box ends at ${item.bottom}`);
    }
    for (let at = 1; at < items.length; at++) {
      assert.ok(items[at].top >= items[at - 1].bottom - 0.5,
        `“${items[at - 1].name}” overlaps “${items[at].name}”: ${items[at - 1].bottom} past ${items[at].top}`);
    }
    assert.ok(scrolls, `at ${width} × ${height} the eighteen items do not fit, and nothing scrolls them`);
    assert.deepEqual(failures, []);
  });
}

test('chromium: 1280 × 800 — the ⋯ menu shades only the edge with more beyond it, over its items and inside its corners', async (t) => {
  const browser = await sharedChromium(t);
  const { page, menu, failures } = await openMenu(browser, 1280, 800);
  await page.waitForFunction(() => document.querySelector('[role="menu"]')?.parentElement?.hasAttribute('data-scroll-edges'), null, { timeout: 5_000 }).catch(() => undefined);
  assertShades(assert, await readEdges(menu), { start: false, end: true }, 'at its top');
  await scrollToShare(menu, 0.5);
  assertShades(assert, await readEdges(menu), { start: true, end: true }, 'in its middle');
  // Over the items: the item under the pointer at the top does not cover the shade there.
  const frame = await frameBox(menu);
  await page.mouse.move(frame.x + frame.width / 2, frame.y + 8);
  assert.ok(await shadeChanges(page, menu, 'start', { x: frame.x + frame.width / 4, y: frame.y + 1, width: frame.width / 2, height: 3 }), 'the shade at the top is hidden by the item under the pointer');
  await scrollToShare(menu, 1);
  assertShades(assert, await readEdges(menu), { start: true, end: false }, 'at its end');
  assert.deepEqual(failures, []);
});

test('chromium: 1280 × 1600 — a ⋯ menu the window holds whole scrolls nowhere and shows no shade', async (t) => {
  const browser = await sharedChromium(t);
  const { menu, failures } = await openMenu(browser, 1280, 1600);
  const read = await readEdges(menu);
  assert.equal(read.scrolls, false, 'the menu scrolls in a window that holds it whole');
  assertShades(assert, read, { start: false, end: false }, 'held whole');
  assert.deepEqual(failures, []);
});

test('chromium: 390 × 844 — the ⋯ menu’s sheet shades its list the same way', async (t) => {
  const browser = await sharedChromium(t);
  const { page, menu, failures } = await openMenu(browser, 390, 844);
  await page.waitForFunction(() => document.querySelector('[role="menu"]')?.parentElement?.hasAttribute('data-scroll-edges'), null, { timeout: 5_000 }).catch(() => undefined);
  assertShades(assert, await readEdges(menu), { start: false, end: true }, 'the sheet at its top');
  await scrollToShare(menu, 0.5);
  assertShades(assert, await readEdges(menu), { start: true, end: true }, 'the sheet in its middle');
  await scrollToShare(menu, 1);
  assertShades(assert, await readEdges(menu), { start: true, end: false }, 'the sheet at its end');
  assert.deepEqual(failures, []);
});
