// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No use case check: the design system's MultiSelect (design-system.md §4, wave 1; ADR-0110),
// walked in the built workbench as workbench-fields.test.mjs walks it, under the same
// `style-src 'self'`. It holds the keyboard of the closed field and of the open list, the option
// that is locked on and cannot be cleared, the refused option and the read-only control, the
// filter, focus never under the list's stuck filter and foot, the visible ticks, the bottom sheet
// at a phone's width with *Done*, and `Escape` closing one layer at a time inside a dialog - at
// both widths. The call sites' checks (UC-WRK-32,
// UC-WRK-23) are walked where a screen uses it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium, firefox, webkit } from 'playwright';

import { serve } from './serve.mjs';

const WORKBENCH = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'packages', 'design-system', 'dist', 'workbench');
const STORY = 'wave-1-forms-multi-select';
const ENGINES = { chromium, firefox, webkit };

/** What has focus: its role and its accessible name as the option's label says it. */
async function focused(page) {
  return page.evaluate(() => {
    const node = document.activeElement;
    const labelledBy = node?.getAttribute('aria-labelledby') ?? '';
    const name = labelledBy.split(' ').filter(Boolean).map((id) => document.getElementById(id)?.textContent?.trim() ?? '').join(' ');
    return { role: node?.getAttribute('role') ?? node?.tagName.toLowerCase(), name, isField: node?.getAttribute('aria-haspopup') === 'listbox' };
  });
}

/** The ids an element's description names, resolved to their text. */
async function description(locator) {
  return locator.evaluate((node) =>
    (node.getAttribute('aria-describedby') ?? '')
      .split(' ')
      .filter(Boolean)
      .map((id) => document.getElementById(id)?.textContent ?? '')
      .join(' '),
  );
}

const field = (page) => page.locator('[aria-haspopup="listbox"]').first();
const list = (page) => page.getByRole('listbox');
const option = (page, name) => list(page).getByRole('option', { name, exact: true });
const selectedNames = (page) =>
  list(page).locator('[role="option"][aria-selected="true"]').evaluateAll((nodes) =>
    nodes.map((node) => document.getElementById(node.getAttribute('aria-labelledby'))?.textContent?.trim()),
  );

async function story(page, origin, name) {
  await page.goto(`${origin}/?story=${STORY}--${name}`);
  await page.locator('.host').waitFor();
}

/**
 * Where the focused option is covered (design-system.md §10, 2.4.7 and 2.4.11): its box and its
 * ring against every stuck region of the open list - the filter and the foot - and the corners
 * of its top and bottom edges against what the page draws there, which also finds a stuck region's cover.
 */
async function focusCover(page) {
  return page.evaluate(() => {
    const node = document.activeElement;
    const style = getComputedStyle(node);
    const ring = Number.parseFloat(style.outlineOffset) + Number.parseFloat(style.outlineWidth);
    const box = node.getBoundingClientRect();
    const reach = { top: box.top - ring, bottom: box.bottom + ring };
    const covers = [];
    for (const stuck of node.closest('[role="dialog"], dialog').querySelectorAll('*')) {
      if (getComputedStyle(stuck).position !== 'sticky') continue;
      const over = stuck.getBoundingClientRect();
      if (reach.top < over.bottom - 0.5 && reach.bottom > over.top + 0.5) covers.push(`${stuck.className} ${Math.round(over.top)}-${Math.round(over.bottom)}`);
    }
    for (const [x, y] of [[box.left + box.width / 2, box.top + 1], [box.left + box.width / 2, box.bottom - 1]]) {
      const hit = document.elementFromPoint(x, y);
      if (!hit || !node.contains(hit)) covers.push(`${hit?.className ?? 'nothing'} at ${Math.round(x)},${Math.round(y)}`);
    }
    const name = document.getElementById(node.getAttribute('aria-labelledby'))?.textContent?.trim();
    return { name, at: `${Math.round(reach.top)}-${Math.round(reach.bottom)}`, covers };
  });
}

