// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// ADR-0039's fallback, tested where it can be: over numbers.
//
// The ADR asks for exactly this. "The CSS path is what is styled and reviewed, and the workbench
// shows the CSS path. A story cannot show both, so the fallback needs a test of its own rather
// than an axis." What follows is that test - the flip, the shift, and the direction handling the
// ADR names as the first thing to check, because `position-area` is logical and a fallback that
// measured in physical pixels would be right in English and wrong in Arabic.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { logicalRect, positionArea, resolve } from '../src/positioning.ts';

const viewport = { inlineStart: 0, blockStart: 0, inlineSize: 1000, blockSize: 600 };
/** A trigger in the middle of the viewport, well clear of every edge. */
const middle = { inlineStart: 400, blockStart: 200, inlineSize: 120, blockSize: 40 };
const size = { inlineSize: 200, blockSize: 100 };

test('an overlay with room sits on the side it was asked for', () => {
  const at = resolve(middle, size, viewport, { side: 'block-end', align: 'start' }, 4);
  assert.equal(at.side, 'block-end');
  assert.equal(at.blockStart, 244); // under the anchor, plus the gap
  assert.equal(at.inlineStart, 400); // start-aligned with it
});

test('centre alignment centres on the anchor, not on the viewport', () => {
  const at = resolve(middle, size, viewport, { side: 'block-end', align: 'center' });
  assert.equal(at.inlineStart, 360); // 400 + (120 - 200) / 2
});

test('an overlay with no room below flips above', () => {
  const low = { ...middle, blockStart: 540 };
  const at = resolve(low, size, viewport, { side: 'block-end', align: 'start' }, 4);
  assert.equal(at.side, 'block-start');
  assert.equal(at.blockStart, 436); // 540 - 100 - 4
});

test('an overlay that fits on neither side stays where it was asked and is shifted in', () => {
  // A viewport shorter than the overlay: flipping would swap one overflow for another, and the
  // side the reader was not looking at is the worse of the two.
  const tiny = { ...viewport, blockSize: 120 };
  const at = resolve({ ...middle, blockStart: 40 }, size, tiny, { side: 'block-end', align: 'start' }, 4);
  assert.equal(at.side, 'block-end');
  assert.equal(at.blockStart, 20); // shifted back inside rather than left off the edge
});

test('the shift keeps the overlay a margin away from the edge', () => {
  const atEnd = { ...middle, inlineStart: 940 };
  const at = resolve(atEnd, size, viewport, { side: 'block-end', align: 'start' }, 4, 8);
  assert.equal(at.inlineStart, 792); // 1000 - 8 - 200
});

test('a rect is read from the inline-end edge in a right-to-left document', () => {
  // The one line that makes the fallback behave like `position-area`. A trigger 40 px from the
  // right edge is 40 px from the *inline start* in RTL, and a fallback that reported 860 would
  // place every menu on the wrong side of the screen.
  const rect = { left: 900, right: 960, top: 100, width: 60, height: 20 };
  assert.deepEqual(logicalRect(rect, 1000, 'rtl'), {
    inlineStart: 40,
    blockStart: 100,
    inlineSize: 60,
    blockSize: 20,
  });
  assert.equal(logicalRect(rect, 1000, 'ltr').inlineStart, 900);
});

test('the CSS path and the fallback agree on what a placement means', () => {
  // `span-inline-end` runs from the anchor's inline start towards its end, which is what
  // `align: 'start'` means here. Getting this table backwards is a bug that only shows up as
  // "the menu is left-aligned in Chrome and right-aligned in Firefox".
  assert.equal(positionArea({ side: 'block-end', align: 'start' }), 'block-end span-inline-end');
  assert.equal(positionArea({ side: 'block-end', align: 'end' }), 'block-end span-inline-start');
  assert.equal(positionArea({ side: 'block-end', align: 'center' }), 'block-end');
  assert.equal(positionArea({ side: 'inline-end', align: 'start' }), 'inline-end span-block-end');
});
