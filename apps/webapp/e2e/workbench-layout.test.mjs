// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A reason never moves the page (UC-WRK-32 checks 1 and 6, design-system.md §4), measured in the
// three engines rather than argued.
//
// What runs is the design system's workbench as `make workbench-build` leaves it in
// packages/design-system/dist/workbench, served by serve.mjs under the product's own
// `style-src 'self'` - a component that writes an inline style fails here as it would in
// production. The stories are the ones every component with a reason carries: *in a row of
// buttons* and *in a table row*, each drawing the row twice, with the reason and without it
// (src/_InContext.svelte). For each, over the axes that move a layout - direction, +40 % text,
// 200 % zoom, the compact and the large width - the test records the box of every element on the
// stage but the bubble, then hovers, clicks, taps, focuses and presses the unavailable control,
// and after each asserts that no box moved, that the bubble says the reason, and that nothing the
// caller or the row listens for was reached.
//
// The pointer is the mouse and the touchscreen at the control's box, never `locator.click()`:
// Playwright refuses to click an `aria-disabled` target, which is the very target under test.
//
// Chromium runs the whole axis matrix; Firefox and WebKit run two corners of it (the plain case,
// and right-to-left with long text at 200 %), which is where an engine difference in layout shows
// first, at a fraction of the time. The second corner is on the compact width: at 200 % the large
// width overflows the workbench's right-to-left pane sideways, and Firefox places an anchored
// element inside a sideways-scrolled right-to-left container a scroll width away from its anchor -
// the workbench's zoom axis, which scales a pane inside a scroller, is the only place that builds
// one; the product zooms the page instead.

import { describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium, firefox, webkit } from 'playwright';

import { serve } from './serve.mjs';

const WORKBENCH = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'packages', 'design-system', 'dist', 'workbench');

const MATRIX = [];
for (const dir of ['ltr', 'rtl'])
  for (const text of ['normal', 'long'])
    for (const zoom of ['100', '200'])
      for (const width of ['compact', 'large']) MATRIX.push({ dir, text, zoom, width });
const CORNERS = [
  { dir: 'ltr', text: 'normal', zoom: '100', width: 'compact' },
  { dir: 'rtl', text: 'long', zoom: '200', width: 'compact' },
];

/**
 * The stories, by workbench id. `reason` stories carry an unavailable control in a row drawn with
 * and without its reason; `busy` stories carry a driver that switches their subject busy and back.
 */
const STORIES = [
  { id: 'wave-1-forms-button--in-row', kind: 'reason' },
  { id: 'wave-1-forms-button--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-button--busy-in-place', kind: 'busy' },
  { id: 'wave-1-forms-icon-button--in-row', kind: 'reason' },
  { id: 'wave-1-forms-icon-button--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-icon-button--busy', kind: 'busy' },
  { id: 'wave-1-forms-input--in-row', kind: 'reason' },
  { id: 'wave-1-forms-input--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-input--range', kind: 'range' },
  { id: 'wave-1-forms-textarea--in-row', kind: 'reason' },
  { id: 'wave-1-forms-textarea--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-select--in-row', kind: 'reason' },
  { id: 'wave-1-forms-select--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-select--option', kind: 'shape' },
  { id: 'wave-1-forms-checkbox--in-row', kind: 'reason' },
  { id: 'wave-1-forms-checkbox--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-checkbox--busy', kind: 'busy' },
  // A refused option of MultiSelect's ticks; in the field it is a line inside the open list.
  { id: 'wave-1-forms-multi-select--in-row', kind: 'reason' },
  { id: 'wave-1-forms-multi-select--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-switch--in-row', kind: 'reason' },
  { id: 'wave-1-forms-switch--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-switch--busy', kind: 'busy' },
  { id: 'wave-3-domain-task-row--in-table-row', kind: 'reason' },
  { id: 'wave-3-time-due-date-control--in-row', kind: 'group' },
  { id: 'wave-3-time-due-date-control--in-table-row', kind: 'group' },
  { id: 'wave-3-time-recurrence-editor--in-row', kind: 'group' },
  { id: 'wave-3-time-recurrence-editor--in-table-row', kind: 'group' },
  { id: 'wave-3-time-reminder-editor--in-row', kind: 'group' },
  { id: 'wave-3-time-reminder-editor--in-table-row', kind: 'group' },
  { id: 'wave-3-domain-custom-field-renderer--in-row', kind: 'group' },
  { id: 'wave-3-domain-custom-field-renderer--in-table-row', kind: 'group' },
  { id: 'wave-3-domain-upload-field--in-row', kind: 'group' },
  { id: 'wave-3-domain-upload-field--in-table-row', kind: 'group' },
  { id: 'wave-3-domain-assignee-control--in-row', kind: 'group' },
  { id: 'wave-3-domain-assignee-control--in-table-row', kind: 'group' },
  { id: 'wave-3-domain-capability-gate--in-row', kind: 'gate' },
  { id: 'wave-3-domain-capability-gate--in-table-row', kind: 'gate' },
  { id: 'wave-3-domain-capability-gate--deciding-permitted', kind: 'permitted' },
  { id: 'wave-3-domain-capability-gate--deciding-refused', kind: 'refused' },
  { id: 'wave-2-structure-save-bar--form', kind: 'savebar' },
  { id: 'wave-2-structure-tabs--in-row', kind: 'roving' },
  { id: 'wave-2-structure-tabs--in-table-row', kind: 'roving' },
  { id: 'wave-3-hubtask-view-switcher--in-row', kind: 'roving' },
  { id: 'wave-3-hubtask-view-switcher--in-table-row', kind: 'roving' },
  // A gallery that is a choice: a grid of tiles in families, one of them refused with its reason.
  { id: 'wave-1-forms-choice-cards--refused-tile', kind: 'tiles' },
  // CheckboxGroup's boxes, drawn by MultiSelect's ticks, where its own row drew them before.
  { id: 'wave-1-forms-checkbox-group--resting', kind: 'ticks' },
  { id: 'wave-1-forms-checkbox-group--missing', kind: 'ticks' },
];