/** Arrows from the first option to the last and back; at each step the focused one is in clear view. */
/** A sheet rises as it opens: measured while it moves, everything in it reads as off the screen. */
async function settled(page) {
  await page.evaluate(() => Promise.all(document.getAnimations().map((each) => each.finished.catch(() => undefined))));
}

async function walkClear(page, what) {
  await settled(page);
  await page.keyboard.press('Home');
  const count = await list(page).getByRole('option').count();
  const hidden = [];
  for (const [key, steps] of [['ArrowDown', count - 1], ['ArrowUp', count - 1]]) {
    for (let step = 0; step <= steps; step += 1) {
      if (step > 0) await page.keyboard.press(key);
      const seen = await focusCover(page);
      if (seen.covers.length > 0) hidden.push(`${key} ${seen.name} (${seen.at}) under ${seen.covers.join(', ')}`);
    }
  }
  assert.deepEqual(hidden, [], `${what}: a focused option lies under a stuck region`);
}

/** Every line of a text, from its top to its bottom, is the text itself and not something drawn over it. */
async function linesCovered(page, locator) {
  await settled(page);
  return locator.evaluate((node) => {
    const range = document.createRange();
    range.selectNodeContents(node);
    const out = [];
    for (const line of range.getClientRects()) {
      for (const x of [line.left + 2, line.left + line.width / 2, line.right - 2]) {
        for (const y of [line.top + 2, line.top + line.height / 2, line.bottom - 2]) {
          const hit = document.elementFromPoint(x, y);
          if (!hit || !node.contains(hit)) out.push(`${hit?.className ?? 'nothing'} at ${Math.round(x)},${Math.round(y)}`);
        }
      }
    }
    return out;
  });
}

