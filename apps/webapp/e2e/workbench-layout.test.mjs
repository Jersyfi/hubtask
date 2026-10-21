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

import { test } from 'node:test';
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
  { id: 'wave-3-domain-comment-thread--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-input--in-row', kind: 'reason' },
  { id: 'wave-1-forms-input--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-input--range', kind: 'range' },
  { id: 'wave-1-forms-textarea--in-row', kind: 'reason' },
  { id: 'wave-1-forms-textarea--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-select--in-row', kind: 'reason' },
  { id: 'wave-1-forms-select--in-table-row', kind: 'reason' },
  { id: 'wave-1-forms-select--option', kind: 'shape' },
  { id: 'wave-2-structure-tabs--in-row', kind: 'roving' },
  { id: 'wave-2-structure-tabs--in-table-row', kind: 'roving' },
  { id: 'wave-3-hubtask-view-switcher--in-row', kind: 'roving' },
  { id: 'wave-3-hubtask-view-switcher--in-table-row', kind: 'roving' },
];

const ENGINES = [
  ['chromium', chromium, MATRIX],
  ['firefox', firefox, CORNERS],
  ['webkit', webkit, CORNERS],
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
async function rowShape(page, which) {
  return page.evaluate((which) => {
    const row = document.querySelector(`.host [data-case="${which}"]`);
    const origin = row.getBoundingClientRect();
    const out = [[origin.width, origin.height]];
    for (const element of row.querySelectorAll('*')) {
      // The bubble is out of the flow by design; an option of a native list has no box of its own.
      if (element.closest('[role="tooltip"], option, optgroup')) continue;
      const box = element.getBoundingClientRect();
      out.push([element.tagName, box.x - origin.x, box.y - origin.y, box.width, box.height]);
    }
    return out;
  }, which);
}

async function counters(page) {
  return page.evaluate(() => {
    const root = document.querySelector('.host .contexts');
    return { caller: root?.dataset.callerClicks, row: root?.dataset.rowClicks };
  });
}

/** The unavailable control of the row drawn with its reason, and its bubble. */
async function subject(page) {
  const control = page.locator('.host [data-case="with"] [aria-disabled="true"]').first();
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

async function checkReason(page, what, engine, { isRoving = false } = {}) {
  const [tab, back] = engine === 'webkit' ? ['Alt+Tab', 'Alt+Shift+Tab'] : ['Tab', 'Shift+Tab'];
  const { control, bubble, described } = await subject(page);
  // A pane at 200 % is wider than the window: the pointer can only reach what is in view.
  await control.evaluate((node) => node.scrollIntoView({ block: 'center', inline: 'center' }));
  const before = await boxes(page);
  const reason = (await bubble.textContent())?.trim();
  assert.ok(reason, `${what}: the bubble is empty`);
  assert.equal(await isShowing(bubble), false, `${what}: the bubble shows before anything happened`);
  const at = await centre(control);

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
  await pastGrace(page);
  assert.equal(await isShowing(bubble), false, `${what}: the bubble stayed after the pointer left`);

  // A click: refused before the caller and the row, and it shows the reason.
  await page.mouse.click(at.x, at.y);
  assert.equal(await isShowing(bubble), true, `${what}: a click shows no reason`);
  assertSame(before, await boxes(page), `${what}, clicked`);
  assert.deepEqual(await counters(page), { caller: '0', row: '0' }, `${what}: a click reached a handler`);

  // The keyboard: the control is in the tab order, focus shows the reason, Enter and Space do
  // nothing, Escape hides it.
  await control.evaluate((node) => node.focus());
  // An item of a roving group is reached by the arrows (checkRoving), not by Tab.
  if (!isRoving) await page.keyboard.press(back);
  if (!isRoving) await page.keyboard.press(tab);
  assert.equal(await control.evaluate((node) => node === document.activeElement), true, `${what}: Tab does not reach the control`);
  assert.equal(await isShowing(bubble), true, `${what}: focus shows no reason`);
  assert.equal(await control.getAttribute('aria-disabled'), 'true');
  assert.ok(described.split(' ').length >= 1);
  const isText = await control.evaluate((node) => node.tagName === 'TEXTAREA' || (node.tagName === 'INPUT' && !['range', 'color', 'file'].includes(node.type)));
  if (isText) {
    const value = await control.inputValue();
    await page.keyboard.type('xy');
    await page.keyboard.press('Backspace');
    assert.equal(await control.inputValue(), value, `${what}: typing changed an unavailable field`);
  }
  await page.keyboard.press('Enter');
  await page.keyboard.press('Space');
  assert.deepEqual(await counters(page), { caller: '0', row: '0' }, `${what}: a key reached a handler`);
  assertSame(before, await boxes(page), `${what}, focused`);
  await page.keyboard.press('Escape');
  assert.equal(await isShowing(bubble), false, `${what}: Escape leaves the reason`);

  // A tap: the same as a click, and a tap elsewhere puts it away.
  await control.evaluate((node) => node.blur());
  // The mouse leaves first: a finger and a mouse on one control at once is no device's case, and
  // the mouse's leaving would put the bubble away during the tap.
  await page.mouse.move(1, 1);
  await pastGrace(page);
  await page.touchscreen.tap(at.x, at.y);
  assert.equal(await isShowing(bubble), true, `${what}: a tap shows no reason`);
  assertSame(before, await boxes(page), `${what}, tapped`);
  assert.deepEqual(await counters(page), { caller: '0', row: '0' }, `${what}: a tap reached a handler`);
  await page.touchscreen.tap(2, 2);
  assert.equal(await isShowing(bubble), false, `${what}: a tap elsewhere leaves the reason`);

  // The reason takes no space: the row with it and the row without it have the same shape.
  const [withReason, without] = [await rowShape(page, 'with'), await rowShape(page, 'without')];
  assertSame(without.slice(1), withReason.slice(1), `${what}, the row with the reason against the row without`);
  assert.ok(Math.abs(withReason[0][1] - without[0][1]) < 0.5, `${what}: the row with the reason is ${withReason[0][1]} high, without ${without[0][1]}`);
}

async function checkBusy(page, what) {
  const measure = () => boxes(page, '.host button, .host .row, .host .contexts');
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

const CHECKS = { reason: checkReason, busy: checkBusy, roving: checkRoving, range: checkRange, shape: checkShape };

for (const [name, engine, axes] of ENGINES.filter(([name]) => !process.env.ENGINES || process.env.ENGINES.split(',').includes(name))) {
  test(`a reason never moves the page, in ${name}`, { timeout: 15 * 60_000 }, async () => {
    assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
    const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
    const browser = await engine.launch();
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
          const what = `${story.id} ${label(each)} in ${name}`;
          await page.goto(`${server.origin}/${query(story.id, each)}`);
          await page.waitForSelector('.host .contexts');
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