/**
 * The runs, side by side: Chromium's matrix in four quarters by direction and zoom, each with its
 * own server, browser and page - in one page the matrix alone took most of the CI job's time, most
 * of it the hover grace each case waits out - and the two corners in Firefox and in WebKit.
 */
const quarter = (dir, zoom) => MATRIX.filter((axes) => axes.dir === dir && axes.zoom === zoom);
const ENGINES = [
  ['chromium', chromium, quarter('ltr', '100'), 'ltr, 100 %'],
  ['chromium', chromium, quarter('ltr', '200'), 'ltr, 200 %'],
  ['chromium', chromium, quarter('rtl', '100'), 'rtl, 100 %'],
  ['chromium', chromium, quarter('rtl', '200'), 'rtl, 200 %'],
  ['firefox', firefox, CORNERS, 'two corners'],
  ['webkit', webkit, CORNERS, 'two corners'],
];

const query = (story, axes) => `?story=${story}&dir=${axes.dir}&text=${axes.text}&zoom=${axes.zoom}&width=${axes.width}`;
const label = (axes) => `${axes.dir}/${axes.text}/${axes.zoom}/${axes.width}`;

/**
 * Every element on the stage but the bubble, by its place in the tree, with its box - relative to
 * the stage, so that focus scrolling a wide pane is not read as a box that moved.
 */
async function boxes(page, selector = '.host *') {
  return page.evaluate((selector) => {
    const out = [];
    const origin = document.querySelector('.host').getBoundingClientRect();
    for (const element of document.querySelectorAll(selector)) {
      // The bubble is out of the flow by design; an option of a native list has no box of its own.
      if (element.closest('[role="tooltip"], option, optgroup')) continue;
      // A visually hidden name is clipped to nothing and positioned against the page, not the
      // pane: when focus scrolls a wide pane it stays where it was, which is no layout change.
      if (getComputedStyle(element).clipPath === 'inset(50%)') continue;
      // A turning mark is a pending state's picture: its box turns with it, which is no layout.
      if (getComputedStyle(element).animationName !== 'none' && element.closest('svg')) continue;
      const box = element.getBoundingClientRect();
      out.push([element.tagName, box.x - origin.x, box.y - origin.y, box.width, box.height]);
    }
    return out;
  }, selector);
}

function assertSame(before, after, what) {
  assert.equal(after.length, before.length, `${what}: the stage gained or lost elements`);
  for (let index = 0; index < before.length; index += 1) {
    const [tag, ...was] = before[index];
    const [, ...is] = after[index];
    for (let side = 0; side < 4; side += 1) {
      assert.ok(Math.abs(was[side] - is[side]) < 0.5, `${what}: ${tag} #${index} moved from ${was} to ${is}`);
    }
  }
}

/** The boxes of one case's row, relative to the row: what "the reason takes no space" compares. */
async function rowShape(page, which, selector = '*') {
  return page.evaluate(([which, selector]) => {
    const row = document.querySelector(`.host [data-case="${which}"]`);
    const origin = row.getBoundingClientRect();
    const out = [[origin.width, origin.height]];
    for (const element of row.querySelectorAll(selector)) {
      // The bubble is out of the flow by design; an option of a native list has no box of its own.
      if (element.closest('[role="tooltip"], option, optgroup')) continue;
      const box = element.getBoundingClientRect();
      out.push([element.tagName, box.x - origin.x, box.y - origin.y, box.width, box.height]);
    }
    return out;
  }, [which, selector]);
}

