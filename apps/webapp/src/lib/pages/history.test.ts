// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { compareBodies } from '@hubtask/editor';

import { changeRuns, sideBySide } from './history.ts';

const lines = compareBodies(['# Overview', 'Public API v2', 'Teams', 'Old rule'].join('\n'), ['# Overview', 'Public API v3', 'Teams', 'Passkeys first', 'Buffered orders'].join('\n'));

test('two versions stand side by side, the older one with what went out, the page with what came in', () => {
  const { before, after } = sideBySide(lines);
  assert.deepEqual(before.map((line) => line.text), ['# Overview', 'Public API v2', 'Teams', 'Old rule']);
  assert.deepEqual(after.map((line) => line.text), ['# Overview', 'Public API v3', 'Teams', 'Passkeys first', 'Buffered orders']);
  assert.deepEqual(before.filter((line) => line.kind === 'removed').map((line) => line.text), ['Public API v2', 'Old rule']);
});

test('two versions count each place that changed once, lines out and in together', () => {
  const runs = changeRuns(lines);
  assert.equal(runs.length, 2);
  assert.deepEqual(runs[1]!.map((line) => `${line.kind}:${line.text}`), ['removed:Old rule', 'added:Passkeys first', 'added:Buffered orders']);
  assert.deepEqual(changeRuns(compareBodies('same', 'same')), []);
});
