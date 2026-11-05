// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-WRK-31 check 3: a move that takes a column over its limit is announced as going over it, also
// when the query left the column's count out.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { goesOverLimit } from './wip.ts';

test('a counted column goes over its limit by the move that passes it', () => {
  assert.equal(goesOverLimit(3, 2, 2), false);
  assert.equal(goesOverLimit(3, 3, 3), true);
  assert.equal(goesOverLimit(null, 99, 50), false);
});

test('a column whose count was left out is read by the cards it holds, never as empty', () => {
  assert.equal(goesOverLimit(5, null, 50), true);
  assert.equal(goesOverLimit(5, undefined, 4), false);
});