async function counters(page) {
  return page.evaluate(() => {
    const root = document.querySelector('.host .contexts');
    return { caller: root?.dataset.callerClicks, row: root?.dataset.rowClicks };
  });
}

/** The unavailable control of the row drawn with its reason, and its bubble. */
async function subject(page) {
  // The first one a keyboard reaches by Tab, where there is one: an upload's native input is out
  // of the tab order on purpose, and its button is the way in. A roving item has none.
  const all = page.locator('.host [data-case="with"] [aria-disabled="true"]');
  const tabbable = page.locator('.host [data-case="with"] [aria-disabled="true"]:not([tabindex="-1"])');
  const control = (await tabbable.count()) > 0 ? tabbable.first() : all.first();
  const described = (await control.getAttribute('aria-describedby')) ?? '';
  const bubbleId = described.split(' ').find((id) => id.includes('reason'));
  assert.ok(bubbleId, 'the unavailable control points at no reason');
  // Only the reason describes an unavailable control; a field keeps its hint and its error too.
  const isField = await control.evaluate((node) => ['INPUT', 'SELECT', 'TEXTAREA'].includes(node.tagName));
  if (!isField) assert.equal(described, bubbleId, `the unavailable control is described by more than its reason: ${described}`);
  return { control, bubble: page.locator(`[id="${bubbleId}"]`), described };
}

const centre = async (locator) => {
  const box = await locator.boundingBox();
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
};

async function isShowing(bubble) {
  return bubble.evaluate((node) => !node.hidden && node.checkVisibility());
}

/** Waits past the hover grace (the `attach` role's duration, read the way the part reads it). */
async function pastGrace(page) {
  const ms = await page.evaluate(() => {
    const value = getComputedStyle(document.querySelector('.host')).getPropertyValue('--motion-attach-duration').trim();
    const amount = Number.parseFloat(value);
    return value.endsWith('ms') ? amount : amount * 1000;
  });
  await page.waitForTimeout(ms + 150);
}

/** Whether the bubble is put away within a second past the grace: a wait that ends when it is. */
async function goesAway(page, bubble) {
  return page.waitForFunction((node) => node.hidden || !node.checkVisibility(), await bubble.elementHandle(), { timeout: 2_000 })
    .then(() => true, () => false);
}

/**
 * Leaves the control backwards and comes back to it forwards, as a keyboard reader would: the
 * control is a stop in the tab order. Forward until it is reached - a date field's own parts and
 * the workbench's chrome may lie between, and what is proven is that it is a stop, not how many
 * presses away. With the key that engine's users press: WebKit on macOS keeps buttons out of `Tab`
 * by default, as Safari does, and its users reach them with Option+Tab; WebKit on Linux, which CI
 * runs, takes buttons on plain `Tab` and moves no focus on Alt+Tab.
 */
async function reachByTab(page, control, engine) {
  const isMacWebKit = engine === 'webkit' && (await page.evaluate(() => navigator.platform.startsWith('Mac')));
  const [back, forward] = isMacWebKit ? ['Alt+Shift+Tab', 'Alt+Tab'] : ['Shift+Tab', 'Tab'];
  await control.evaluate((node) => node.focus());
  await page.keyboard.press(back);
  for (let step = 0; step < 40; step += 1) {
    if (await control.evaluate((node) => node === document.activeElement)) return;
    await page.keyboard.press(forward);
  }
}

