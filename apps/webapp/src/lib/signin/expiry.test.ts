// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { phaseOf, remaining } from './expiry.ts';

const at = '2026-09-24T10:05:00Z';
const now = Date.parse('2026-09-24T10:00:08Z');

test('the clock reads as minutes and seconds', () => {
  assert.equal(remaining(at, now), '4:52');
});

test('a second place is kept, so the number does not jump from 4:09 to 4:9', () => {
  assert.equal(remaining('2026-09-24T10:04:17Z', now), '4:09');
});

test('an expired credential is zero rather than a negative number', () => {
  assert.equal(remaining('2026-09-24T09:59:00Z', now), '0:00');
});

test('an unreadable instant answers nothing rather than NaN', () => {
  assert.equal(remaining('soon', now), undefined);
});

// UC-ID-02 check 2 (SC-18): the step says when it is about to end, and ends at zero.
test('the wait runs, then closes in its last minute, then is over', () => {
  assert.equal(phaseOf(at, Date.parse('2026-09-24T10:03:59Z')), 'running');
  assert.equal(phaseOf(at, Date.parse('2026-09-24T10:04:00Z')), 'closing', 'sixty seconds left is the last minute');
  assert.equal(phaseOf(at, Date.parse('2026-09-24T10:04:59Z')), 'closing');
  assert.equal(phaseOf(at, Date.parse('2026-09-24T10:05:00Z')), 'over', 'zero is over, not a last second');
  assert.equal(phaseOf(at, Date.parse('2026-09-24T10:09:00Z')), 'over');
  assert.equal(phaseOf('soon', now), undefined);
});
