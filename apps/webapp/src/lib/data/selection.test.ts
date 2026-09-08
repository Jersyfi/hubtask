// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { bulkCapOf, isWithinCap, rangeTo, stillVisible, toggle, toggleAll } from './selection.ts';

const VISIBLE = ['a', 'b', 'c', 'd', 'e'];

test('a plain pick adds, and picking it again takes it away', () => {
  assert.deepEqual(toggle([], 'b'), ['b']);
  assert.deepEqual(toggle(['a', 'b'], 'b'), ['a']);
});

test('a range runs through what is on screen, in either direction', () => {
  // Not through identifiers and not through creation time: shift-click means "everything between
  // these two rows as I see them", and the visible order is the only order that answers that.
  assert.deepEqual(rangeTo(VISIBLE, [], 'b', 'd'), ['b', 'c', 'd']);
  assert.deepEqual(rangeTo(VISIBLE, [], 'd', 'b'), ['b', 'c', 'd']);
});

test('a range adds to what is held rather than replacing it', () => {
  // Somebody who picked one row and then shift-clicked a range meant to have both.
  assert.deepEqual(rangeTo(VISIBLE, ['a'], 'c', 'd'), ['a', 'c', 'd']);
  // And nothing is added twice.
  assert.deepEqual(rangeTo(VISIBLE, ['c'], 'b', 'd'), ['c', 'b', 'd']);
});

test('a range whose anchor has gone is a plain pick', () => {
  // The other end of the range is not on screen any more, and inventing one would select rows
  // nobody pointed at.
  assert.deepEqual(rangeTo(VISIBLE, ['a'], 'gone', 'd'), ['a', 'd']);
  assert.deepEqual(rangeTo(VISIBLE, [], undefined, 'd'), ['d']);
});

test('one control selects everything visible and clears it again', () => {
  assert.deepEqual(toggleAll(VISIBLE, []), VISIBLE);
  assert.deepEqual(toggleAll(VISIBLE, VISIBLE), []);
  // Half-selected means "select the rest", which is what a reader pressing it once more expects.
  assert.deepEqual(toggleAll(VISIBLE, ['b']), ['b', 'a', 'c', 'd', 'e']);
  // And it only ever speaks about what is on screen: a row on another page stays picked.
  assert.deepEqual(toggleAll(VISIBLE, [...VISIBLE, 'elsewhere']), ['elsewhere']);
});

test('a selection is never wider than the list, and keeps the drawn order', () => {
  // A row that was filtered away or trashed by somebody else leaves the selection with it: a bar
  // saying "12 selected" over eleven rows would send an operation about an entry nobody can see.
  assert.deepEqual(stillVisible(VISIBLE, ['d', 'gone', 'a']), ['a', 'd']);
});

test('the cap is the installation’s, and an absent one is not a cap of zero', () => {
  assert.equal(bulkCapOf({ max_bulk_operations: 500 }), 500);
  assert.equal(bulkCapOf({}), undefined);
  assert.equal(bulkCapOf(undefined), undefined);
  assert.equal(isWithinCap(500, 500), true);
  assert.equal(isWithinCap(501, 500), false);
  assert.equal(isWithinCap(9000, undefined), true);
});