async function checkReason(page, what, engine, { isRoving = false, isGroup = false, shape = '*' } = {}) {
  const { control, bubble, described } = await subject(page);
  // A composite's bubble comes from its mark: the pointer goes there, where the bubble is drawn.
  const target = isGroup ? page.locator('.host [data-case="with"] [data-reason-mark]').first() : control;
  // A pane at 200 % is wider than the window: the pointer can only reach what is in view.
  await target.evaluate((node) => node.scrollIntoView({ block: 'center', inline: 'center' }));
  const before = await boxes(page);
  const reason = (await bubble.textContent())?.trim();
  assert.ok(reason, `${what}: the bubble is empty`);
  assert.equal(await isShowing(bubble), false, `${what}: the bubble shows before anything happened`);
  const at = await centre(target);

  // Hover: the bubble shows, and stays while the pointer crosses onto it (SC 1.4.13, hoverable).
  await page.mouse.move(at.x, at.y);
  assert.equal(await isShowing(bubble), true, `${what}: hover shows no reason`);
  assertSame(before, await boxes(page), `${what}, hovered`);
  const onto = await centre(bubble);
  // One move, straight onto it: stepped moves on a loaded machine can spend the whole grace in
  // the gap between control and bubble, and what is proven here is that the bubble keeps.
  await page.mouse.move(onto.x, onto.y);
  await pastGrace(page);
  assert.equal(await isShowing(bubble), true, `${what}: the bubble went away under the pointer`);
  await page.mouse.move(1, 1);
  assert.equal(await goesAway(page, bubble), true, `${what}: the bubble stayed after the pointer left`);

  // A click: refused before the caller and the row, and it shows the reason.
  await page.mouse.click(at.x, at.y);
  assert.equal(await isShowing(bubble), true, `${what}: a click shows no reason`);
  assertSame(before, await boxes(page), `${what}, clicked`);
  assert.deepEqual(await counters(page), { caller: '0', row: '0' }, `${what}: a click reached a handler`);

  // The keyboard: the control is in the tab order, focus shows the reason, Enter and Space do
  // nothing, Escape hides it.
  // An item of a roving group is reached by the arrows (checkRoving), not by Tab.
  if (isRoving) await control.evaluate((node) => node.focus());
  else await reachByTab(page, control, engine);
  assert.equal(await control.evaluate((node) => node === document.activeElement), true, `${what}: Tab does not reach the control`);
  assert.equal(await isShowing(bubble), true, `${what}: focus shows no reason`);
  assert.equal(await control.getAttribute('aria-disabled'), 'true');
  assert.ok(described.split(' ').length >= 1);
  const isText = await control.evaluate((node) => node.tagName === 'TEXTAREA' || (node.tagName === 'INPUT' && !['range', 'color', 'file', 'checkbox', 'radio'].includes(node.type)));
  if (isText) {
    const value = await control.inputValue();
    await page.keyboard.type('xy');
    await page.keyboard.press('Backspace');
    assert.equal(await control.inputValue(), value, `${what}: typing changed an unavailable field`);
  }
  const checked = () => control.evaluate((node) => (node.type === 'checkbox' ? node.checked : null));
  const wasChecked = await checked();
  await page.keyboard.press('Enter');
  await page.keyboard.press('Space');
  assert.deepEqual(await counters(page), { caller: '0', row: '0' }, `${what}: a key reached a handler`);
  assert.equal(await checked(), wasChecked, `${what}: a key switched an unavailable box`);
  assertSame(before, await boxes(page), `${what}, focused`);
  await page.keyboard.press('Escape');
  assert.equal(await isShowing(bubble), false, `${what}: Escape leaves the reason`);

  // A tap: the same as a click, and a tap elsewhere puts it away.
  await control.evaluate((node) => node.blur());
  // The mouse leaves first: a finger and a mouse on one control at once is no device's case, and
  // the mouse's leaving would put the bubble away during the tap.
  await page.mouse.move(1, 1);
  await pastGrace(page);
  // Where it is now: the walk by keyboard may have scrolled the page through the workbench's chrome.
  await target.evaluate((node) => node.scrollIntoView({ block: 'center', inline: 'center' }));
  const tapAt = await centre(target);
  await page.touchscreen.tap(tapAt.x, tapAt.y);
  assert.equal(await isShowing(bubble), true, `${what}: a tap shows no reason`);
  assertSame(before, await boxes(page), `${what}, tapped`);
  assert.deepEqual(await counters(page), { caller: '0', row: '0' }, `${what}: a tap reached a handler`);
  await page.touchscreen.tap(2, 2);
  assert.equal(await isShowing(bubble), false, `${what}: a tap elsewhere leaves the reason`);

  // The reason takes no space: the row with it and the row without it have the same shape.
  const [withReason, without] = [await rowShape(page, 'with', shape), await rowShape(page, 'without', shape)];
  assertSame(without.slice(1), withReason.slice(1), `${what}, the row with the reason against the row without`);
  assert.ok(Math.abs(withReason[0][1] - without[0][1]) < 0.5, `${what}: the row with the reason is ${withReason[0][1]} high, without ${without[0][1]}`);
}

async function checkBusy(page, what) {
  const measure = () => boxes(page, '.host button, .host input, .host label, .host .row, .host .contexts');
  const before = await measure();
  await page.locator('.host [data-driver]').click();
  await page.waitForSelector('.host [aria-busy="true"]');
  assertSame(before, await measure(), `${what}, busy`);
  await page.locator('.host [data-driver]').click();
  await page.waitForSelector('.host [aria-busy="true"]', { state: 'detached' });
  assertSame(before, await measure(), `${what}, done`);
}

/**
 * A tab strip or a radio group with its middle item unavailable: the arrows land on it without
 * choosing it, show its reason and move nothing; the next press moves on and chooses. "Next" is
 * the arrow the writing direction says it is.
 */
