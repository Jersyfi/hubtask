// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: where the design system's reason bubble at a field and its HelpTip stand
// (design-system.md §4, "Where a bubble stands"), walked in the built workbench under the
// product's `style-src 'self'`, with the classic scrollbars a mouse's system draws. A field's
// reason stands above the field; a help tip below its ⓘ, and above it where the window has no
// room below. Each tip points at its element within a pixel and stays out of the surface's rounded
// corners, no border is wider than a hairline, light and dark. On a phone, with the ⓘ at the
// screen's right edge, the help tip comes nearer that edge - never nearer than sp-100 - so that
// its tip reaches the ⓘ on the straight part of its edge. A help tip whose ⓘ the page scrolls out
// of the window closes. At the stage's 200 % zoom both stand as they do at 100 %. In forced colours no tip breaks the outline. Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';
import { serve } from './serve.mjs';
import { assertTip, measureTip, middleOf } from './tipmeasure.mjs';

const WORKBENCH = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'packages', 'design-system', 'dist', 'workbench');

let server;
test.before(async () => {
  assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
  server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
});
test.after(() => server?.close());

/** The workbench at a story and a theme, in a tab with scrollbars. */
async function story(t, id, theme, { width = 1280, height = 800, touch = false, zoom } = {}) {
  const browser = await sharedChromium(t, { hasScrollbars: true });
  const context = await browser.newContext({ viewport: { width, height }, ...(touch ? { hasTouch: true, isMobile: true } : {}) });
  const page = await context.newPage();
  await page.goto(`${server.origin}/?story=${id}&theme=${theme}${zoom ? `&zoom=${zoom}` : ''}`);
  return page;
}

/** The help tip's popover, once its tip is placed. */
async function helpOf(page, subject) {
  const surface = page.getByRole('dialog', { name: subject });
  await surface.waitFor();
  await page.waitForFunction((name) => [...document.querySelectorAll('[role="dialog"]')].some((each) => each.getAttribute('aria-label') === name && each.dataset.hbtTip !== undefined), subject);
  return surface;
}

for (const theme of ['light', 'dark']) {
  test(`chromium: ${theme} — a field's reason stands above the field, its tip on it`, async (t) => {
    const page = await story(t, 'wave-1-forms-input--unavailable', theme);
    const field = page.getByRole('textbox', { name: 'Title', exact: true });
    await field.focus();
    const id = await field.getAttribute('aria-describedby');
    await page.waitForFunction((bubbleId) => document.getElementById(bubbleId)?.dataset.hbtTip !== undefined, id.split(/\s+/)[0]);
    const bubble = page.locator(`[id="${id.split(/\s+/)[0]}"]`);
    const measured = await measureTip(bubble);
    const { box } = await middleOf(field);
    assert.ok(measured.box.bottom <= box.y, 'the reason does not stand above the field');
    // Into a field wider than the bubble, the tip points sp-300 past the field's start.
    assertTip(measured, { x: box.x + 24, y: box.y + box.height / 2 }, `field ${theme}`, { side: 'bottom' });
    assert.ok(measured.box.width <= 240.5, `the reason is ${measured.box.width} px wide, more than tooltip.max`);
  });

  test(`chromium: ${theme} — a help tip stands below its ⓘ, and above it where the window has no room below`, async (t) => {
    const page = await story(t, 'wave-1-overlays-help-tip--label', theme);
    const tip = page.getByRole('button', { name: 'More about Expires', exact: true });
    await tip.click();
    const below = await measureTip(await helpOf(page, 'Expires'));
    assertTip(below, await middleOf(tip), `below ${theme}`, { side: 'top' });
    await page.keyboard.press('Escape');
    await page.getByRole('dialog', { name: 'Expires' }).waitFor({ state: 'detached' });

    // The window ends a little under the ⓘ: the popover goes above it.
    const { box } = await middleOf(tip);
    await page.setViewportSize({ width: 1280, height: Math.ceil(box.y + box.height + 60) });
    await tip.click();
    const above = await measureTip(await helpOf(page, 'Expires'));
    assertTip(above, await middleOf(tip), `above ${theme}`, { side: 'bottom' });
  });

  test(`chromium: 390 × 844, ${theme} — a help tip at the right edge of a phone reaches its ⓘ outside its rounded corner`, async (t) => {
    const page = await story(t, 'wave-1-overlays-help-tip--long', theme, { width: 390, height: 844, touch: true });
    const tip = page.getByRole('button', { name: 'More about Guests at the end', exact: true });
    await tip.waitFor();
    // The line is set at the screen's edge, sp-200 from it, as a field's label row stands on a phone.
    await tip.evaluate((node) => {
      const line = node.closest('p');
      line.style.position = 'fixed';
      line.style.insetInlineEnd = '16px';
      line.style.insetBlockStart = '120px';
    });
    const mark = await middleOf(tip);
    assert.ok(390 - (mark.box.x + mark.box.width) <= 16.5, 'the ⓘ is not at the screen\'s edge');
    await tip.tap();
    const measured = await measureTip(await helpOf(page, 'Guests at the end'));
    assertTip(measured, mark, `phone edge ${theme}`, { side: 'top' });
    // Its edge came nearer the screen's than sp-200 for the tip, and no nearer than sp-100.
    const fromEdge = measured.window.width - measured.box.right;
    assert.ok(fromEdge >= 8 - 0.5, `the help tip stands ${fromEdge} px from the screen's edge, nearer than sp-100`);
    assert.ok(fromEdge < 16, `the help tip stands ${fromEdge} px from the edge: this walk does not reach the case it is for`);
    assert.ok(measured.radius >= 12, 'the help tip lost its rounded corners');
  });
}

