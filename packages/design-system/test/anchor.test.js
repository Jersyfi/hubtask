// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// ADR-0039's positioner, tested where it can be without a layout engine: the table that turns a
// placement into `position-area`, and the top layer. Whether an engine then draws the overlay
// where the table says is the `engines` job's question (ADR-0048), asked of each engine in CI.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { boxWithin, placeWithTip, positionArea, raiseToTopLayer, tipSides } from '../src/anchor.ts';

test('a placement reads as the position-area the engine expects', () => {
  // `span-inline-end` runs from the anchor's inline start towards its end, which is what
  // `align: 'start'` means here. Getting this table backwards is a bug that only shows up as
  // "the menu is left-aligned in Chrome and right-aligned in Firefox".
  assert.equal(positionArea({ side: 'block-end', align: 'start' }), 'block-end span-inline-end');
  assert.equal(positionArea({ side: 'block-end', align: 'end' }), 'block-end span-inline-start');
  assert.equal(positionArea({ side: 'block-end', align: 'center' }), 'block-end');
  assert.equal(positionArea({ side: 'block-start', align: 'center' }), 'block-start');
  assert.equal(positionArea({ side: 'inline-end', align: 'start' }), 'inline-end span-block-end');
  assert.equal(positionArea({ side: 'inline-start', align: 'end' }), 'inline-start span-block-start');
});

// ---------------------------------------------------------------------------------------------
// The top layer. No DOM here: the function touches four methods of the element it is given, so a
// test can give it four methods.
// ---------------------------------------------------------------------------------------------

function fakeOverlay({ showPopover }) {
  const element = {
    attributes: {},
    shown: 0,
    hidden: 0,
    setAttribute(name, value) {
      this.attributes[name] = value;
    },
    removeAttribute(name) {
      delete this.attributes[name];
    },
    showPopover() {
      showPopover?.();
      this.shown += 1;
    },
    hidePopover() {
      this.hidden += 1;
    },
  };
  return element;
}

test('an overlay is raised into the top layer and lowered again', () => {
  const overlay = fakeOverlay({});
  const lower = raiseToTopLayer(overlay);
  assert.equal(overlay.attributes.popover, 'manual', 'manual, so light dismiss cannot take Escape');
  assert.equal(overlay.shown, 1);

  lower();
  assert.equal(overlay.hidden, 1);
  assert.equal(overlay.attributes.popover, undefined, 'the attribute goes off with it');
});

test('an overlay that cannot be raised is left visible rather than deleted', () => {
  // The branch that matters most: a `[popover]` which was never shown is `display: none`. Leaving
  // the attribute on after a failed `showPopover` would not misplace the overlay, it would remove
  // it from the screen entirely.
  const overlay = fakeOverlay({
    showPopover() {
      throw new Error('not connected');
    },
  });
  const lower = raiseToTopLayer(overlay);
  assert.equal(overlay.attributes.popover, undefined, 'the overlay is still rendered');
  lower();
  assert.equal(overlay.hidden, 0, 'and nothing is lowered that was never raised');
});

test('a browser with no top layer is left alone', () => {
  const overlay = { setAttribute: () => assert.fail('nothing may be set'), attributes: {} };
  const lower = raiseToTopLayer(overlay);
  assert.equal(typeof lower, 'function');
  lower();
});

// ---------------------------------------------------------------------------------------------
// A box with no element (a text selection, an editor's line) read inside its container.
// ---------------------------------------------------------------------------------------------

const frame = { top: 100, left: 50, width: 400, height: 300, borderTop: 1, borderLeft: 1, scrollTop: 0, scrollLeft: 0, clientWidth: 398 };

test('a selection box is placed inside its container, from its padding edge', () => {
  const placed = boxWithin({ top: 141, left: 81, width: 120, height: 20 }, frame);
  assert.deepEqual(placed, { blockStart: 40, inlineStart: 30, inlineSize: 120, blockSize: 20 });
});

test('a scrolled container moves the phantom with its content', () => {
  const placed = boxWithin({ top: 141, left: 81, width: 120, height: 20 }, { ...frame, scrollTop: 200 });
  assert.equal(placed.blockStart, 240, 'the box sits where the content is, not where the viewport shows it');
});

test('right to left, the inline start is measured from the right edge', () => {
  const placed = boxWithin({ top: 141, left: 81, width: 120, height: 20 }, frame, 'rtl');
  assert.equal(placed.inlineStart, 398 - 30 - 120);
});

// ---------------------------------------------------------------------------------------------
// A surface with a tip: which side, how far it is shifted, and where its tip stands.
// ---------------------------------------------------------------------------------------------

/** A reason bubble's measures: tip 6, the air before it 4, edge 16, near edge 8, radius 6. */
const BUBBLE = { gap: 10, edge: 16, nearEdge: 8, radius: 6, tip: 6, wideAim: 24, dir: 'ltr' };
/** A help tip's: tip 7, radius 12. */
const HELP = { ...BUBBLE, gap: 11, radius: 12, tip: 7 };
const edges = (left, top, width, height) => ({ left, top, right: left + width, bottom: top + height });

