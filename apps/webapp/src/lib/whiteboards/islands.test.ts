// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { isLevelWithIslands } from './islands.ts';

test('the session bar stands level with the top islands where it clears both, else below them (W-5)', () => {
  const cases: [string, number, number, number[], boolean][] = [
    ['a timer between a short title and the faces', 1336, 244, [420, 250], true],
    ['timer and vote beside a long title', 1175, 540, [420, 250], false],
    ['the wider island decides, either side', 1000, 244, [200, 400], false],
    ['exactly the room', 1000, 200, [368, 100], true],
    ['not yet measured', 1000, 0, [900, 900], true],
  ];
  for (const [what, stage, bar, islands, level] of cases) assert.equal(isLevelWithIslands(stage, bar, islands, 16), level, what);
});
