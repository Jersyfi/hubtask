// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { classPath, classesBelow, classesInOrder, classesUnder, shownClasses, typesJoining } from './classTree.ts';

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

const listed = [
  { key: 'software', parent: 'ci', lineage: ['software', 'ci'] },
  { key: 'server', parent: 'hardware', lineage: ['server', 'hardware', 'ci'] },
  { key: 'ci', parent: '', lineage: ['ci'] },
  { key: 'hardware', parent: 'ci', lineage: ['hardware', 'ci'] },
];

test('the classes read in hierarchy order, each after its parent, siblings as listed', () => {
  assert.deepEqual(classesInOrder(listed).map((each) => each.key), ['ci', 'software', 'hardware', 'server']);
});

test('a class page lists the classes directly beneath it, not deeper ones', () => {
  assert.deepEqual(classesUnder(listed, 'ci').map((each) => each.key), ['software', 'hardware']);
  assert.deepEqual(classesUnder(listed, 'server'), []);
});

test('a class is used by the relation types that join it or a class above it, at either end', () => {
  const types = [
    { key: 'runs_on', pairs: [{ from: 'software', to: 'hardware' }] },
    { key: 'depends_on', pairs: [{ from: 'ci', to: 'ci' }] },
    { key: 'hosts', pairs: [{ from: 'server', to: 'software' }] },
  ];
  assert.deepEqual(typesJoining(types, ['server', 'hardware', 'ci']).map((each) => each.key), ['runs_on', 'depends_on', 'hosts']);
  assert.deepEqual(typesJoining(types, ['software', 'ci']).map((each) => each.key), ['runs_on', 'depends_on', 'hosts']);
  assert.deepEqual(typesJoining(types.slice(0, 1), ['other']).map((each) => each.key), []);
});

test('a class page has an address of its own, its key escaped', () => {
  assert.equal(classPath('server'), '/assets/classes/server');
  assert.equal(classPath('a b'), '/assets/classes/a%20b');
});
