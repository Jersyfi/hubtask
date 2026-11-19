// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { followsMatrix } from './priority.ts';

test('a priority follows impact × urgency only where it is the matrix\'s (UC-SVC-02 check 14)', () => {
  const cases: [string, string, string, boolean][] = [
    ['High', 'High', 'P1', true],
    ['High', 'Medium', 'P2', true],
    ['Medium', 'High', 'P2', true],
    ['High', 'Low', 'P3', true],
    ['Medium', 'Medium', 'P3', true],
    ['Low', 'High', 'P3', true],
    ['Medium', 'Low', 'P4', true],
    ['Low', 'Low', 'P4', true],
    ['High', 'Medium', 'P1', false],
    ['Low', 'Low', 'P2', false],
  ];
  for (const [impact, urgency, priority, follows] of cases) {
    assert.equal(followsMatrix({ impact, urgency, priority }), follows, `${impact} × ${urgency} = ${priority}`);
  }
  assert.equal(followsMatrix({ priority: 'P1' }), false, 'no impact or urgency, nothing follows');
  assert.equal(followsMatrix(undefined), false);
});
