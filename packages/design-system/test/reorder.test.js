// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The rank arithmetic, checked the way `focus.test.js` checks the keyboard: as numbers, in plain
// Node. A drag is the one interaction in this product that cannot be checked without a browser,
// which is exactly why everything about it that *can* be a function is one.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  DRAG_THRESHOLD_PX,
  boundariesOf,
  dropIndex,
  hasLeftTheHandle,
  rankIntent,
  rankTarget,
} from '../src/reorder.ts';

// --- the command ------------------------------------------------------------------------------

test('the four commands name the four positions', () => {
  assert.equal(rankTarget('up', 3, 6), 2);
  assert.equal(rankTarget('down', 3, 6), 4);
  assert.equal(rankTarget('top', 3, 6), 0);
  assert.equal(rankTarget('bottom', 3, 6), 5);
});

test('a command that asks for the position it already holds answers nothing', () => {
  // The reason a menu shows a reason rather than a control that does nothing: "already first" is a
  // different answer from "move to the first position", and only one of them is a request.
  assert.equal(rankTarget('up', 0, 6), null);
  assert.equal(rankTarget('top', 0, 6), null);
  assert.equal(rankTarget('down', 5, 6), null);
  assert.equal(rankTarget('bottom', 5, 6), null);
});

test('a row that is not in the list ranks nothing', () => {
  // The ordinary case rather than a defensive one: a level re-read underneath the reader can leave
  // a menu open over a row that is no longer in it.
  assert.equal(rankTarget('up', -1, 6), null);
  assert.equal(rankTarget('down', 6, 6), null);
  assert.equal(rankTarget('top', 0, 0), null);
});

// --- the keys ---------------------------------------------------------------------------------

const press = (key, modifiers = {}) => ({ key, altKey: false, shiftKey: false, ...modifiers });

test('Alt and the vertical arrows move one place', () => {
  assert.equal(rankIntent(press('ArrowUp', { altKey: true }), 3, 6), 2);
  assert.equal(rankIntent(press('ArrowDown', { altKey: true }), 3, 6), 4);
});

test('Shift widens one step into the whole level', () => {
  assert.equal(rankIntent(press('ArrowUp', { altKey: true, shiftKey: true }), 3, 6), 0);
  assert.equal(rankIntent(press('ArrowDown', { altKey: true, shiftKey: true }), 3, 6), 5);
});

test('the horizontal arrows are the browser’s, and are left alone', () => {
  // Alt+Left and Alt+Right are back and forward. A shortcut on them is a shortcut that sometimes
  // navigates away from the list instead of ranking in it.
  assert.equal(rankIntent(press('ArrowLeft', { altKey: true }), 3, 6), null);
  assert.equal(rankIntent(press('ArrowRight', { altKey: true }), 3, 6), null);
  assert.equal(rankIntent(press('Home', { altKey: true }), 3, 6), null);
});

test('an arrow without Alt is the arrow, not a rank change', () => {
  assert.equal(rankIntent(press('ArrowUp'), 3, 6), null);
  assert.equal(rankIntent(press('ArrowDown'), 3, 6), null);
});

test('the platform’s own modifiers are refused rather than ignored', () => {
  // Ctrl and Meta are what a platform builds its shortcuts out of. A handler that answered
  // Ctrl+Alt+Down would be answering somebody else's key.
  assert.equal(rankIntent(press('ArrowDown', { altKey: true, ctrlKey: true }), 3, 6), null);
  assert.equal(rankIntent(press('ArrowDown', { altKey: true, metaKey: true }), 3, 6), null);
});

test('a key at the end of the level answers nothing, exactly as the command does', () => {
  assert.equal(rankIntent(press('ArrowUp', { altKey: true }), 0, 6), null);
  assert.equal(rankIntent(press('ArrowDown', { altKey: true }), 5, 6), null);
});

// --- the pointer ------------------------------------------------------------------------------

const rows = [
  { start: 0, end: 20 },
  { start: 24, end: 44 },
  { start: 48, end: 68 },
];

test('the boundaries are the middles of the gaps, one fewer than there are rows', () => {
  // The middle of the gap rather than the top of the next row: the gap belongs to neither, and a
  // pointer resting in it would otherwise already count as being on the lower row.
  assert.deepEqual(boundariesOf(rows), [22, 46]);
  assert.deepEqual(boundariesOf([rows[0]]), []);
  assert.deepEqual(boundariesOf([]), []);
});

test('the position is how many boundaries the pointer has passed', () => {
  const lines = boundariesOf(rows);
  assert.equal(dropIndex(10, lines), 0);
  assert.equal(dropIndex(30, lines), 1);
  assert.equal(dropIndex(60, lines), 2);
});

test('past either edge is the end of the list rather than nothing', () => {
  // A reader dragging above the first row means the top. Answering `null` there would refuse the
  // one position that is hardest to hit exactly.
  const lines = boundariesOf(rows);
  assert.equal(dropIndex(-400, lines), 0);
  assert.equal(dropIndex(4000, lines), 2);
});

test('a list of one has nowhere to drop but itself', () => {
  assert.equal(dropIndex(10, []), 0);
  assert.equal(dropIndex(-10, []), 0);
});

test('a press is not a drag until the pointer has left the handle', () => {
  // A row is a link. A drag that began at the first pixel would make a list of links unopenable
  // with a shaky hand or a trackpad.
  const from = { x: 100, y: 100 };
  assert.equal(hasLeftTheHandle(from, { x: 102, y: 101 }), false);
  assert.equal(hasLeftTheHandle(from, { x: 100, y: 100 + DRAG_THRESHOLD_PX }), true);
  assert.equal(hasLeftTheHandle(from, { x: 100 - DRAG_THRESHOLD_PX, y: 100 }), true);
});

test('the threshold is a distance rather than one axis', () => {
  // Diagonal travel counts: a drag begun at an angle is still a drag, and measuring one axis at a
  // time would start it late in the direction the reader happened to be moving.
  assert.equal(hasLeftTheHandle({ x: 0, y: 0 }, { x: 5, y: 5 }), true);
  assert.equal(hasLeftTheHandle({ x: 0, y: 0 }, { x: 3, y: 3 }), false);
});
