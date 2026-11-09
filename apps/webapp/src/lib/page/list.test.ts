// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { countOf, matchesChoice, matchesSearch, rowsPerPageOf } from './list.ts';

test('an empty search matches every row', () => {
  assert.equal(matchesSearch('  ', ['Lukas Brandt']), true);
});

test('a search ignores case and accents and needs every word somewhere in the row', () => {
  assert.equal(matchesSearch('yilmaz', ['Mehmet Yılmaz'.replace('ı', 'i')]), true);
  assert.equal(matchesSearch('KRÜGER jana', ['Jana Kruger', 'owner@example.org']), true);
  assert.equal(matchesSearch('jana brandt', ['Jana Krüger']), false);
  assert.equal(matchesSearch('example', [undefined, 'owner@example.org']), true);
});

test('a chip with nothing chosen narrows nothing', () => {
  assert.equal(matchesChoice(undefined, 'ADMIN'), true);
  assert.equal(matchesChoice('ADMIN', 'ADMIN'), true);
  assert.equal(matchesChoice('ADMIN', 'MEMBER'), false);
});

test('a list starts at 25 rows unless the device kept 50 or 100', () => {
  assert.equal(rowsPerPageOf(null), 25);
  assert.equal(rowsPerPageOf('50'), 50);
  assert.equal(rowsPerPageOf('100'), 100);
  assert.equal(rowsPerPageOf('37'), 25);
});

test('the foot counts the endpoint total only while nothing narrows the rows', () => {
  assert.equal(countOf(50, 1240, false), 1240);
  assert.equal(countOf(50, 1240, true), 50);
  assert.equal(countOf(12, undefined, false), 12);
  // A row written after the count does not make the foot say fewer than it shows.
  assert.equal(countOf(51, 50, false), 51);
});
