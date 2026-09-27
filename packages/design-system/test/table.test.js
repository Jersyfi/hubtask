// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic behind a sorted, paged table, checked without a browser.
//
// The same bargain structure.test.js makes. Three of these are the ones a browser would never
// have caught: an empty cell staying last when the direction flips, a page number surviving the
// deletion of the page it was on, and a pager keeping its width as the reader walks through it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { comparing, gapTarget, nextSort, pageOf, pageSteps } from '../src/table.ts';

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

// --- the page ---------------------------------------------------------------------------------

const list = (n) => Array.from({ length: n }, (_, i) => i + 1);

test('a page names the range a reader is looking at', () => {
  const page = pageOf(list(91), 3, 16);
  assert.equal(page.firstRow, 33);
  assert.equal(page.lastRow, 48);
  assert.equal(page.total, 91);
  assert.equal(page.pageCount, 6);
});

test('the last page is short and says so', () => {
  const page = pageOf(list(91), 6, 16);
  assert.equal(page.firstRow, 81);
  assert.equal(page.lastRow, 91);
  assert.equal(page.rows.length, 11);
});

test('a page past the end lands on the last one rather than on nothing', () => {
  // The reader is on page 9 and deletes the last row of it. Without the clamp they are left
  // looking at an empty page under a pager that says 9 of 8.
  const page = pageOf(list(20), 9, 10);
  assert.equal(page.page, 2);
  assert.equal(page.rows.length, 10);
});

test('an empty list is page 1 of 1 with a range of nothing', () => {
  const page = pageOf([], 1, 10);
  assert.equal(page.pageCount, 1);
  assert.equal(page.firstRow, 0);
  assert.equal(page.lastRow, 0);
  assert.equal(page.total, 0);
});

// --- the pager ---------------------------------------------------------------------------------

const shape = (page, count, window = 1) =>
  pageSteps(page, count, window)
    .map((step) => (step.kind === 'page' ? String(step.page) : `${step.from}-${step.to}`))
    .join(' ');

test('a short list shows every page and no gap', () => {
  assert.equal(shape(1, 4), '1 2 3 4');
});

test('the ends are always reachable and the middle is a gap', () => {
  assert.equal(shape(5, 20), '1 2-3 4 5 6 7-19 20');
});

test('the pager keeps its width as the reader walks through it', () => {
  // Five steps at every position, so nothing slides under the pointer between one press and the
  // next. The window is clipped at the ends, so it has to grow back inwards to hold the count.
  const widths = [1, 2, 5, 10, 19, 20].map((page) => pageSteps(page, 20).length);
  assert.deepEqual(widths, [7, 7, 7, 7, 7, 7]);
});

test('a gap is never drawn in place of a single page', () => {
  // Between 1 and 3 there is one page. A control hiding it costs the same width as the page it
  // hides and adds a press, so page 2 is drawn instead of a gap standing for page 2.
  assert.equal(shape(4, 7), '1 2 3 4 5 6 7');
  assert.ok(pageSteps(4, 7).every((step) => step.kind !== 'gap'));
});

test('a gap knows which pages it stands for, so it can be announced and pressed', () => {
  const gap = pageSteps(2, 40).find((step) => step.kind === 'gap');
  assert.deepEqual({ from: gap.from, to: gap.to }, { from: 6, to: 39 });
  // Pressing it goes to the middle of what it hides rather than nowhere, which is the difference
  // between a control and a dead ellipsis.
  assert.equal(gapTarget(gap), 22);
});

test('one page is one step and no arrows worth drawing', () => {
  assert.deepEqual(pageSteps(1, 1), [{ kind: 'page', page: 1 }]);
});
