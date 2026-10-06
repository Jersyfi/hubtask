// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic behind a sorted table, checked without a browser.
//
// The same bargain structure.test.js makes. Two of these are the ones a browser would never have
// caught: an empty cell staying last when the direction flips, and the third press of a heading
// giving the caller's own order back.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { comparing, nextSort } from '../src/table.ts';

// --- the sort cycle ---------------------------------------------------------------------------

test('a heading cycles ascending, descending, and back to the list as it arrived', () => {
  let sort = { direction: 'none' };
  sort = nextSort(sort, 'due');
  assert.deepEqual(sort, { columnId: 'due', direction: 'ascending' });
  sort = nextSort(sort, 'due');
  assert.deepEqual(sort, { columnId: 'due', direction: 'descending' });

  // The third press is the one Atlassian's table does not have. `SessionsView` answers "this
  // device first, then newest"; without a way home that order is gone for the rest of the visit.
  sort = nextSort(sort, 'due');
  assert.deepEqual(sort, { direction: 'none' });
});

test('a different column starts at ascending rather than continuing this one phase', () => {
  const sort = nextSort({ columnId: 'due', direction: 'descending' }, 'title');
  assert.deepEqual(sort, { columnId: 'title', direction: 'ascending' });
});

// --- the comparator ---------------------------------------------------------------------------

const by = (rows, read, direction) => [...rows].sort(comparing(read, direction, 'en')).map((r) => r.id);

test('numbers compare as numbers, which is why a key is not stringified first', () => {
  const rows = [{ id: 'a', n: 9 }, { id: 'b', n: 10 }, { id: 'c', n: 1 }];
  assert.deepEqual(by(rows, (r) => r.n, 'ascending'), ['c', 'a', 'b']);
  assert.deepEqual(by(rows, (r) => r.n, 'descending'), ['b', 'a', 'c']);
});

test('text compares under the reader collation, so A-umlaut sits beside A', () => {
  const rows = [{ id: 'z', s: 'Zebra' }, { id: 'ae', s: 'Ärger' }, { id: 'a', s: 'Anfang' }];
  // Under a codepoint sort `Ärger` (U+00C4) follows `Zebra`. Under any collation a German reader
  // has, it does not. `i18n-l10n.md` §5.
  assert.deepEqual([...rows].sort(comparing((r) => r.s, 'ascending', 'de')).map((r) => r.id), [
    'a',
    'ae',
    'z',
  ]);
});

test('an empty cell sorts last in BOTH directions', () => {
  const rows = [{ id: 'a', due: '2026-01-01' }, { id: 'none', due: null }, { id: 'b', due: '2026-06-01' }];

  // A missing due date is not "earliest" and not "latest" - it is absent. Flipping the direction
  // must not promote every undated entry to the top of the screen, which is exactly what a naive
  // `sign * compare` does.
  assert.deepEqual(by(rows, (r) => r.due, 'ascending'), ['a', 'b', 'none']);
  assert.deepEqual(by(rows, (r) => r.due, 'descending'), ['b', 'a', 'none']);
});

test('an empty string counts as absent, because a cleared field is not a name', () => {
  const rows = [{ id: 'a', s: 'Anna' }, { id: 'blank', s: '' }];
  assert.deepEqual(by(rows, (r) => r.s, 'ascending'), ['a', 'blank']);
  assert.deepEqual(by(rows, (r) => r.s, 'descending'), ['a', 'blank']);
});

test('none keeps the order the caller handed over', () => {
  const rows = [{ id: 'c', n: 3 }, { id: 'a', n: 1 }, { id: 'b', n: 2 }];
  assert.deepEqual(by(rows, (r) => r.n, 'none'), ['c', 'a', 'b']);
});
