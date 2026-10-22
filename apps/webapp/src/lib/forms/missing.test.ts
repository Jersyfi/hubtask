// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { check, isComplete, isMet, without } from './missing.ts';

test('a field is missing when it holds nothing', () => {
  assert.equal(isMet({ kind: 'field', key: 'name', code: 'app.x.name_required', isFilled: false }), false);
  assert.equal(isMet({ kind: 'field', key: 'name', code: 'app.x.name_required', isFilled: true }), true);
});

test('a group needs at least one chosen', () => {
  assert.equal(isMet({ kind: 'at-least-one', key: 'scopes', code: 'c', chosen: 0 }), false);
  assert.equal(isMet({ kind: 'at-least-one', key: 'scopes', code: 'c', chosen: 2 }), true);
});

test('an either/or is met by exactly one way', () => {
  // A reminder counted from a due date the entry lacks is no way at all; a fixed time is the way out.
  assert.equal(isMet({ kind: 'one-of', key: 'when', code: 'c', ways: [false, false] }), false);
  assert.equal(isMet({ kind: 'one-of', key: 'when', code: 'c', ways: [false, true] }), true);
  assert.equal(isMet({ kind: 'one-of', key: 'when', code: 'c', ways: [true, true] }), false);
});

test('what is missing comes back in the order it was listed, with its code', () => {
  const missing = check([
    { kind: 'field', key: 'name', code: 'name_required', isFilled: false },
    { kind: 'field', key: 'limit', code: 'limit_required', isFilled: true },
    { kind: 'at-least-one', key: 'types', code: 'types_required', chosen: 0 },
  ]);
  assert.deepEqual(Object.entries(missing), [
    ['name', 'name_required'],
    ['types', 'types_required'],
  ]);
  assert.equal(isComplete(missing), false);
});

test('nothing missing is a form that may be sent', () => {
  assert.equal(isComplete(check([{ kind: 'field', key: 'name', code: 'c', isFilled: true }])), true);
  assert.equal(isComplete(check([])), true);
});

test('changing a marked field takes only its own mark away', () => {
  const missing = check([
    { kind: 'field', key: 'name', code: 'a', isFilled: false },
    { kind: 'field', key: 'file', code: 'b', isFilled: false },
  ]);
  assert.deepEqual(without(missing, 'name'), { file: 'b' });
  assert.equal(without(missing, 'other'), missing);
});