test('chromium: a help tip closes when the page scrolls its ⓘ out of the window', async (t) => {
  const page = await story(t, 'wave-1-overlays-help-tip--label', 'light', { width: 1280, height: 420 });
  const tip = page.getByRole('button', { name: 'More about Expires', exact: true });
  await tip.click();
  await helpOf(page, 'Expires');
  // The wheel over the page, below the popover, until the ⓘ is above the window.
  const { box } = await middleOf(tip);
  await page.mouse.move(640, 400);
  await page.mouse.wheel(0, box.y + box.height + 40);
  await page.waitForFunction((button) => button.getBoundingClientRect().bottom <= 0, await tip.elementHandle());
  await page.getByRole('dialog', { name: 'Expires' }).waitFor({ state: 'detached' });
  assert.equal(await tip.getAttribute('aria-expanded'), 'false');
});

test('chromium: in forced colours the bubble and the help tip draw no tip over their outline', async (t) => {
  const page = await story(t, 'wave-1-forms-input--unavailable', 'light');
  await page.emulateMedia({ forcedColors: 'active' });
  const field = page.getByRole('textbox', { name: 'Title', exact: true });
  await field.focus();
  const id = (await field.getAttribute('aria-describedby')).split(/\s+/)[0];
  await page.waitForFunction((bubbleId) => document.getElementById(bubbleId)?.dataset.hbtTip !== undefined, id);
  const bubble = await page.locator(`[id="${id}"]`).evaluate((node) => ({ tip: getComputedStyle(node, '::before').display, border: getComputedStyle(node).borderTopStyle }));
  assert.deepEqual(bubble, { tip: 'none', border: 'solid' }, 'the bubble\'s outline is broken by its tip, or it has none');

  await page.goto(page.url().replace('wave-1-forms-input--unavailable', 'wave-1-overlays-help-tip--label'));
  await page.getByRole('button', { name: 'More about Expires', exact: true }).click();
  const surface = await helpOf(page, 'Expires');
  const hidden = await surface.evaluate((node) => getComputedStyle(node.querySelector(':scope > [aria-hidden="true"]:empty')).display);
  assert.equal(hidden, 'none', 'the help tip\'s tip breaks its outline');
});

test('chromium: at the stage\'s 200 % zoom a field\'s reason and a help tip still point at their element, outside their corners', async (t) => {
  const page = await story(t, 'wave-1-forms-input--unavailable', 'light', { zoom: '200' });
  const field = page.getByRole('textbox', { name: 'Title', exact: true });
  await field.focus();
  const id = (await field.getAttribute('aria-describedby')).split(/\s+/)[0];
  await page.waitForFunction((bubbleId) => document.getElementById(bubbleId)?.dataset.hbtTip !== undefined, id);
  const measured = await measureTip(page.locator(`[id="${id}"]`));
  assert.equal(measured.zoom, 2, 'the stage is not zoomed: this walk does not reach its case');
  const { box } = await middleOf(field);
  assert.ok(measured.box.bottom <= box.y, 'the reason does not stand above the field');
  assertTip(measured, { x: box.x + 24 * measured.zoom, y: box.y + box.height / 2 }, 'field at 200 %', { side: 'bottom' });

  await page.goto(page.url().replace('wave-1-forms-input--unavailable', 'wave-1-overlays-help-tip--label'));
  const tip = page.getByRole('button', { name: 'More about Expires', exact: true });
  await tip.click();
  const help = await measureTip(await helpOf(page, 'Expires'));
  assertTip(help, await middleOf(tip), 'help tip at 200 %', { side: 'top' });
});
