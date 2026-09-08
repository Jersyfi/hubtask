// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Capabilities, TemplateNode } from '@hubtask/sync-engine';

import {
  addUnder,
  childTypesAt,
  childVerdictAt,
  countNodes,
  fieldPathOf,
  fitsInCap,
  nodeCapOf,
  removeAt,
  replaceAt,
  scopeCodeOf,
  walk,
} from './templates.ts';

const node = (title: string, type = 'TASK', children: TemplateNode[] = []) =>
  ({ type, title, children }) as TemplateNode;

/** A three-level tree: a task, a work package under it, an activity under that. */
const TREE = [
  node('Kick off', 'TASK', [node('Plan', 'WORK_PACKAGE', [node('Draft', 'ACTIVITY')])]),
  node('Wrap up', 'TASK'),
];

const manifest = {
  item_types: [
    { type: 'TASK', capabilities: [], allowed_child_types: ['WORK_PACKAGE'], max_depth: 3 },
    { type: 'WORK_PACKAGE', capabilities: [], allowed_child_types: ['ACTIVITY'], max_depth: 3 },
    { type: 'ACTIVITY', capabilities: [], allowed_child_types: [], max_depth: 3 },
  ],
} as unknown as Capabilities;

test('the cap counts every node, not every root', () => {
  assert.equal(countNodes(TREE), 4);
  assert.equal(nodeCapOf({ max_template_nodes: 500 }), 500);
  assert.equal(nodeCapOf({}), undefined);
  assert.equal(fitsInCap(TREE, 4), false);
  assert.equal(fitsInCap(TREE, 5), true);
  assert.equal(fitsInCap(TREE, undefined), true);
});

test('the tree is walked in the order it is drawn, and each node knows where it sits', () => {
  const walked = walk(TREE);
  assert.deepEqual(walked.map((each) => each.node.title), ['Kick off', 'Plan', 'Draft', 'Wrap up']);
  assert.deepEqual(walked.map((each) => each.depth), [0, 1, 2, 0]);
  assert.deepEqual(walked[2]?.path, [0, 0, 0]);
  // The parent's type travels too: what may be added under a node is a question about its parent.
  assert.equal(walked[1]?.parentType, 'TASK');
  assert.equal(walked[0]?.parentType, undefined);
});

test('a node is addressed the way the server addresses it', () => {
  // A field error names `/nodes/0/children/0/title`, and the editor has to put the sentence on
  // that node rather than at the top of the form.
  assert.equal(fieldPathOf([0], 'title'), '/nodes/0/title');
  assert.equal(fieldPathOf([0, 0, 0], 'due_offset'), '/nodes/0/children/0/children/0/due_offset');
});

test('a node is replaced without touching the rest of the shape', () => {
  const next = replaceAt(TREE, [0, 0, 0], node('Redrafted', 'ACTIVITY'));
  assert.equal(walk(next).map((each) => each.node.title).join(','), 'Kick off,Plan,Redrafted,Wrap up');
  // The original is untouched, which is what makes an editor's undo a matter of keeping it.
  assert.equal(walk(TREE)[2]?.node.title, 'Draft');
});

test('a node takes everything under it when it goes', () => {
  assert.deepEqual(walk(removeAt(TREE, [0, 0])).map((each) => each.node.title), ['Kick off', 'Wrap up']);
  assert.deepEqual(walk(removeAt(TREE, [1])).map((each) => each.node.title), ['Kick off', 'Plan', 'Draft']);
});

test('a child is added under a node, or at the root', () => {
  assert.deepEqual(
    walk(addUnder(TREE, [1], node('Follow up', 'WORK_PACKAGE'))).map((each) => each.node.title),
    ['Kick off', 'Plan', 'Draft', 'Wrap up', 'Follow up'],
  );
  assert.equal(countNodes(addUnder(TREE, [], node('Third', 'TASK'))), 5);
});

test('what may be added comes from the manifest, and depth is half the answer', () => {
  assert.deepEqual(childTypesAt(manifest, 'TASK', 0), ['WORK_PACKAGE']);
  // An activity takes no children at all, so the editor offers nothing rather than a refusal.
  assert.deepEqual(childTypesAt(manifest, 'ACTIVITY', 2), []);
  // …and neither does anything at the maximum depth, whatever its type permits.
  assert.deepEqual(childTypesAt(manifest, 'TASK', 3), []);
  assert.equal(childVerdictAt(manifest, 'TASK', 'ACTIVITY', 0).status, 'refused');
  assert.equal(childVerdictAt(undefined, 'TASK', 'WORK_PACKAGE', 0).status, 'undetermined');
});

test('a row says which scope it came from', () => {
  // A collection's list carries three scopes at once; without the mark it is three lists that look
  // like one.
  assert.equal(scopeCodeOf('COLLECTION'), 'app.templates.scope_COLLECTION');
  assert.equal(scopeCodeOf('TENANT'), 'app.templates.scope_TENANT');
});
