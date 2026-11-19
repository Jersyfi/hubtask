// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { KEPT_SHOWN, keptRows, percentOf } from './kept.ts';

const row = (goal: 'FIRST_RESPONSE' | 'NEXT_RESPONSE' | 'RESOLUTION', ended: number, kept: number, kind: 'SLA' | 'OLA' = 'SLA', group_id: string | null = null) =>
  ({ kind, goal, group_id, ended, kept });

test('the deadlines kept come worst first, any number of kinds, the share the client\'s arithmetic (UC-SVC-02 check 15)', () => {
  const rows = keptRows([row('FIRST_RESPONSE', 2, 1), row('RESOLUTION', 4, 4), row('FIRST_RESPONSE', 3, 1, 'OLA', 'g-1'), row('NEXT_RESPONSE', 0, 0)]);
  assert.deepEqual(rows.map((each) => [each.kind, each.goal, each.share, each.breached]), [
    ['OLA', 'FIRST_RESPONSE', 1 / 3, 2],
    ['SLA', 'FIRST_RESPONSE', 0.5, 1],
    ['SLA', 'RESOLUTION', 1, 0],
  ], 'a kind with nothing ended in the window is no row');
  assert.equal(KEPT_SHOWN, 3, 'at most three rows, then "+ n more" (the owner\'s decision of 2026-11-12)');
});

test('an equal share puts the kind with more breaches first', () => {
  const rows = keptRows([row('RESOLUTION', 2, 1), row('FIRST_RESPONSE', 4, 2)]);
  assert.deepEqual(rows.map((each) => each.goal), ['FIRST_RESPONSE', 'RESOLUTION']);
});

test('a share is a whole percent, rounded down so that a breach never reads as 100 %', () => {
  assert.equal(percentOf(199 / 200), 99);
  assert.equal(percentOf(1), 100);
  assert.equal(percentOf(0), 0);
});
