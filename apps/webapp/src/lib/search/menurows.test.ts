// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { rowsOf, step } from './menurows.ts';
import type { Quick } from '../data/searchquery.ts';

const QUICK: readonly Quick[] = [
  { code: 'a', line: 'who:me is:open', field: 'assignee_id' },
  { code: 'b', line: 'is:open due:week', field: 'due_at' },
];

test('with words, the rows are the hits, the narrowings, and both ways on', () => {
  const rows = rowsOf('milk', ['one', 'two'], QUICK);
  assert.deepEqual(
    rows.map((row) => row.kind),
    ['hit', 'hit', 'quick', 'quick', 'all', 'refine'],
  );
});

test('with an empty field there is nothing to refine, so that row is not offered', () => {
  const rows = rowsOf('   ', [], QUICK);
  assert.deepEqual(
    rows.map((row) => row.kind),
    ['quick', 'quick', 'all'],
  );
});

test('a row carries what pressing it needs: the hit, or the narrowing as a line', () => {
  const rows = rowsOf('milk', ['one'], QUICK);
  assert.equal(rows[0]?.id, 'one');
  assert.equal(rows[1]?.id, 'who:me is:open');
});

test('the arrows walk down and come back to the words', () => {
  // -1 is "nothing highlighted", which is the state a field being typed in is always in: Enter
  // then searches for what was typed rather than opening whatever happened to be first.
  assert.equal(step(-1, 3, 1), 0);
  assert.equal(step(0, 3, 1), 1);
  assert.equal(step(2, 3, 1), -1);
});

test('and up, from the words to the last row', () => {
  assert.equal(step(-1, 3, -1), 2);
  assert.equal(step(0, 3, -1), -1);
});

test('an empty menu has nowhere to go', () => {
  assert.equal(step(-1, 0, 1), -1);
  assert.equal(step(-1, 0, -1), -1);
});
