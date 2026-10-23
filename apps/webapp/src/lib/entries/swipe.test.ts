// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-25 check 6 and ADR-0088: a swipe ticks, a still hold opens the menu, a hold that moves
// drags, and a scroll is left alone.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { classify, HOLD_MS, isSwipe, startsAtEdge } from './swipe.ts';

const start = { x: 100, y: 100, at: 0 };
const threshold = 48;

test('a horizontal move past the threshold before the hold is a swipe, towards the inline end or back', () => {
  assert.equal(classify(start, { x: 160, y: 110, at: 120 }, threshold), 'swipe-end');
  assert.equal(classify(start, { x: 40, y: 110, at: 120 }, threshold), 'swipe-start');
  // Mirrored in a right-to-left page: the inline end is on the left.
  assert.equal(classify(start, { x: 40, y: 110, at: 120 }, threshold, true), 'swipe-end');
  assert.equal(classify(start, { x: 160, y: 110, at: 120 }, threshold, true), 'swipe-start');
});

test('a vertical move is a scroll, and a small one is nothing yet', () => {
  assert.equal(classify(start, { x: 130, y: 200, at: 120 }, threshold), 'scroll');
  assert.equal(classify(start, { x: 120, y: 110, at: 120 }, threshold), 'pending');
});

test('a still hold opens the menu, and a hold that then moves drags', () => {
  assert.equal(classify(start, { x: 105, y: 102, at: HOLD_MS }, threshold), 'hold');
  assert.equal(classify(start, { x: 105, y: 180, at: HOLD_MS + 50 }, threshold), 'drag');
  // A sideways move after the hold is a drag too, never a late swipe.
  assert.equal(classify(start, { x: 200, y: 100, at: HOLD_MS + 50 }, threshold), 'drag');
});

test('the board reads the same rule', () => {
  assert.equal(isSwipe(60, 10, threshold), true);
  assert.equal(isSwipe(30, 5, threshold), false);
  assert.equal(isSwipe(60, 80, threshold), false);
});

test('a touch in the edge band is the platform\'s back gesture', () => {
  assert.equal(startsAtEdge(10, 375, 24), true);
  assert.equal(startsAtEdge(370, 375, 24), true);
  assert.equal(startsAtEdge(100, 375, 24), false);
});
