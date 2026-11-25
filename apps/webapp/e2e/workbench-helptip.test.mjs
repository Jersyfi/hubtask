// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the design system's HelpTip (design-system.md §4, "Help text") - the ⓘ
// beside a label or a heading that holds *more about* it. Named "More about ‹label›" and never part
// of the label's or the heading's name; `aria-expanded` says whether it is open. Enter, Space, a
// click and a tap open it, a hover alone never does; its words take focus; `Escape`, a press
// elsewhere and a second press close it, focus back on the ⓘ. On a phone it stays a popover at the
// ⓘ, not a sheet, inside the screen, and in a right-to-left box too. Walked in the built workbench
// under the product's `style-src 'self'`.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium, firefox, webkit } from 'playwright';

import { serve } from './serve.mjs';

const WORKBENCH = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'packages', 'design-system', 'dist', 'workbench');
const ENGINES = { chromium, firefox, webkit };

/** The workbench at a story, in a page of its own; closed with the browser. */
async function story(engine, name, { width = 1280, height = 800, touch = false } = {}) {
  assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
  const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
  const browser = await ENGINES[engine].launch().catch(async (error) => { await server.close(); throw error; });
  const close = async () => { await browser.close(); await server.close(); };
  try {
    const context = await browser.newContext({ viewport: { width, height }, ...(touch ? { hasTouch: true, isMobile: true } : {}) });
    const page = await context.newPage();
    await page.goto(`${server.origin}/?story=wave-1-overlays-help-tip--${name}`);
    return { page, close };
  } catch (error) {
    await close();
    throw error;
  }
}

/** The popover the ⓘ opens: a surface at the ⓘ, never the sheet a popover becomes on a phone. */
const popover = (page, subject) => page.locator('div[role="dialog"]').filter({ has: page.locator('.words') }).and(page.getByRole('dialog', { name: subject }));

/** Two frames: long enough for anything a pointer started to be drawn. */
const frames = (page) => page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(done))));

const focusIsOn = (locator) => locator.evaluate((node) => node === document.activeElement);
const wordsHaveFocus = (page) => page.evaluate(() => document.activeElement?.classList.contains('words') === true);

/** Whether all of the element is inside the window. */
const isInside = (locator) => locator.evaluate((node) => {
  const box = node.getBoundingClientRect();
  return box.left >= 0 && box.top >= 0 && box.right <= innerWidth && box.bottom <= innerHeight && box.width > 0;
});

