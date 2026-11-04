// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { keyboardTool, sizeOf, tabTarget } from './keys.ts';

const order = ['a', 'b', 'c'];

test('Tab walks the objects and, from the last, leaves the board (WCAG 2.1.2)', () => {
  assert.equal(tabTarget(order, undefined, false), 'a');
  assert.equal(tabTarget(order, 'a', false), 'b');
  assert.equal(tabTarget(order, 'c', false), undefined, 'past the last object focus moves on');
});

test('Shift+Tab walks back and, from the first, leaves the board', () => {
  assert.equal(tabTarget(order, undefined, true), 'c');
  assert.equal(tabTarget(order, 'b', true), 'a');
  assert.equal(tabTarget(order, 'a', true), undefined);
  assert.equal(tabTarget([], undefined, false), undefined, 'an empty board holds nobody');
  assert.equal(tabTarget(order, 'gone', false), 'a', 'a chosen object since removed starts again');
});

test('every tool but the path-following pen and eraser works without a pointer (WCAG 2.1.1)', () => {
  for (const tool of ['STICKY', 'SHAPE', 'TEXT', 'FRAME'] as const) assert.equal(keyboardTool(tool), 'place');
  assert.equal(keyboardTool('LINK'), 'link');
  assert.equal(keyboardTool('CONNECTOR'), 'connect');
  assert.equal(keyboardTool('STROKE'), 'pointer');
  assert.equal(keyboardTool('ERASER'), 'pointer');
  assert.equal(keyboardTool('select'), 'none');
});

test('a size typed in the side panel is whole and never below the handle', () => {
  assert.equal(sizeOf('240.4'), 240);
  assert.equal(sizeOf('3'), 24);
  assert.equal(sizeOf(''), undefined);
  assert.equal(sizeOf('wide'), undefined);
  // Past the board's largest size the scene would be refused (`whiteboards.element_size_invalid`)
  // and no later save of the board would land.
  assert.equal(sizeOf('250000'), 100_000);
  assert.equal(sizeOf('1e400'), undefined);
});
