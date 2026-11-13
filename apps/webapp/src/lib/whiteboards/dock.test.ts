// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DOCK, afterUse, coverOf, neighbourOf, shortcutOf, toolOfKey } from './dock.ts';

test('is grouped move · create · draw · conversation with the concept shortcuts', () => {
  assert.deepEqual(DOCK.map((g) => g.group), ['move', 'create', 'draw', 'conversation']);
  assert.equal(DOCK.flatMap((g) => g.tools.map((each) => each.key)).join(''), 'VHNRTLFPMEC');
});

test('chooses a tool by its key, never with a modifier', () => {
  assert.equal(toolOfKey('n', false), 'STICKY');
  assert.equal(toolOfKey('H', false), 'HAND');
  assert.equal(toolOfKey('c', true), undefined);
  assert.equal(toolOfKey('x', false), undefined);
  assert.equal(toolOfKey('Enter', false), undefined);
  assert.equal(shortcutOf('COMMENT'), 'C');
});

test('springs back to select after one use unless held', () => {
  assert.equal(afterUse('STICKY', false), 'select');
  assert.equal(afterUse('STICKY', true), 'STICKY');
  assert.equal(afterUse('COMMENT', false), 'select');
  assert.equal(afterUse('STROKE', false), 'STROKE');
  assert.equal(afterUse('HAND', false), 'HAND');
});

test('puts the neighbour beside the note, its centre a gap away', () => {
  assert.deepEqual(neighbourOf({ x: 0, y: 0, width: 160, height: 120 }), { x: 260, y: 60 });
  assert.deepEqual(neighbourOf({ x: 0, y: 0, width: 160, height: 120 }, 'below'), { x: 80, y: 200 });
});

test('measures the share of the surface the controls cover', () => {
  assert.ok(Math.abs(coverOf({ width: 1000, height: 1000 }, [{ width: 200, height: 100 }, { width: 100, height: 100 }]) - 0.03) < 1e-9);
  assert.equal(coverOf({ width: 0, height: 0 }, []), 0);
});
