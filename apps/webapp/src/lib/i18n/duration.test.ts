// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatDuration, parseDuration } from './duration.ts';

test('a duration is written as hours and minutes', () => {
  assert.equal(formatDuration(90), '1:30');
  assert.equal(formatDuration(5), '0:05');
  assert.equal(formatDuration(0), '0:00');
});

test('a duration is read from a clock, from minutes, or from hours in the reader’s notation', () => {
  assert.equal(parseDuration('1:30', ','), 90);
  assert.equal(parseDuration('45', ','), 45);
  assert.equal(parseDuration('1,5', ','), 90);
  assert.equal(parseDuration('1.5', '.'), 90);
  assert.equal(parseDuration('', ','), null);
  // Not a duration: shown as such, never sent.
  assert.equal(parseDuration('1:75', ','), undefined);
  assert.equal(parseDuration('-5', ','), undefined);
  assert.equal(parseDuration('soon', ','), undefined);
  assert.equal(parseDuration(String(11 * 366 * 24 * 60), ','), undefined);
});
