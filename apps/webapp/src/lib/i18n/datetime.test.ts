// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatDateTime } from './datetime.ts';

test('a moment is written the way the locale writes moments', () => {
  // Not the exact string — that is the platform's and it moves between ICU versions. What is
  // asserted is that the two locales disagree, which is the whole reason this goes through Intl.
  const at = '2026-09-04T15:30:00Z';
  const english = formatDateTime(at, 'en-GB');
  const german = formatDateTime(at, 'de-DE');
  assert.notEqual(english, german);
  assert.match(english, /2026/);
  assert.match(german, /2026/);
});

test('a value this cannot read is shown as it arrived', () => {
  // Never a blank and never `Invalid Date`: the reader sees something they can quote.
  assert.equal(formatDateTime('not a moment', 'en'), 'not a moment');
  assert.equal(formatDateTime('', 'en'), '');
});