test('the sides a tip tries follow the placement and the writing direction', () => {
  assert.deepEqual(tipSides({ side: 'inline-end', align: 'start' }, 'ltr'), ['right', 'left', 'bottom', 'top']);
  assert.deepEqual(tipSides({ side: 'inline-end', align: 'start' }, 'rtl'), ['left', 'right', 'bottom', 'top']);
  assert.deepEqual(tipSides({ side: 'block-start', align: 'start' }, 'ltr'), ['top', 'bottom', 'right', 'left']);
  assert.deepEqual(tipSides({ side: 'block-start', align: 'start' }, 'rtl'), ['top', 'bottom', 'left', 'right']);
  assert.deepEqual(tipSides({ side: 'block-end', align: 'start' }, 'ltr'), ['bottom', 'top', 'right', 'left']);
  assert.deepEqual(tipSides({ side: 'block-end', align: 'start' }, 'rtl'), ['bottom', 'top', 'left', 'right']);
});

test('a field\'s reason with no room above or below stands at the line\'s end, then its start', () => {
  // A window as short as the field and its bubble together: neither above nor below holds it.
  const field = edges(300, 40, 200, 36);
  const sides = ['top', 'bottom', 'right', 'left'];
  const atEnd = placeWithTip({ ...BUBBLE, anchor: field, aim: field, width: 200, height: 40, viewport: { width: 1280, height: 110 }, sides });
  assert.equal(atEnd.side, 'right');
  assert.equal(atEnd.left, 500 + 10);
  assert.equal(atEnd.top, 40, 'beside it, lined up with its top');
  const wide = edges(900, 40, 200, 36);
  assert.equal(placeWithTip({ ...BUBBLE, anchor: wide, aim: wide, width: 200, height: 40, viewport: { width: 1280, height: 110 }, sides }).side, 'left', 'no room at the end either: the start');
});

test('a help tip with no room below or above stands at the line\'s end', () => {
  const mark = edges(100, 40, 24, 24);
  const placed = placeWithTip({ ...HELP, anchor: mark, aim: mark, width: 300, height: 80, viewport: { width: 1280, height: 110 }, sides: ['bottom', 'top', 'right', 'left'] });
  assert.equal(placed.side, 'right');
  assert.equal(placed.left, 124 + 11);
});

test('a surface lines up with the middle or the end of its anchor where its placement asks', () => {
  const button = edges(600, 300, 40, 40);
  const base = { ...BUBBLE, anchor: button, aim: button, width: 120, height: 30, viewport: { width: 1280, height: 800 }, sides: ['top', 'bottom'] };
  assert.equal(placeWithTip({ ...base, align: 'center' }).left, 620 - 60);
  assert.equal(placeWithTip({ ...base, align: 'end' }).left, 640 - 120);
  assert.equal(placeWithTip({ ...base, align: 'end', dir: 'rtl' }).left, 600, 'right to left, the end is the left edge');
  assert.equal(placeWithTip({ ...base, align: 'center' }).at, 60, 'the tip at its middle');
  const beside = placeWithTip({ ...base, sides: ['right'], align: 'center' });
  assert.equal(beside.top, 320 - 15);
});

test('a reason beside a menu at the right edge turns left, clear of the menu, its tip on the row', () => {
  const menu = edges(1000, 280, 240, 300);
  const row = edges(1005, 300, 230, 32);
  const placed = placeWithTip({
    ...BUBBLE, anchor: row, beside: menu, aim: row, width: 200, height: 40,
    viewport: { width: 1265, height: 800 }, sides: ['right', 'left', 'bottom', 'top'],
  });
  assert.equal(placed.side, 'left');
  assert.equal(placed.left, 1000 - 10 - 200, 'the gap is measured from the menu, not from the row inside it');
  assert.equal(placed.top, 300, 'lined up with the row\'s top');
  assert.equal(placed.at, 16, 'the tip at the row\'s middle');
});

test('with room on the right, the reason stays there', () => {
  const row = edges(400, 300, 230, 32);
  const placed = placeWithTip({ ...BUBBLE, anchor: row, aim: row, width: 200, height: 40, viewport: { width: 1265, height: 800 }, sides: ['right', 'left', 'bottom', 'top'] });
  assert.equal(placed.side, 'right');
  assert.equal(placed.left, 640);
});

test('at the foot of the window a reason lines up with the row\'s bottom, and comes nearer the edge so its tip reaches the row', () => {
  const row = edges(400, 760, 230, 32);
  const placed = placeWithTip({ ...BUBBLE, anchor: row, aim: row, width: 200, height: 60, viewport: { width: 1265, height: 800 }, sides: ['right', 'left'] });
  assert.equal(placed.side, 'right');
  // Bottom-aligned it would end at 792, past the edge of 16; shifted in it ends at 784, and the
  // row's middle (776) is then beyond the straight edge (top + 60 - 6 - 6 = 772). It moves down
  // again, no nearer than 8 to the edge, until the tip reaches the middle.
  assert.equal(placed.top, 728);
  assert.equal(placed.at, 48);
  assert.ok(placed.top + 60 <= 800 - BUBBLE.nearEdge);
});