async function checkRoving(page, what, engine) {
  await checkReason(page, what, engine, { isRoving: true });
  const items = page.locator('.host [data-case="with"] :is([role="tab"], [role="radio"])');
  const chosen = (item) => item.evaluate((node) => (node.getAttribute('aria-selected') ?? node.getAttribute('aria-checked')) === 'true');
  const isRtl = await items.nth(0).evaluate((node) => getComputedStyle(node).direction === 'rtl');
  const next = isRtl ? 'ArrowLeft' : 'ArrowRight';
  assert.equal(await chosen(items.nth(0)), true, `${what}: the first item is not the chosen one`);
  await items.nth(0).evaluate((node) => node.focus());
  const before = await boxes(page);

  await page.keyboard.press(next);
  assert.equal(await items.nth(1).evaluate((node) => node === document.activeElement), true, `${what}: ${next} does not reach the unavailable item`);
  assert.equal(await chosen(items.nth(1)), false, `${what}: the unavailable item was chosen`);
  assert.equal(await chosen(items.nth(0)), true, `${what}: the choice moved onto nothing`);
  const bubbleId = (await items.nth(1).getAttribute('aria-describedby')) ?? '';
  assert.equal(await isShowing(page.locator(`[id="${bubbleId}"]`)), true, `${what}: focus by arrow shows no reason`);
  assertSame(before, await boxes(page), `${what}, arrowed onto`);

  await page.keyboard.press(next);
  assert.equal(await items.nth(2).evaluate((node) => node === document.activeElement), true, `${what}: ${next} does not move on`);
  assert.equal(await chosen(items.nth(2)), true, `${what}: the next available item was not chosen`);
}

/** A range ignores `readonly`: the guard has to keep its arrows from moving it. */
async function checkRange(page, what, engine) {
  await checkReason(page, what, engine);
  const { control } = await subject(page);
  const before = await control.inputValue();
  await control.evaluate((node) => node.focus());
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('End');
  assert.equal(await control.inputValue(), before, `${what}: the keys moved an unavailable range`);
}

/**
 * Nothing unavailable to try, only a shape to keep: an option's reason does not widen the closed
 * select, and the open list says it.
 */
async function checkShape(page, what) {
  const [withReason, without] = [await rowShape(page, 'with'), await rowShape(page, 'without')];
  assertSame(without.slice(1), withReason.slice(1), `${what}, the row with an option's reason against the row without`);
  // The same option says more where it is unavailable than where it is not: its reason. Compared
  // with its twin rather than with the reason's words, which the +40 % axis rewrites.
  const option = page.locator('.host [data-case="with"] option[disabled]').first();
  const value = await option.getAttribute('value');
  const twin = page.locator(`.host [data-case="without"] option[value="${value}"]`);
  const [said, plain] = [(await option.textContent()).trim(), (await twin.textContent()).trim()];
  assert.ok(said.length > plain.length, `${what}: the unavailable option says "${said}", not why`);
}

/**
 * A composite says its reason once: one bubble for all of its parts, none in the row without the
 * reason, every unavailable part pointing at that one.
 */
async function checkGroup(page, what, engine) {
  await checkReason(page, what, engine, { isGroup: true });
  const reasons = await page.evaluate(() => ({
    with: document.querySelectorAll('.host [data-case="with"] [role="tooltip"]').length,
    without: document.querySelectorAll('.host [data-case="without"] [role="tooltip"]').length,
    described: [...document.querySelectorAll('.host [data-case="with"] [aria-disabled="true"]')].map(
      (node) => (node.getAttribute('aria-describedby') ?? '').split(' ').filter((id) => id.includes('reason')),
    ),
  }));
  assert.equal(reasons.with, 1, `${what}: the composite says its reason ${reasons.with} times`);
  assert.equal(reasons.without, 0, `${what}: the available composite carries a reason`);
  const ids = new Set(reasons.described.flat());
  assert.equal(ids.size, 1, `${what}: its parts point at ${ids.size} reasons`);
}

/**
 * A control in a line behind a refused gate: the lock over it is what is tried, and the row keeps
 * its shape - compared by the controls and cells both rows have, the gate's own wrapper aside.
 */
async function checkGate(page, what, engine) {
  await checkReason(page, what, engine, { shape: 'button:not(.lock), td, tr' });
}

/**
 * A gate that has not been answered yet, then answered: the content keeps its boxes when the
 * answer is "permitted", and its size when the answer is "refused" (the reason arrives above it).
 */
async function checkAnswer(page, what, _engine, { isSizeOnly }) {
  const measure = async () =>
    (await boxes(page, '.host [data-case="with"] :is(select, label, .shell)')).map(([tag, x, y, w, h]) =>
      isSizeOnly ? [tag, 0, 0, w, h] : [tag, x, y, w, h],
    );
  const before = await measure();
  await page.locator('.host [data-driver]').click();
  // The answer renders the content anew; the +40 % axis rewrites new text a moment later.
  await page.waitForTimeout(400);
  assertSame(before, await measure(), `${what}, answered`);
}

