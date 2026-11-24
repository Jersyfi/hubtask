// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the help texts' pass (design-system.md §4, "Help text"), walked on the
// busiest screens at 1280 × 800 and at 390 × 844 on a touch phone. The token form's *Expires*, the
// list of one's tokens and the AI settings keep what is needed now - a field's or a page's
// one sentence - visible, and say the rest behind an ⓘ named *More about …*: closed at first, its
// words nowhere on the screen, opened by a click or a tap as a popover inside the window, closed by
// `Escape` with focus back on the ⓘ; where the bar carries a list page's title, the ⓘ ends its
// introduction's sentence and opens under it. The create-hub page's *Private* keeps its
// description whole and visible, with no ⓘ: a description never moves (the owner's condition 1).
// Chromium only.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { sharedChromium } from './browser.mjs';
import { signedIn } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const SIZES = [[1280, 800], [390, 844]];

/** Whether a text is drawn anywhere on the screen, in a box with a size. */
const isDrawn = (page, text) => page.getByText(text, { exact: true }).filter({ visible: true }).count().then((count) => count > 0);

/**
 * Asserts one ⓘ: the visible sentence is drawn, the moved words are not until the ⓘ is pressed,
 * then they are, inside the window, and `Escape` closes them with focus back on the ⓘ.
 */
async function assertMore(page, { where, name, visible, more, isPhone }) {
  // Not exact: where the bar carries the title, the ⓘ's own name stands in the same line.
  await page.getByText(visible).filter({ visible: true }).first().waitFor({ timeout: 15_000 });
  assert.equal(await isDrawn(page, more), false, `${where}: the words behind the ⓘ are drawn before it is pressed`);
  const tip = page.getByRole('button', { name, exact: true }).filter({ visible: true });
  assert.equal(await tip.count(), 1, `${where}: no one ⓘ named “${name}”`);
  assert.equal(await tip.getAttribute('aria-expanded'), 'false', `${where}: the ⓘ says it is open before it is pressed`);
  if (isPhone) await tip.tap();
  else await tip.click();
  const words = page.getByText(more, { exact: true }).filter({ visible: true });
  await words.waitFor({ timeout: 5_000 });
  assert.equal(await tip.getAttribute('aria-expanded'), 'true', `${where}: the ⓘ does not say it is open`);
  const box = await words.boundingBox();
  const size = page.viewportSize();
  assert.ok(box && box.x >= 0 && box.y >= 0 && box.x + box.width <= size.width + 0.5 && box.y + box.height <= size.height + 0.5,
    `${where}: the words behind the ⓘ stand outside the window: ${JSON.stringify(box)}`);
  assert.equal(await page.locator('dialog[open]').filter({ hasText: more }).count(), 0, `${where}: the words open as a sheet, not a popover`);
  await page.keyboard.press('Escape');
  await words.waitFor({ state: 'hidden', timeout: 5_000 });
  assert.equal(await tip.evaluate((node) => node === document.activeElement), true, `${where}: focus did not go back to the ⓘ`);
}

/**
 * Asserts where a list page's ⓘ stands: beside the title where the page draws it, and at the end of
 * the introduction's sentence where the bar carries the title - never alone on a line above it - so
 * that its words open under the sentence instead of over it.
 */