for (const engine of (process.env.ENGINES ?? 'chromium').split(',')) {
  test(`MultiSelect is operable by keyboard, keeps a locked option and becomes a sheet, in ${engine}`, { timeout: 5 * 60_000 }, async () => {
    assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
    const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
    const browser = await ENGINES[engine].launch().catch(async (error) => { await server.close(); throw error; });
    try {
      const page = await browser.newPage({ viewport: { width: 1280, height: 860 } });
      const select = process.platform === 'darwin' && engine === 'webkit' ? 'Meta+A' : 'Control+A';

      // --- the closed field and the keyboard of the list --------------------------------------
      await story(page, server.origin, 'some');
      const closed = field(page);
      assert.equal(await closed.evaluate((node) => node.tagName), 'BUTTON', 'the closed field is a button');
      assert.equal(await closed.getAttribute('aria-expanded'), 'false');

      for (const key of ['ArrowDown', 'Enter', 'Space']) {
        await closed.focus();
        await page.keyboard.press(key);
        await list(page).waitFor();
        assert.equal(await closed.getAttribute('aria-expanded'), 'true', `${key} did not open the list`);
        assert.equal(await list(page).getAttribute('aria-multiselectable'), 'true');
        assert.deepEqual(await focused(page), { role: 'option', name: 'Dates', isField: false }, `${key}: focus is not on the first chosen option`);
        await page.keyboard.press('Escape');
        await list(page).waitFor({ state: 'detached' });
        assert.equal((await focused(page)).isField, true, `${key}: Escape did not put focus back on the field`);
      }

      await closed.focus();
      await page.keyboard.press('ArrowDown');
      await list(page).waitFor();
      await page.keyboard.press('ArrowDown');
      assert.equal((await focused(page)).name, 'Reminders', 'ArrowDown does not move');
      await page.keyboard.press('ArrowUp');
      assert.equal((await focused(page)).name, 'Dates', 'ArrowUp does not move');
      await page.keyboard.press('End');
      assert.equal((await focused(page)).name, 'Amount', 'End is not the last');
      await page.keyboard.press('Home');
      assert.equal((await focused(page)).name, 'Done or open', 'Home is not the first');

      // Space toggles and the list stays open; the closed field says the new choice.
      await page.keyboard.press('ArrowDown');
      await page.keyboard.press('ArrowDown');
      await page.keyboard.press('Space');
      assert.equal(await option(page, 'Reminders').getAttribute('aria-selected'), 'true', 'Space did not tick');
      assert.equal(await list(page).isVisible(), true, 'the list closed on a toggle');
      assert.deepEqual(await selectedNames(page), ['Dates', 'Reminders', 'Notes', 'Comments']);
      await page.keyboard.press('Space');
      assert.equal(await option(page, 'Reminders').getAttribute('aria-selected'), 'false', 'Space did not clear');

      // Type-ahead: a letter moves to the next option starting with it, again to the one after.
      await page.keyboard.press('c');
      assert.equal((await focused(page)).name, 'Comments', 'a letter does not move to its option');
      await page.keyboard.press('c');
      assert.equal((await focused(page)).name, 'Cover', 'the same letter again does not move on');

      // Ctrl+A ticks every option.
      await page.keyboard.press(select);
      assert.equal((await selectedNames(page)).length, 15, `${select} did not tick all`);
      await page.keyboard.press('Escape');
      await list(page).waitFor({ state: 'detached' });
      const said = await closed.evaluate((node) => node.getAttribute('aria-labelledby').split(' ').map((id) => document.getElementById(id).textContent).join(' '));
      for (const name of ['Done or open', 'Amount', 'Board column']) assert.match(said, new RegExp(name), `the field does not say ${name}`);

      // --- all chosen: the chips are cut after two lines, a reader still hears all -----------
      await story(page, server.origin, 'all');
      const chips = page.locator('.host .chips:not(.measure) .chip');
      await chips.first().waitFor();
      const drawn = await chips.allTextContents();
      assert.ok(drawn.length < 15, 'every chip is drawn in two lines');
      assert.match(drawn.at(-1), /^\+\d+$/, `the last chip is not the count of the rest: ${drawn.at(-1)}`);
      assert.equal(drawn.length - 1 + Number(drawn.at(-1).slice(1)), 15, 'the chips and the count do not add up to fifteen');
      const rows = new Set(await chips.evaluateAll((nodes) => nodes.map((node) => Math.round(node.getBoundingClientRect().top))));
      assert.ok(rows.size <= 2, `the chips take ${rows.size} lines`);
      assert.equal(await page.locator('.host .chips:not(.measure) button').count(), 0, 'a chip is a button');
      // Right to left, the count of the rest still reads "+n": its sign is drawn before its digits.
      await page.goto(`${server.origin}/?story=${STORY}--all&dir=rtl`);
      const more = page.locator('.host .chips:not(.measure) .chip').last();
      await more.waitFor();
      const order = await more.evaluate((node) => {
        const text = node.firstChild;
        const at = (index) => { const range = document.createRange(); range.setStart(text, index); range.setEnd(text, index + 1); return range.getBoundingClientRect().left; };
        return { text: text.textContent, sign: at(0), digit: at(1) };
      });
      assert.ok(order.sign < order.digit, `right to left, the count of the rest reads with its sign after its digits: ${order.text}`);

      // --- a locked option: ticked, focusable, says why, and nothing clears it ----------------
      await story(page, server.origin, 'locked');
      assert.match(await page.locator('.host .chips:not(.measure) .chip').first().textContent(), /Done or open/, 'the locked option is not the first chip');
      await field(page).focus();
      await page.keyboard.press('Enter');
      await list(page).waitFor();
      const locked = option(page, 'Done or open');
      assert.deepEqual(await focused(page), { role: 'option', name: 'Done or open', isField: false }, 'focus is not on the locked option, the first chosen');
      assert.equal(await locked.getAttribute('aria-disabled'), 'true');
      assert.match(await description(locked), /Every entry can be done/, 'the locked option is not described by its reason');
      await page.keyboard.press('Space');
      await locked.click({ force: true }); // Playwright refuses an aria-disabled target, the one under test.
      assert.equal(await locked.getAttribute('aria-selected'), 'true', 'a press cleared the locked option');
      await page.getByRole('button', { name: 'None', exact: true }).click();
      assert.deepEqual(await selectedNames(page), ['Done or open'], 'None cleared the locked option, or left another');
      await locked.focus();
      await page.keyboard.press(select);
      assert.equal((await selectedNames(page)).length, 15, `${select} did not tick all`);
      await page.getByRole('button', { name: 'None', exact: true }).click();
      assert.deepEqual(await selectedNames(page), ['Done or open']);
      // A line under an option describes it; it is not part of its name.
      const dates = option(page, 'Dates');
      assert.equal(await dates.count(), 1, 'the option with a hint is not named by its label alone');
      assert.match(await description(dates), /A start and a due date/);
      await page.keyboard.press('Escape');

      // --- a refused option: reached by the arrows, says why, cannot be ticked ----------------
      await story(page, server.origin, 'refused');
      await field(page).focus();
      await page.keyboard.press('Enter');
      await list(page).waitFor();
      await page.keyboard.press('End');
      for (let step = 0; step < 15 && (await focused(page)).name !== 'Board column'; step += 1) await page.keyboard.press('ArrowUp');
      assert.equal((await focused(page)).name, 'Board column', 'the arrows do not reach the refused option');
      const refused = option(page, 'Board column');
      assert.equal(await refused.getAttribute('aria-disabled'), 'true');
      assert.match(await description(refused), /Offer the board first/);
      await page.keyboard.press('Space');
      await page.keyboard.press(select);
      assert.equal(await refused.getAttribute('aria-selected'), 'false', 'the refused option was ticked');
      await page.keyboard.press('Escape');

      // --- the whole control locked: it opens, read-only -------------------------------------
      await story(page, server.origin, 'read-only');
      assert.match(await description(field(page)), /Locked by the workspace/, 'the locked field is not described by its reason');
      await field(page).click();
      await list(page).waitFor();
      const before = await selectedNames(page);
      await page.keyboard.press('Space');
      await page.keyboard.press(select);
      await option(page, 'Reminders').click({ force: true });
      assert.deepEqual(await selectedNames(page), before, 'a read-only list changed');
      assert.equal(await page.getByRole('button', { name: 'All', exact: true }).count(), 0, 'a read-only list offers All');
      assert.match(await description(option(page, 'Reminders')), /Locked by the workspace/);
      assert.deepEqual(await linesCovered(page, page.getByRole('dialog').getByText(/^Locked by the workspace/)), [], 'the read-only reason is covered at 1280');
      await page.keyboard.press('Escape');

      // --- the filter: label and hint, groups left out, a sentence for no match ---------------
      // At a height where the list scrolls under its stuck filter and foot: the arrows never leave
      // focus under them.
      await page.setViewportSize({ width: 1280, height: 720 });
      await story(page, server.origin, 'grouped');
      await field(page).click();
      await list(page).waitFor();
      assert.equal(await list(page).getByRole('group').count(), 4, 'the four groups are not named groups');
      await walkClear(page, 'Grouped at 1280×720');
      // Tab walks the open list's own controls, and leaving them closes the list (APG combobox:
      // Tab closes the popup and moves on); focus goes on, not back to the field.
      await page.keyboard.press('Tab');
      assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'All', 'Tab from the list does not reach All');
      await page.keyboard.press('Tab');
      await page.keyboard.press('Tab');
      await list(page).waitFor({ state: 'detached', timeout: 5_000 }).catch(() => undefined);
      assert.equal(await field(page).getAttribute('aria-expanded'), 'false', 'the list stays open behind the focus');
      assert.equal((await focused(page)).isField, false, 'Tab out of the list put focus back on the field');
      await field(page).click();
      await list(page).waitFor();
      const filter = page.getByRole('searchbox', { name: 'Filter the fields' });
      // The button that empties the filter has a name of its own, not the filter's.
      await filter.fill('da');
      assert.equal(await page.getByRole('button', { name: 'Filter the fields', exact: true }).count(), 0, 'a button is named as the filter is');
      assert.equal(await page.getByRole('button', { name: 'Clear the filter', exact: true }).count(), 1, 'the filter has no button named Clear the filter');
      await filter.fill('person');
      assert.deepEqual(
        await list(page).getByRole('group').evaluateAll((nodes) => nodes.map((node) => document.getElementById(node.getAttribute('aria-labelledby')).textContent)),
        ['People'],
        'a group with no match is still drawn',
      );
      await filter.fill('a due date');
      assert.equal(await list(page).getByRole('option').count(), 1, 'the filter does not match a hint');
      await filter.fill('zzz');
      await page.getByText('No field matches this filter.').waitFor();
      assert.equal(await list(page).count(), 0, 'an empty listbox is left standing');
      // Nothing left to act on: the field controls no absent list, All and None are gone, and the
      // sentence is said where a reader hears it arrive.
      const controls = await field(page).getAttribute('aria-controls');
      assert.ok(controls === null || (await page.locator(`[id="${controls}"]`).count()) === 1, `the field controls ${controls}, which is not in the page`);
      for (const name of ['All', 'None']) assert.equal(await page.getByRole('button', { name, exact: true }).count(), 0, `${name} stays with nothing to act on`);
      assert.match(await page.getByRole('status').filter({ hasText: 'No field matches' }).textContent(), /No field matches this filter\./, 'the sentence is not in a live region');
      await filter.fill('');
      assert.equal(await page.getByRole('button', { name: 'All', exact: true }).count(), 1, 'All does not come back with the options');
      await page.keyboard.press('Escape');
      await page.setViewportSize({ width: 1280, height: 860 });

      // --- a question that needs an answer: a reader hears it, not only a drawn asterisk -------
      await story(page, server.origin, 'invalid');
      await page.getByRole('button', { name: /What a task carries \(required\)/ }).waitFor({ timeout: 5_000 }).catch(() => undefined);
      assert.equal(await page.getByRole('button', { name: /What a task carries \(required\)/ }).count(), 1, 'the field needing an answer is not named required');
      await field(page).click();
      await list(page).waitFor();
      assert.equal(await list(page).getAttribute('aria-required'), 'true', 'the list needing an answer is not aria-required');
      await page.keyboard.press('Escape');

      // --- the visible ticks: each one its own stop -------------------------------------------
      await story(page, server.origin, 'inline');
      const boxes = page.locator('.host').getByRole('checkbox');
      assert.equal(await boxes.count(), 4);
      assert.equal(await page.locator('.host [aria-haspopup="listbox"]').count(), 0, 'four options drew a field');
      assert.equal(await page.locator('.host').getByRole('group', { name: 'Views offered (required)' }).count(), 1, 'the ticks needing an answer are not named required');
      await boxes.first().focus();
      for (let index = 1; index < 4; index += 1) {
        await page.keyboard.press('Tab');
        assert.equal(await boxes.nth(index).evaluate((node) => node === document.activeElement), true, `Tab does not reach box ${index}`);
      }
      await page.keyboard.press('Space');
      assert.equal(await boxes.nth(3).isChecked(), true, 'Space does not tick a box');

      // --- inside a dialog: Escape closes the list, the dialog stays with its answers ---------
      await story(page, server.origin, 'in-dialog');
      await page.getByRole('button', { name: 'Edit the step' }).click();
      const dialog = page.getByRole('dialog', { name: 'Edit the step' });
      await dialog.waitFor();
      await dialog.getByRole('textbox', { name: 'Title' }).fill('Ship it');
      await dialog.locator('[aria-haspopup="listbox"]').click();
      await list(page).waitFor();
      await page.keyboard.press('Space');
      await page.keyboard.press('Escape');
      await list(page).waitFor({ state: 'detached' });
      assert.equal(await dialog.isVisible(), true, 'Escape closed the dialog with the list');
      assert.equal(await dialog.getByRole('textbox', { name: 'Title' }).inputValue(), 'Ship it', 'the dialog lost its answer');
      assert.equal((await focused(page)).isField, true, 'focus did not go back to the field');
      await page.keyboard.press('Escape');
      await dialog.waitFor({ state: 'hidden' });

      // --- a phone's width: the list is the bottom sheet, rows a thumb's height, Done ---------
      await page.setViewportSize({ width: 390, height: 844 });
      await story(page, server.origin, 'some');
      await field(page).click();
      const sheet = page.getByRole('dialog', { name: 'What a task carries' });
      await sheet.waitFor();
      assert.equal(await sheet.evaluate((node) => node.tagName), 'DIALOG', 'the list is not the sheet at 390');
      await list(page).waitFor();
      assert.equal((await focused(page)).name, 'Dates', 'the sheet does not focus the first chosen option');
      const target = await page.evaluate(() => Number.parseFloat(getComputedStyle(document.querySelector('.host')).getPropertyValue('--layout-target')));
      const height = await option(page, 'Reminders').evaluate((node) => node.getBoundingClientRect().height);
      assert.ok(height >= target - 0.5, `a row is ${height} high, below a thumb's ${target}`);
      await option(page, 'Reminders').click();
      assert.equal(await option(page, 'Reminders').getAttribute('aria-selected'), 'true');
      // Tab stays in the sheet, which is modal: onto its Done, and the sheet stays open.
      for (let step = 0; step < 6 && (await page.evaluate(() => document.activeElement?.textContent?.trim())) !== 'Done'; step += 1) await page.keyboard.press('Tab');
      assert.equal(await page.evaluate(() => document.activeElement?.textContent?.trim()), 'Done', 'Tab does not reach Done');
      assert.equal(await sheet.isVisible(), true, 'Tab onto Done closed the sheet');
      await sheet.getByRole('button', { name: 'Done' }).click();
      await sheet.waitFor({ state: 'hidden' });
      assert.equal((await focused(page)).isField, true, 'Done did not put focus back on the field');

      // The sheet scrolls under its stuck filter and foot too, and the read-only reason stays clear.
      await story(page, server.origin, 'grouped');
      await field(page).click();
      await list(page).waitFor();
      await walkClear(page, 'Grouped as the sheet at 390×844');
      await page.keyboard.press('Escape');
      await story(page, server.origin, 'read-only');
      await field(page).click();
      await list(page).waitFor();
      assert.deepEqual(await linesCovered(page, page.getByRole('dialog').getByText(/^Locked by the workspace/)), [], 'the read-only reason is covered at 390');
      await page.keyboard.press('Escape');
      await page.setViewportSize({ width: 390, height: 667 });
      await story(page, server.origin, 'german');
      await field(page).click();
      await list(page).waitFor();
      await walkClear(page, 'German as the sheet at 390×667');
      await page.keyboard.press('Escape');
      await page.setViewportSize({ width: 390, height: 844 });

      // The sheet over a full-screen dialog: Escape closes the sheet only, Done returns focus.
      await story(page, server.origin, 'in-dialog');
      await page.getByRole('button', { name: 'Edit the step' }).click();
      await dialog.waitFor();
      await dialog.locator('[aria-haspopup="listbox"]').click();
      const over = page.getByRole('dialog', { name: 'What a task carries' });
      await over.waitFor();
      await list(page).waitFor();
      await page.keyboard.press('Escape');
      await over.waitFor({ state: 'hidden' });
      assert.equal(await dialog.isVisible(), true, 'Escape on the sheet closed the dialog under it');
      await dialog.locator('[aria-haspopup="listbox"]').click();
      await over.waitFor();
      await over.getByRole('button', { name: 'Done' }).click();
      await over.waitFor({ state: 'hidden' });
      assert.equal(await dialog.isVisible(), true);
      assert.equal((await focused(page)).isField, true, 'Done over the dialog did not put focus back on the field');
    } finally {
      await browser.close();
      await server.close();
    }
  });
}
