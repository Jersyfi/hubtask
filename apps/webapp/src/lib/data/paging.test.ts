// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { reachOf } from './paging.ts';

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