async function assertPlace(page, { where, name, visible, more, isPhone }) {
  const tip = page.getByRole('button', { name, exact: true }).filter({ visible: true });
  const sentence = page.locator('p').filter({ hasText: visible }).filter({ visible: true }).first();
  await sentence.waitFor({ timeout: 15_000 });
  const isInSentence = await sentence.evaluate((node, label) => [...node.querySelectorAll('button')].some((button) => button.getAttribute('aria-label') === label || button.textContent.trim() === label), name);
  if (!isPhone) {
    assert.equal(isInSentence, false, `${where}: the ⓘ stands in the introduction, not beside the title`);
    const [title, button] = await Promise.all([page.getByRole('heading', { level: 1 }).filter({ visible: true }).boundingBox(), tip.boundingBox()]);
    const middle = button.y + button.height / 2;
    assert.ok(middle >= title.y && middle <= title.y + title.height, `${where}: the ⓘ is not on the title's line: ${JSON.stringify({ title, button })}`);
    return;
  }
  assert.equal(isInSentence, true, `${where}: the ⓘ is not at the end of the introduction's sentence`);
  const [box, button, lineHeight] = await Promise.all([
    sentence.boundingBox(),
    tip.boundingBox(),
    sentence.evaluate((node) => parseFloat(getComputedStyle(node).lineHeight)),
  ]);
  const middle = button.y + button.height / 2;
  assert.ok(middle >= box.y + box.height - lineHeight - 0.5 && middle <= box.y + box.height + 0.5,
    `${where}: the ⓘ is not on the sentence's last line: ${JSON.stringify({ box, button, lineHeight })}`);
  await tip.tap();
  const words = page.getByText(more, { exact: true }).filter({ visible: true });
  await words.waitFor({ timeout: 5_000 });
  const opened = await words.boundingBox();
  const overlap = Math.min(opened.y + opened.height, box.y + box.height) - Math.max(opened.y, box.y);
  assert.ok(overlap <= 0.5, `${where}: the ⓘ's words cover its sentence: ${JSON.stringify({ box, opened })}`);
  await page.keyboard.press('Escape');
  await words.waitFor({ state: 'hidden', timeout: 5_000 });
}

for (const [width, height] of SIZES) {
  const isPhone = width < 600;
  const touch = isPhone ? { hasTouch: true, isMobile: true } : {};

  test(`chromium: ${width} × ${height} — the token form keeps what Expires needs visible and says why behind an ⓘ`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, context } = await signedIn(browser, width, height, touch);
    t.after(() => context.close());
    await page.goto(`${served.origin}/profile/tokens/new`);
    await assertMore(page, {
      where: `token form at ${width}`,
      name: 'More about Expires',
      visible: 'Required, and at most a year away.',
      more: 'There is no default on purpose: a short one gets worked around, a long one is the forever-credential this rule exists to prevent.',
      isPhone,
    });
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} × ${height} — the list of one's tokens keeps its first sentence and says the rest behind an ⓘ beside its title, or at the sentence's end on a phone`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, context } = await signedIn(browser, width, height, touch);
    t.after(() => context.close());
    await page.goto(`${served.origin}/profile/tokens`);
    await assertPlace(page, {
      where: `tokens at ${width}`,
      name: 'More about Access tokens',
      visible: 'A token lets a script or a tool act as you, bounded by what you may do and by the scopes you give it.',
      more: 'Nobody else can see this list, not even an administrator.',
      isPhone,
    });
    await assertMore(page, {
      where: `tokens at ${width}`,
      name: 'More about Access tokens',
      visible: 'A token lets a script or a tool act as you, bounded by what you may do and by the scopes you give it.',
      more: 'Nobody else can see this list, not even an administrator.',
      isPhone,
    });
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} × ${height} — the AI settings keep their first sentence and say the rest behind an ⓘ beside the title`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, context } = await signedIn(browser, width, height, touch);
    t.after(() => context.close());
    await page.goto(`${served.origin}/administration/ai`);
    await assertMore(page, {
      where: `AI settings at ${width}`,
      name: 'More about AI',
      visible: 'Which provider this workspace asks, under which models, and whether its content may be sent there at all.',
      more: 'Choosing a provider and consenting to use it are two decisions, and both are made here.',
      isPhone,
    });
    assert.deepEqual(failures, []);
  });

  test(`chromium: ${width} × ${height} — the create-hub page keeps Private's description whole and visible, with no ⓘ`, async (t) => {
    const browser = await sharedChromium(t);
    const { page, failures, context } = await signedIn(browser, width, height, touch);
    t.after(() => context.close());
    await page.goto(`${served.origin}/hubs/new`);
    const description = 'Only the people you give a role in it can open it. The workspace\'s owners and administrators cannot, unless you give them one.';
    const drawn = page.getByText(description, { exact: true }).filter({ visible: true });
    await drawn.first().waitFor({ timeout: 15_000 });
    const isWhole = await drawn.first().evaluate((node) => node.scrollWidth <= node.clientWidth && node.scrollHeight <= node.clientHeight + 1 && getComputedStyle(node).textOverflow !== 'ellipsis');
    assert.ok(isWhole, `create hub at ${width}: Private's description is cut`);
    assert.equal(await page.getByRole('button', { name: /^More about Private/ }).count(), 0, `create hub at ${width}: Private's description moved behind an ⓘ`);
    assert.deepEqual(failures, []);
  });
}
