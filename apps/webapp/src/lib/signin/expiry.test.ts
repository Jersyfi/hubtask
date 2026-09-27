// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { remaining } from './expiry.ts';

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
