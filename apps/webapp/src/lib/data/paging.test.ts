// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { columnCards, columnRequest, reachOf } from './paging.ts';

const rows = (n: number) => Array.from({ length: n }, (_, i) => ({ id: `i-${i}` }));

test('a collection of 250 read 200 at a time says there is more, and how much', () => {
  assert.deepEqual(reachOf({ data: rows(200), total: 250, page: { next_cursor: 'c1', has_more: true } }), { shown: 200, total: 250, hasMore: true });
});

test('the appended second page reaches the end', () => {
  assert.deepEqual(reachOf({ data: rows(250), total: 250, page: { next_cursor: null, has_more: false } }), { shown: 250, total: 250, hasMore: false });
});

test('an answer without has_more but with a cursor still has more', () => {
  assert.equal(reachOf({ data: rows(200), page: { next_cursor: 'c1' } }).hasMore, true);
  assert.equal(reachOf({ data: rows(200), page: { next_cursor: null } }).hasMore, false);
});

test('an answer not yet there holds nothing', () => {
  assert.deepEqual(reachOf(undefined), { shown: 0, total: undefined, hasMore: false });
});

test('a column is continued by its key and its cursor, without the grouping', () => {
  const board = {
    scope: { container_id: 'c-1', include_descendants: false }, include_archived: true,
    group_by: { field: 'bucket_id', limit_per_group: 50 }, sort: [{ field: 'order_key', dir: 'ASC' }],
    filter: { op: 'EQ', field: 'is_completed', value: false }, expand: ['labels', 'counts'], count: 'exact',
  };
  assert.deepEqual(columnRequest(board, 'b-1', 'cur-1'), {
    scope: { container_id: 'c-1', include_descendants: false }, include_archived: true,
    sort: [{ field: 'order_key', dir: 'ASC' }], expand: ['labels', 'counts'],
    filter: { op: 'AND', nodes: [{ op: 'EQ', field: 'is_completed', value: false }, { field: 'bucket_id', op: 'EQ', value: 'b-1' }] },
    page: { cursor: 'cur-1', size: 50 }, count: 'none',
  });
  // The cards with no column are the ones whose value is absent.
  assert.deepEqual(columnRequest({ scope: {}, group_by: { field: 'entry_type' } }, null, 'cur-2').filter, { field: 'entry_type', op: 'IS_NULL' });
});

test('a column of 120 cards draws each of them once', () => {
  const first = rows(50);
  const more = rows(120).slice(48);
  const cards = columnCards(first, more);
  assert.equal(cards.length, 120);
  assert.equal(new Set(cards.map((card) => card.id)).size, 120);
});