/**
 * A form's save bar arrives with its first change and moves nothing above it; a field focused at
 * the end of the scroll area scrolls clear of it rather than under it.
 */
async function checkSaveBar(page, what) {
  const measure = () => boxes(page, '.host [data-scroll-area] :not([data-save-bar], [data-save-bar] *)');
  const before = await measure();
  await page.locator('.host [data-driver]').click();
  const bar = page.locator('.host [data-save-bar] [role="region"]');
  await bar.waitFor();
  assertSame(before, await measure(), `${what}, the bar arrived`);
  const status = await page.locator('.host [data-save-bar] [role="status"]').textContent();
  assert.ok(status && status.trim().length > 0, `${what}: the count is not announced`);

  const last = page.locator('.host [data-last]');
  await last.evaluate((node) => node.focus());
  await page.waitForTimeout(100);
  const [field, over] = [await last.boundingBox(), await bar.boundingBox()];
  assert.ok(field.y + field.height <= over.y + 0.5, `${what}: the focused last field ends at ${field.y + field.height}, under the bar from ${over.y}`);

  await last.evaluate((node) => node.blur());
  await page.locator('.host [data-driver]').click();
  await bar.waitFor({ state: 'detached' });

  // A field that already has focus when the first change brings the bar up - typed into at the
  // foot of the scroll area, as on a phone - is scrolled clear of the bar as it arrives.
  await page.locator('.host [data-scroll-area]').evaluate((area) => { area.scrollTop = area.scrollHeight; });
  await last.focus();
  await page.keyboard.type('x');
  await bar.waitFor();
  await page.waitForTimeout(100);
  const [typed, arrived] = [await last.boundingBox(), await bar.boundingBox()];
  assert.ok(typed.y + typed.height <= arrived.y + 0.5, `${what}: the field being typed into ends at ${typed.y + typed.height}, under the arriving bar from ${arrived.y}`);
  await page.keyboard.press('Backspace');
  await bar.waitFor({ state: 'detached' });
  await last.evaluate((node) => node.blur());
}

/**
 * ChoiceCards' tiles: the grid lays the tiles out side by side without overlap in every engine (it
 * sits on an element inside the fieldset, not on the fieldset), and the radio group spans the
 * families. A tile is chosen by a press; the next arrow moves across the families' border and
 * chooses; the refused tile is passed over and a press on it chooses nothing; Tab comes back in on
 * the chosen tile - and none of it moves a box.
 */
