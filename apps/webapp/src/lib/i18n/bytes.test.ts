// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatBytes } from './bytes.ts';

test('a size climbs the units at a thousand, not at a kibibyte', () => {
  // The unit name is Intl's and means a power of ten. A client that divided by 1024 under the
  // name "kB" would be disagreeing with its own label, and with the file managers the reader
  // compares against.
  assert.match(formatBytes(999, 'en'), /999/);
  assert.match(formatBytes(1_000, 'en'), /^1 kB$/);
  assert.match(formatBytes(2_400_000, 'en'), /^2\.4 MB$/);
});

test('bytes are whole and the units above them get one decimal', () => {
  assert.match(formatBytes(3, 'en'), /^3 byte/);
  assert.match(formatBytes(1_536, 'en'), /^1\.5 kB$/);
});

test('the separator is the reader’s', () => {
  // The whole reason this goes through Intl: German writes the decimal comma, and a hard-coded
  // dot would be one more sentence this client wrote itself.
  assert.notEqual(formatBytes(1_536, 'de'), formatBytes(1_536, 'en'));
});

test('a size that is not one is shown as it arrived rather than as NaN', () => {
  assert.equal(formatBytes(Number.NaN, 'en'), 'NaN');
  assert.equal(formatBytes(-1, 'en'), '-1');
});
