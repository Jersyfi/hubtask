// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { classesBelow, shownClasses } from './classTree.ts';

const tree = [
  { key: 'ci', lineage: ['ci'] },
  { key: 'hardware', lineage: ['hardware', 'ci'] },
  { key: 'server', lineage: ['server', 'hardware', 'ci'] },
  { key: 'virtual', lineage: ['virtual', 'server', 'hardware', 'ci'] },
  { key: 'software', lineage: ['software', 'ci'] },
];

test('the class tree shows the root and its families, each deeper branch folded until opened (§5.4)', () => {
  assert.deepEqual(shownClasses(tree, new Set()).map((each) => each.key), ['ci', 'hardware', 'software']);
  assert.deepEqual(shownClasses(tree, new Set(['hardware'])).map((each) => each.key), ['ci', 'hardware', 'server', 'software']);
  assert.deepEqual(shownClasses(tree, new Set(['hardware', 'server'])).map((each) => each.key), ['ci', 'hardware', 'server', 'virtual', 'software']);
  assert.deepEqual(shownClasses(tree, new Set(['server'])).map((each) => each.key), ['ci', 'hardware', 'software'], 'a branch under a folded one stays folded');
});

test('each class counts the classes below it at any depth', () => {
  const below = classesBelow(tree);
  assert.equal(below.get('ci'), 4);
  assert.equal(below.get('hardware'), 2);
  assert.equal(below.get('server'), 1);
  assert.equal(below.get('software'), undefined);
});