for (const engine of (process.env.ENGINES ?? 'chromium').split(',')) {
  test(`the ⓘ beside a field's label opens by keyboard and returns focus, in ${engine}`, { timeout: 2 * 60_000 }, async () => {
    const { page, close } = await story(engine, 'label');
    try {
      const tip = page.getByRole('button', { name: 'More about Expires', exact: true });
      await tip.waitFor();
      await page.getByRole('textbox', { name: 'Expires', exact: true }).waitFor();
      assert.equal(await tip.getAttribute('aria-expanded'), 'false');

      await tip.focus();
      await page.keyboard.press('Enter');
      await popover(page, 'Expires').waitFor();
      assert.equal(await tip.getAttribute('aria-expanded'), 'true');
      assert.ok(await wordsHaveFocus(page), 'the words did not take focus, so a screen reader does not read them');
      assert.match(await popover(page, 'Expires').innerText(), /no default on purpose/);

      await page.keyboard.press('Escape');
      await popover(page, 'Expires').waitFor({ state: 'detached' });
      assert.equal(await tip.getAttribute('aria-expanded'), 'false');
      assert.ok(await focusIsOn(tip), 'Escape did not give focus back to the ⓘ');

      await page.keyboard.press('Space');
      await popover(page, 'Expires').waitFor();
      await page.keyboard.press('Escape');
      await popover(page, 'Expires').waitFor({ state: 'detached' });
      assert.ok(await focusIsOn(tip), 'Escape after Space did not give focus back to the ⓘ');
    } finally {
      await close();
    }
  });

  test(`the ⓘ opens on a click and never on a hover alone, in ${engine}`, { timeout: 2 * 60_000 }, async () => {
    const { page, close } = await story(engine, 'label');
    try {
      const tip = page.getByRole('button', { name: 'More about Expires', exact: true });
      await tip.waitFor();
      await tip.hover();
      await frames(page);
      assert.equal(await popover(page, 'Expires').count(), 0, 'the ⓘ opened on a hover');
      assert.equal(await tip.getAttribute('aria-expanded'), 'false');

      await tip.click();
      await popover(page, 'Expires').waitFor();
      assert.ok(await isInside(popover(page, 'Expires')), 'the popover stands outside the window');
      // A second press closes it.
      await tip.click();
      await popover(page, 'Expires').waitFor({ state: 'detached' });
      // A press elsewhere closes it, and focus goes back to the ⓘ.
      await tip.click();
      await popover(page, 'Expires').waitFor();
      await page.getByRole('textbox', { name: 'Expires', exact: true }).click();
      await popover(page, 'Expires').waitFor({ state: 'detached' });
      assert.equal(await tip.getAttribute('aria-expanded'), 'false');
    } finally {
      await close();
    }
  });

  test(`the ⓘ beside a heading and a setting's name is not part of their names, in ${engine}`, { timeout: 2 * 60_000 }, async () => {
    const { page, close } = await story(engine, 'heading');
    try {
      await page.getByRole('heading', { name: 'Guests', exact: true }).waitFor();
      await page.getByRole('switch', { name: 'Invite guests', exact: true }).waitFor();
      const section = page.getByRole('button', { name: 'More about Guests', exact: true });
      await section.click();
      await popover(page, 'Guests').waitFor();
      assert.ok(await isInside(popover(page, 'Guests')), 'the popover stands outside the window');
      await page.keyboard.press('Escape');
      await popover(page, 'Guests').waitFor({ state: 'detached' });
      assert.ok(await focusIsOn(section), 'Escape did not give focus back to the ⓘ');
      await page.getByRole('button', { name: 'More about Invite guests', exact: true }).click();
      await popover(page, 'Invite guests').waitFor();
    } finally {
      await close();
    }
  });
}

test('chromium: 390 × 844 — a tap opens the ⓘ as a popover at it, not a sheet, and a tap elsewhere closes it', { timeout: 2 * 60_000 }, async () => {
  const { page, close } = await story('chromium', 'phone', { width: 390, height: 844, touch: true });
  try {
    const tip = page.getByRole('button', { name: 'More about Expires', exact: true });
    await tip.waitFor();
    await tip.tap();
    await popover(page, 'Expires').waitFor();
    assert.equal(await page.locator('dialog[open]').count(), 0, 'the ⓘ opened a sheet on a phone');
    assert.ok(await isInside(popover(page, 'Expires')), 'the popover stands outside the screen');
    assert.equal(await tip.getAttribute('aria-expanded'), 'true');
    // The popover lies over the field on a phone's width: a tap below it is a tap elsewhere.
    const box = await popover(page, 'Expires').boundingBox();
    await page.touchscreen.tap(box.x + box.width / 2, Math.min(box.y + box.height + 24, 834));
    await popover(page, 'Expires').waitFor({ state: 'detached' });
  } finally {
    await close();
  }
});

test('chromium: in a right-to-left box the ⓘ follows its label and its popover stays inside the window', { timeout: 2 * 60_000 }, async () => {
  const { page, close } = await story('chromium', 'rtl');
  try {
    const tip = page.getByRole('button', { name: 'More about Expires', exact: true });
    await tip.waitFor();
    const [label, mark] = await Promise.all([page.getByText('Expires', { exact: true }).first().boundingBox(), tip.boundingBox()]);
    assert.ok(mark.x + mark.width <= label.x + 0.5, 'in a right-to-left box the ⓘ does not follow its label');
    await tip.click();
    await popover(page, 'Expires').waitFor();
    assert.ok(await isInside(popover(page, 'Expires')), 'the popover stands outside the window');
  } finally {
    await close();
  }
});