test('where no side holds it, the side with the most room', () => {
  const row = edges(20, 20, 100, 30);
  const placed = placeWithTip({ ...BUBBLE, anchor: row, aim: row, width: 200, height: 400, viewport: { width: 300, height: 500 }, sides: ['top', 'bottom'] });
  assert.equal(placed.side, 'bottom');
});

test('a field\'s reason goes above, and below where there is no room above', () => {
  const field = edges(300, 200, 320, 36);
  const above = placeWithTip({ ...BUBBLE, anchor: field, aim: field, width: 200, height: 40, viewport: { width: 1280, height: 800 }, sides: ['top', 'bottom'] });
  assert.equal(above.side, 'top');
  assert.equal(above.top, 200 - 10 - 40);
  assert.equal(above.left, 300, 'lined up with the field\'s start');
  assert.equal(above.at, 24, 'into a field wider than the bubble, the tip points sp-300 past its start');
  const high = edges(300, 30, 320, 36);
  assert.equal(placeWithTip({ ...BUBBLE, anchor: high, aim: high, width: 200, height: 40, viewport: { width: 1280, height: 800 }, sides: ['top', 'bottom'] }).side, 'bottom');
});

test('right to left, a field\'s reason lines up with the field\'s right edge', () => {
  const field = edges(300, 200, 320, 36);
  const placed = placeWithTip({ ...BUBBLE, dir: 'rtl', anchor: field, aim: field, width: 200, height: 40, viewport: { width: 1280, height: 800 }, sides: ['top', 'bottom'] });
  assert.equal(placed.left, 620 - 200);
  assert.equal(placed.at, 200 - 24);
});

test('a help tip at the right edge of a phone comes nearer the edge so its tip reaches the ⓘ outside the corner', () => {
  const mark = edges(342, 100, 24, 24);
  const placed = placeWithTip({ ...HELP, anchor: mark, aim: mark, width: 300, height: 120, viewport: { width: 390, height: 844 }, sides: ['bottom', 'top'] });
  assert.equal(placed.side, 'bottom');
  const centre = 342 + 12;
  // 16 from the edge the surface starts at 74, and its straight edge ends at 74 + 281 = 355: the
  // ⓘ's middle (354) is on it, and nothing moves.
  assert.equal(placed.left + placed.at, centre, 'the tip is not on the ⓘ');
  assert.ok(placed.at <= 300 - HELP.radius - HELP.tip && placed.at >= HELP.radius + HELP.tip, 'the tip runs into a rounded corner');

  const nearer = edges(348, 100, 24, 24);
  const shifted = placeWithTip({ ...HELP, anchor: nearer, aim: nearer, width: 300, height: 120, viewport: { width: 390, height: 844 }, sides: ['bottom', 'top'] });
  assert.equal(shifted.left + shifted.at, 360, 'the tip does not reach the ⓘ');
  assert.equal(shifted.at, 300 - 12 - 7, 'the tip is at the end of the straight edge');
  assert.equal(shifted.left, 360 - 281, 'the surface came nearer the edge, only as far as the tip needs');
  assert.ok(390 - (shifted.left + 300) >= HELP.nearEdge, 'nearer the edge than the least');

  const atEdge = edges(366, 100, 24, 24);
  const stopped = placeWithTip({ ...HELP, anchor: atEdge, aim: atEdge, width: 300, height: 120, viewport: { width: 390, height: 844 }, sides: ['bottom', 'top'] });
  assert.equal(stopped.left, 390 - 8 - 300, 'no nearer the edge than the least');
  assert.equal(stopped.at, 300 - 12 - 7, 'the tip stops at the end of the straight edge, outside the corner');
});

test('a tip on a surface too small for a straight edge stands in its middle', () => {
  const mark = edges(100, 100, 24, 24);
  const placed = placeWithTip({ ...HELP, anchor: mark, aim: mark, width: 30, height: 20, viewport: { width: 390, height: 844 }, sides: ['bottom'] });
  assert.equal(placed.at, 15);
});

test('the tip points at its aim, not at the anchor: a sheet row\'s ⓘ', () => {
  const row = edges(0, 400, 390, 44);
  const mark = edges(350, 414, 16, 16);
  const placed = placeWithTip({ ...BUBBLE, anchor: row, aim: mark, width: 358, height: 40, viewport: { width: 390, height: 844 }, sides: ['top', 'bottom'] });
  assert.equal(placed.side, 'top');
  assert.equal(placed.left, 16);
  assert.equal(placed.left + placed.at, 358);
});