async function checkTiles(page, what, engine) {
  const tiles = page.locator('.host .card');
  const radios = page.locator('.host input[type="radio"]');
  const geometry = await tiles.evaluateAll((nodes) => nodes.map((node) => {
    const box = node.getBoundingClientRect();
    return { x: box.x, y: box.y, w: box.width, h: box.height, room: node.parentElement.getBoundingClientRect().width };
  }));
  // The gallery card's width as this pane draws it - the zoom axis scales the pane.
  const card = await page.evaluate(() => {
    const grid = document.querySelector('.host .card').parentElement;
    const scale = grid.getBoundingClientRect().width / grid.offsetWidth;
    return Number.parseFloat(getComputedStyle(grid).getPropertyValue('--layout-gallery-card')) * scale;
  });
  for (const [index, tile] of geometry.entries()) {
    assert.ok(tile.w >= Math.min(card, tile.room) - 0.5, `${what}: tile ${index} is ${tile.w} wide, under a gallery card in ${tile.room}`);
    for (const other of geometry.slice(index + 1)) {
      const apart = tile.x + tile.w <= other.x + 0.5 || other.x + other.w <= tile.x + 0.5 || tile.y + tile.h <= other.y + 0.5 || other.y + other.h <= tile.y + 0.5;
      assert.ok(apart, `${what}: tile ${index} overlaps another`);
    }
  }
  const names = await page.locator('.host fieldset fieldset legend').allTextContents();
  assert.ok(names.length >= 2, `${what}: the families are not fieldsets with legends`);
  assert.equal(await page.locator('.host [role="radiogroup"]').count(), 1, `${what}: the families are more than one radio group`);
  assert.equal(new Set(await radios.evaluateAll((nodes) => nodes.map((node) => node.name))).size, 1, `${what}: the radios do not share one name`);

  const isRtl = await radios.first().evaluate((node) => getComputedStyle(node).direction === 'rtl');
  const next = isRtl ? 'ArrowLeft' : 'ArrowRight';
  const chosen = () => page.locator('.host input[type="radio"]:checked').getAttribute('value');
  const focused = () => page.evaluate(() => document.activeElement?.getAttribute('value'));
  const lastOfFirst = page.locator('.host fieldset fieldset').first().locator('input[type="radio"]').last();
  const firstOfSecond = page.locator('.host fieldset fieldset').nth(1).locator('input[type="radio"]').first();
  const refused = page.locator('.host input[type="radio"]:disabled');
  assert.equal(await refused.count(), 1, `${what}: the story has no refused tile`);

  await page.mouse.move(1, 1);
  const before = await boxes(page);
  // A press anywhere on the tile chooses it.
  const tileOf = async (radio) => page.locator('.host .card').filter({ has: page.locator(`input[value="${await radio.getAttribute('value')}"]`) });
  await lastOfFirst.evaluate((node) => node.closest('.card').scrollIntoView({ block: 'center', inline: 'center' }));
  const at = await centre(await tileOf(lastOfFirst));
  await page.mouse.click(at.x, at.y);
  assert.equal(await chosen(), await lastOfFirst.getAttribute('value'), `${what}: a press on the tile did not choose it`);

  await page.keyboard.press(next);
  assert.equal(await focused(), await firstOfSecond.getAttribute('value'), `${what}: ${next} from the last tile of a family does not reach the next family`);
  assert.equal(await chosen(), await firstOfSecond.getAttribute('value'), `${what}: ${next} into the next family did not choose`);

  // Past the refused tile: the arrow skips it and chooses the one after.
  const order = await radios.evaluateAll((nodes) => nodes.map((node) => node.value));
  const refusedValue = await refused.getAttribute('value');
  const afterRefused = order[order.indexOf(refusedValue) + 1];
  await page.keyboard.press(next);
  assert.equal(await chosen(), afterRefused, `${what}: ${next} did not pass over the refused tile`);
  const reason = await refused.evaluate((node) => (node.getAttribute('aria-describedby') ?? '').split(' ').find((id) => id.endsWith('-reason')));
  assert.ok(reason && (await page.locator(`[id="${reason}"]`).isVisible()), `${what}: the refused tile does not say its reason`);

  // A press on the refused tile chooses nothing.
  const refusedAt = await centre(await tileOf(refused));
  await page.mouse.click(refusedAt.x, refusedAt.y);
  assert.equal(await chosen(), afterRefused, `${what}: a press on the refused tile chose it`);

  // Out of the group and back: Tab lands on the chosen tile, not on the first.
  await radios.first().evaluate((node) => node.focus());
  await page.keyboard.press(engine === 'webkit' && (await page.evaluate(() => navigator.platform.startsWith('Mac'))) ? 'Alt+Shift+Tab' : 'Shift+Tab');
  await page.keyboard.press(engine === 'webkit' && (await page.evaluate(() => navigator.platform.startsWith('Mac'))) ? 'Alt+Tab' : 'Tab');
  assert.equal(await focused(), afterRefused, `${what}: Tab does not land on the chosen tile`);

  await page.mouse.move(1, 1);
  assertSame(before, await boxes(page), `${what}, after choosing, arrowing and pressing the refused tile`);
}

/**
 * CheckboxGroup's boxes lie as its own row laid them out before they became MultiSelect's ticks:
 * a row that wraps, `--sp-200` between boxes on a line and `--sp-100` between lines and under the
 * hint, every line starting at the group's start, and a box on a new line only where it did not
 * fit on the one before. Measured against the tokens, so a changed arrangement fails at every axis.
 */
