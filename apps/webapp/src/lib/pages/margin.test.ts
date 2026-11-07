// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-DOC-11 check 4: a thread is drawn level with its passage; two passages close together keep
// their order without one thread covering the other, and a thread whose words are gone follows.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { levelThreads } from './margin.ts';

test('each thread is drawn level with its passage', () => {
  const layout = levelThreads([
    { id: 'late', top: 400, height: 80 },
    { id: 'early', top: 40, height: 80 },
  ], 8);
  assert.equal(layout.tops.get('early'), 40);
  assert.equal(layout.tops.get('late'), 400);
  assert.equal(layout.height, 480);
});

test('a thread is pushed below the one above instead of covering it', () => {
  const layout = levelThreads([
    { id: 'a', top: 100, height: 120 },
    { id: 'b', top: 130, height: 60 },
    { id: 'gone', top: undefined, height: 50 },
  ], 10);
  assert.equal(layout.tops.get('a'), 100);
  assert.equal(layout.tops.get('b'), 230);
  assert.equal(layout.tops.get('gone'), 300);
});

test('no thread is drawn above the margin', () => {
  assert.equal(levelThreads([{ id: 'a', top: -30, height: 10 }], 4).tops.get('a'), 0);
});
