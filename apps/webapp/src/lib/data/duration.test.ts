// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { durationOf, isPlainDuration, partsOf } from './duration.ts';

test('years and months are refused, and minutes are not', () => {
  // Two features refuse them for one reason, which is why the rule lives in one module: they are
  // calendar arithmetic rather than a length of time. The `M` after a `T` is minutes.
  assert.equal(isPlainDuration('-P1Y'), false);
  assert.equal(isPlainDuration('-P2M'), false);
  assert.equal(isPlainDuration('P3D'), true);
  assert.equal(isPlainDuration('-PT30M'), true);
  assert.equal(isPlainDuration('P1W'), true);
  assert.equal(isPlainDuration('P1DT12H'), true);
  assert.equal(isPlainDuration('P'), false);
  assert.equal(isPlainDuration('3 days'), false);
});

test('an amount and a unit become a duration, on the right side of the T', () => {
  // The grammar rather than a preference: `PT3D` and `P3M` are both something other than meant.
  assert.equal(durationOf(3, 'D'), 'P3D');
  assert.equal(durationOf(3, 'D', true), '-P3D');
  assert.equal(durationOf(1, 'W'), 'P1W');
  assert.equal(durationOf(2, 'H', true), '-PT2H');
  assert.equal(durationOf(30, 'M'), 'PT30M');
  // A negative amount with the flag already saying "before" is not two negatives.
  assert.equal(durationOf(-3, 'D', true), '-P3D');
});

test('a duration comes back apart, and a compound one does not pretend to', () => {
  assert.deepEqual(partsOf('P3D'), { amount: 3, unit: 'D', isBefore: false });
  assert.deepEqual(partsOf('-PT2H'), { amount: 2, unit: 'H', isBefore: true });
  // Two units is not something an amount-and-unit control can show, so it says so rather than
  // rounding it into something it is not — the raw value stays whatever it was.
  assert.equal(partsOf('P1DT12H'), undefined);
  assert.equal(partsOf('-P1Y'), undefined);
});

test('every duration this composes is one it can read back', () => {
  for (const unit of ['W', 'D', 'H', 'M'] as const) {
    for (const before of [true, false]) {
      const composed = durationOf(5, unit, before);
      assert.deepEqual(partsOf(composed), { amount: 5, unit, isBefore: before }, composed);
    }
  }
});
