// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { marked, readMarks, unmarked } from './newmark.ts';

test('a mark is set once and taken away by a visit', () => {
  const once = marked(marked([], 'WHITEBOARD'), 'WHITEBOARD');
  assert.deepEqual(once, ['WHITEBOARD']);
  assert.deepEqual(unmarked(marked(once, 'TIME'), 'WHITEBOARD'), ['TIME']);
});

test('what the device kept is read leniently: anything but a list of names is no mark', () => {
  assert.deepEqual(readMarks(null), []);
  assert.deepEqual(readMarks('not json'), []);
  assert.deepEqual(readMarks('{"WHITEBOARD":true}'), []);
  assert.deepEqual(readMarks('["TIME", 3, null]'), ['TIME']);
});
