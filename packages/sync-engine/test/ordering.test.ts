// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The server's own cases for its order keys (`core/domain/service/Ordering_test.go`), ported: a
// key minted here between two neighbours is the key the server would mint.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { OrderKeyError, orderKeyAfter, orderKeyBetween } from '../src/ordering.ts';

test('the server\'s cases', () => {
  for (const [previous, next, want] of [
    ['', '', 'a0'],
    ['a0', '', 'a1'],
    ['az', '', 'b00'],
    ['', 'a0', 'Zz'],
    ['', 'a0V', 'a0'],
    ['a0', 'a1', 'a0V'],
    ['a0', 'a2', 'a1'],
    ['a0V', 'a0W', 'a0VV'],
    ['a0VVV', 'a0VVW', 'a0VVVV'],
  ] as const) {
    assert.equal(orderKeyBetween(previous, next), want, `between ${JSON.stringify(previous)} and ${JSON.stringify(next)}`);
  }
});

test('a key is always strictly between its neighbours, and appends stay short', () => {
  let last = '';
  const keys: string[] = [];
  for (let i = 0; i < 2000; i += 1) {
    last = orderKeyAfter(last);
    keys.push(last);
  }
  for (let i = 1; i < keys.length; i += 1) assert.ok((keys[i - 1] as string) < (keys[i] as string));
  assert.ok((keys.at(-1) as string).length <= 4, `2000 appends grew to ${keys.at(-1)}`);

  let lower = 'a0';
  const upper = 'a1';
  for (let i = 0; i < 50; i += 1) {
    const between = orderKeyBetween(lower, upper);
    assert.ok(lower < between && between < upper, `${lower} < ${between} < ${upper}`);
    lower = between;
  }
  let first = 'a0';
  for (let i = 0; i < 100; i += 1) {
    const before = orderKeyBetween('', first);
    assert.ok(before < first);
    first = before;
  }
});

test('neighbours the wrong way round and malformed keys are refused by code', () => {
  assert.throws(() => orderKeyBetween('a1', 'a0'), (e: unknown) => e instanceof OrderKeyError && e.detailCode === 'ordering.bounds_invalid');
  assert.throws(() => orderKeyBetween('a0', 'a0'), (e: unknown) => e instanceof OrderKeyError && e.detailCode === 'ordering.bounds_invalid');
  assert.throws(() => orderKeyBetween('a0!', ''), (e: unknown) => e instanceof OrderKeyError && e.detailCode === 'ordering.key_malformed');
  assert.throws(() => orderKeyBetween('a00', ''), (e: unknown) => e instanceof OrderKeyError && e.detailCode === 'ordering.key_malformed');
});
