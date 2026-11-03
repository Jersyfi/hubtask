// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { amountIn, minutesOf, unitFor } from './duration.ts';

test('a held duration is shown in the largest unit it fills exactly', () => {
  assert.equal(unitFor(''), 'MINUTES');
  assert.equal(unitFor('45'), 'MINUTES');
  assert.equal(unitFor('90'), 'MINUTES');
  assert.equal(unitFor('240'), 'HOURS');
  assert.equal(amountIn('240', 'HOURS'), '4');
  assert.equal(amountIn('90', 'MINUTES'), '90');
  assert.equal(amountIn('', 'HOURS'), '');
});

test('what the reader types is converted to the minutes the server holds', () => {
  assert.equal(minutesOf('4', 'HOURS'), '240');
  assert.equal(minutesOf('1.5', 'HOURS'), '90');
  assert.equal(minutesOf('1,5', 'HOURS'), '90', 'a decimal comma is a decimal');
  assert.equal(minutesOf('30', 'MINUTES'), '30');
  assert.equal(minutesOf('', 'HOURS'), '', 'empty stays empty: no goal');
  // Not a number is handed on as typed, so the server's refusal names what was written.
  assert.equal(minutesOf('soon', 'HOURS'), 'soon');
});
