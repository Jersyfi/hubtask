// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-25 checks 3, 4 and 5: Done for the last 24 hours, Recently by use, open entries under
// their categories in order.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DONE_WINDOW_MS, narrow, partition, type ListEntry } from './simplelist.ts';

const now = Date.parse('2026-10-21T12:00:00Z');
const ago = (ms: number) => new Date(now - ms).toISOString();

function entry(id: string, extra: Partial<ListEntry> & { done?: number; count?: number } = {}): ListEntry {
  const { done, count, ...rest } = extra;
  return {
    id,
    title: id,
    completion: done === undefined ? { is_completed: false } : { is_completed: true, completed_at: ago(done) },
    completion_count: count ?? 0,
    ...rest,
  };
}

test('a ticked entry stays under Done for 24 hours, newest first, and then moves to Recently', () => {
  const sections = partition(
    [
      entry('bread'),
      entry('milk', { done: 60_000 }),
      entry('eggs', { done: 5 * 60_000 }),
      entry('edge', { done: DONE_WINDOW_MS - 1000 }),
      entry('gone', { done: DONE_WINDOW_MS }),
    ],
    now,
  );
  assert.deepEqual(sections.open.flatMap((group) => group.entries.map((each) => each.id)), ['bread']);
  assert.deepEqual(sections.done.map((each) => each.id), ['milk', 'eggs', 'edge']);
  assert.deepEqual(sections.recently.map((each) => each.id), ['gone']);
  // The timer re-sorts when the oldest under Done reaches its 24 hours.
  assert.equal(sections.nextBoundary, now + 1000);
});

test('Recently is most used first, then most recently ticked, and leaves archived entries out', () => {
  const day = DONE_WINDOW_MS;
  const sections = partition(
    [
      entry('salt', { done: 3 * day, count: 1 }),
      entry('milk', { done: 9 * day, count: 7 }),
      entry('tea', { done: 2 * day, count: 1 }),
      entry('old', { done: 2 * day, count: 9, archived_at: ago(day) }),
    ],
    now,
  );
  assert.deepEqual(sections.recently.map((each) => each.id), ['milk', 'tea', 'salt']);
  assert.equal(sections.nextBoundary, undefined);
});

test('Recently narrows to what the capture line asks for, in any case and form', () => {
  const recently = [entry('Milk'), entry('oat milk'), entry('Bread'), entry('Café')];
  assert.deepEqual(narrow(recently, 'mil').map((each) => each.id), ['Milk', 'oat milk']);
  assert.deepEqual(narrow(recently, 'café').map((each) => each.id), ['Café']);
  assert.equal(narrow(recently, '  ').length, 4);
});

test('open entries sit under the first category they carry, in the categories\' order, the rest last', () => {
  const dairy = 'l-dairy';
  const produce = 'l-produce';
  const deleted = 'l-gone';
  const sections = partition(
    [
      entry('milk', { label_ids: [dairy] }),
      entry('apples', { label_ids: [produce, dairy] }),
      entry('salt'),
      entry('bread', { label_ids: [deleted] }),
      entry('cheese', { label_ids: ['l-organic', dairy] }),
    ],
    now,
    [deleted, produce, dairy],
    new Set([produce, dairy, 'l-organic']),
  );
  assert.deepEqual(
    sections.open.map((group) => [group.category, group.entries.map((each) => each.id)]),
    [
      [produce, ['apples']],
      [dairy, ['milk', 'cheese']],
      [null, ['salt', 'bread']],
    ],
  );
});

test('without categories the open entries are one group in their order, and none is no group', () => {
  assert.deepEqual(partition([entry('b'), entry('a')], now).open.map((group) => group.entries.map((each) => each.id)), [['b', 'a']]);
  assert.deepEqual(partition([], now).open, []);
});
