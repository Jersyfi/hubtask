// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MAX_STARS, isStarred, toggled } from './stars.ts';

test('starring appends to the person\'s order, starring again takes it out (§6.2)', () => {
  const one = toggled([], 'HUB', 'h')!;
  assert.deepEqual(one, [{ kind: 'HUB', id: 'h' }]);
  const two = toggled(one, 'PAGE', 'p')!;
  assert.deepEqual(two.map((star) => star.id), ['h', 'p']);
  assert.ok(isStarred(two, 'PAGE', 'p'));
  assert.ok(!isStarred(two, 'COLLECTION', 'p'), 'a star is its kind and its id');
  assert.deepEqual(toggled(two, 'HUB', 'h'), [{ kind: 'PAGE', id: 'p' }]);
});

test('nothing is starred past the bound the server keeps', () => {
  const full = Array.from({ length: MAX_STARS }, (_, i) => ({ kind: 'PAGE' as const, id: `p${i}` }));
  assert.equal(toggled(full, 'HUB', 'h'), undefined);
  assert.equal(toggled(full, 'PAGE', 'p0')?.length, MAX_STARS - 1);
});
