// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The conversion from a position to a neighbour, checked at the case that is easy to get wrong.
//
// These tests were `containers.test.ts`'s while only containers could be ranked. They moved with
// the function when entries learned to, because the assertion is about the arithmetic and not
// about what kind of thing is being ranked.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { anchorFor } from './rank.ts';

const level = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];

test('moving up lands before the sibling at that position', () => {
  assert.equal(anchorFor(level, 'c', 0), 'a');
  assert.equal(anchorFor(level, 'c', 1), 'b');
});

test('moving down past its own position lands before the one after it', () => {
  // The case that is easy to get wrong. `a` moving to position 1 must land before `c`, not before
  // `b` — because with `a` taken out of the list, position 1 *is* `c`.
  assert.equal(anchorFor(level, 'a', 1), 'c');
});

test('the end of the list appends rather than naming a sibling', () => {
  // Null is the API's own word for "the end", so there is nothing to invent here.
  assert.equal(anchorFor(level, 'a', 2), null);
  assert.equal(anchorFor(level, 'a', 99), null);
});

test('an entry that is not in the level appends rather than throwing', () => {
  // A level re-read underneath the reader can leave a menu open over a row it no longer holds.
  // Appending is the harmless answer; the position the reader asked for no longer exists.
  assert.equal(anchorFor(level, 'z', 3), null);
  assert.equal(anchorFor([], 'a', 0), null);
});