async function checkTicks(page, what) {
  const seen = await page.evaluate(() => {
    const group = document.querySelector('.host fieldset');
    const inputs = [...group.querySelectorAll('input[type="checkbox"]')];
    // The row is the nearest element holding every box; its children are the boxes' blocks.
    let row = inputs[0].parentElement;
    while (!inputs.every((input) => row.contains(input))) row = row.parentElement;
    const scale = group.getBoundingClientRect().width / group.offsetWidth;
    const style = getComputedStyle(group);
    const space = (name) => Number.parseFloat(style.getPropertyValue(name)) * scale;
    const isRtl = getComputedStyle(row).direction === 'rtl';
    const box = (node) => {
      const rect = node.getBoundingClientRect();
      // What the box's content needs - its label and its text from the box's start - not what a
      // stretched block is given.
      const ends = [...node.querySelectorAll('label')].map((label) => label.getBoundingClientRect());
      const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
      for (let text = walker.nextNode(); text; text = walker.nextNode()) {
        if (!text.textContent.trim()) continue;
        const range = document.createRange();
        range.selectNode(text);
        ends.push(range.getBoundingClientRect());
      }
      const needs = isRtl ? rect.right - Math.min(...ends.map((each) => each.left)) : Math.max(...ends.map((each) => each.right)) - rect.left;
      return { top: rect.top, bottom: rect.bottom, left: rect.left, right: rect.right, width: rect.width, needs };
    };
    return {
      isRtl,
      row: box(row),
      above: box(row.previousElementSibling),
      items: [...row.children].map(box),
      count: inputs.length,
      inline: space('--sp-200'),
      block: space('--sp-100'),
    };
  });
  const near = (a, b) => Math.abs(a - b) < 0.5;
  assert.equal(seen.items.length, seen.count, `${what}: the row holds more than the boxes`);
  assert.ok(near(seen.row.top - seen.above.bottom, seen.block), `${what}: the boxes start ${seen.row.top - seen.above.bottom} under the hint, not ${seen.block}`);
  const start = (item) => (seen.isRtl ? seen.row.right - item.right : item.left - seen.row.left);
  const end = (item) => (seen.isRtl ? seen.row.right - item.left : item.right - seen.row.left);
  const room = seen.row.right - seen.row.left;
  let line = [seen.items[0]];
  assert.ok(near(start(seen.items[0]), 0), `${what}: the first box does not start at the group's start`);
  for (const [index, item] of seen.items.entries()) {
    assert.ok(near(item.width, item.needs), `${what}: box ${index} is ${item.width} wide for content of ${item.needs}`);
    if (index === 0) continue;
    const last = line.at(-1);
    if (near(item.top, last.top)) {
      assert.ok(near(start(item) - end(last), seen.inline), `${what}: box ${index} lies ${start(item) - end(last)} after the one before, not ${seen.inline}`);
      line.push(item);
      continue;
    }
    assert.ok(end(last) + seen.inline + item.needs > room + 0.5, `${what}: box ${index} starts a new line though it fits on the one before`);
    assert.ok(near(start(item), 0), `${what}: the line of box ${index} does not start at the group's start`);
    const bottom = Math.max(...line.map((each) => each.bottom));
    assert.ok(near(item.top - bottom, seen.block), `${what}: the line of box ${index} lies ${item.top - bottom} under the one before, not ${seen.block}`);
    line = [item];
  }
}

const CHECKS = {
  ticks: checkTicks,
  tiles: checkTiles,
  savebar: checkSaveBar,
  gate: checkGate,
  permitted: (page, what, engine) => checkAnswer(page, what, engine, { isSizeOnly: false }),
  refused: (page, what, engine) => checkAnswer(page, what, engine, { isSizeOnly: true }),
  group: checkGroup, reason: checkReason, busy: checkBusy, roving: checkRoving, range: checkRange, shape: checkShape };

describe('a reason never moves the page', { concurrency: true }, () => {
  for (const [name, engine, axes, part] of ENGINES.filter(([name]) => !process.env.ENGINES || process.env.ENGINES.split(',').includes(name))) {
    test(`a reason never moves the page, in ${name} (${part})`, { timeout: 15 * 60_000 }, async () => {
      assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
      const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
      // A browser that cannot start closes the server too: an open server keeps the run alive forever.
      const browser = await engine.launch().catch(async (error) => { await server.close(); throw error; });
      try {
        const context = await browser.newContext({ viewport: { width: 1800, height: 1100 }, hasTouch: true });
        const page = await context.newPage();
        await page.addInitScript(() => {
          window.__violations = [];
          document.addEventListener('securitypolicyviolation', (event) => {
            const inside = event.target instanceof Element && event.target.closest('.host') !== null;
            window.__violations.push({ inside, directive: event.violatedDirective, sample: event.sample });
          });
        });
        for (const story of STORIES.filter((each) => !process.env.STORIES || each.id.includes(process.env.STORIES))) {
          for (const each of axes) {
            // The +40 % axis rewrites the text once, as the story renders; content an answer renders
            // anew arrives in English and would be measured against its rewritten self.
            if (each.text === 'long' && (story.kind === 'permitted' || story.kind === 'refused')) continue;
            const what = `${story.id} ${label(each)} in ${name}`;
            await page.goto(`${server.origin}/${query(story.id, each)}`);
            const ready = { tiles: '.host [role="radiogroup"]', ticks: '.host fieldset' }[story.kind] ?? '.host :is(.contexts, [data-scroll-area])';
            await page.waitForSelector(ready);
            // A row arrives with the entrance role (motion.ts), a transform that ends: measured while
            // it runs, the arrival would read as a box the hover moved. A pending mark never ends.
            await page.evaluate(() => Promise.all(document.getAnimations()
              .filter((each) => each.effect?.getTiming().iterations !== Infinity)
              .map((each) => each.finished.catch(() => undefined))));
            await page.mouse.move(1, 1);
            await CHECKS[story.kind](page, what, name);
            const violations = await page.evaluate(() => window.__violations.filter((each) => each.inside));
            assert.deepEqual(violations, [], `${what}: a component broke style-src 'self'`);
          }
        }
        await context.close();
      } finally {
        await browser.close();
        await server.close();
      }
    });
  }
});
