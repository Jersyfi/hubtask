// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel
import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Element } from '@hubtask/whiteboard';

import { listOfBoard } from './list.ts';

const at = (id: string, kind: Element['kind'], x: number, y: number, width = 100, height = 100): Element => ({ id, kind, x, y, width, height });

test('reads each frame in reading order, then the loose objects, and leaves arrows out', () => {
  const frames = [at('right', 'FRAME', 600, 0, 500, 500), at('left', 'FRAME', 0, 0, 500, 500)];
  const elements = [
    ...frames,
    at('b', 'STICKY', 200, 10), at('a', 'STICKY', 20, 30), at('c', 'STICKY', 20, 200),
    at('d', 'CARD', 700, 50), at('arrow', 'CONNECTOR', 0, 0, 0, 0), at('loose', 'TEXT', 50, 900),
  ];
  const groups = listOfBoard(elements, frames);
  assert.deepEqual(groups.map((g) => g.frame?.id), ['left', 'right', undefined]);
  assert.deepEqual(groups.map((g) => g.items.map((e) => e.id)), [['a', 'b', 'c'], ['d'], ['loose']]);
});
