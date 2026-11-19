// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { handOver, takeOver } from './listhandoff.ts';

// UC-GOL-04 check 22: a widget's line reaches the collection in memory, once, and never another one.
test('a handed line is taken once, by the collection it was meant for', () => {
  handOver('c1', 'status: open', 1_000);
  assert.equal(takeOver('c2', 1_100), undefined);
  assert.equal(takeOver('c1', 1_200), 'status: open');
  assert.equal(takeOver('c1', 1_300), undefined);
});

test('a line left too long ago opens nothing', () => {
  handOver('c1', 'status: open', 1_000);
  assert.equal(takeOver('c1', 20_000), undefined);
});
