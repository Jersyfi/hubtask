// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic behind Breadcrumb and SideNav, checked without a browser.
//
// The same bargain focus.test.js and layers.test.js make: the questions worth getting right are
// questions about a list - which crumb is in the middle, which row is this one's parent when three
// of five branches are collapsed - and a component that answered them inline could only be checked
// by opening one.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { collapseTrail, flattenTree, parentRow, treeIntent } from '../src/structure.ts';

const crumbs = (...ids) => ids.map((id) => ({ id, label: id }));
const shownIds = (result) => result.shown.map((entry) => entry.crumb.id);

// --- the trail --------------------------------------------------------------------------------

test('the five levels collapse to hub, parent and current', () => {
  const trail = crumbs('hub', 'collection', 'task', 'package', 'activity');
  const result = collapseTrail(trail);

  // design-system.md §4's `Hub / … / Parent / Current`, and which three is not arbitrary: the
  // first says which workspace, the last two say what this is and what holds it.
  assert.deepEqual(shownIds(result), ['hub', 'package', 'activity']);
  assert.equal(result.hidden, 2);
});

test('a shown crumb keeps its position in the whole trail', () => {
  const trail = crumbs('hub', 'collection', 'task', 'package', 'activity');
  // The index is what the component draws the ellipsis before. If it were the position in the
  // *shown* list, the gap would appear in the wrong place the moment anything collapsed.
  assert.deepEqual(collapseTrail(trail).shown.map((entry) => entry.index), [0, 3, 4]);
});

test('three levels or fewer never collapse', () => {
  for (const size of [0, 1, 2, 3]) {
    const trail = crumbs(...Array.from({ length: size }, (_, index) => `c${index}`));
    const result = collapseTrail(trail);
    assert.equal(result.hidden, 0, `a trail of ${size} hid something`);
    assert.equal(result.shown.length, size);
  }
});

test('four is the shortest trail with a middle to hide', () => {
  // One below the threshold the rule needs: hiding a single level would cost a control to save a
  // word, so the ellipsis appears at four and not at three.
  assert.equal(collapseTrail(crumbs('a', 'b', 'c', 'd')).hidden, 1);
});

test('expanded shows everything, and hides nothing behind a control', () => {
  const trail = crumbs('hub', 'collection', 'task', 'package', 'activity');
  const result = collapseTrail(trail, true);
  assert.deepEqual(shownIds(result), ['hub', 'collection', 'task', 'package', 'activity']);
  assert.equal(result.hidden, 0);
});

// --- the tree ---------------------------------------------------------------------------------

const tree = [
  {
    id: 'private',
    label: 'Private',
    children: [
      { id: 'shopping', label: 'Shopping' },
      { id: 'renovation', label: 'Renovation' },
    ],
  },
  { id: 'work', label: 'Work', children: [{ id: 'hubtask', label: 'Hubtask' }] },
  { id: 'jumble', label: 'Jumble' },
];

const ids = (rows) => rows.map((row) => row.node.id);

test('a collapsed branch contributes its own row and nothing under it', () => {
  assert.deepEqual(ids(flattenTree(tree, [])), ['private', 'work', 'jumble']);
});

test('an expanded branch contributes its children in reading order', () => {
  assert.deepEqual(ids(flattenTree(tree, ['private'])), [
    'private', 'shopping', 'renovation', 'work', 'jumble',
  ]);
});

test('depth is the level, not the position', () => {
  const rows = flattenTree(tree, ['private', 'work']);
  assert.deepEqual(rows.map((row) => row.depth), [0, 1, 1, 0, 1, 0]);
});

test('a branch with no children is a leaf, whatever the expanded list says', () => {
  // `jumble` has no children, so naming it expanded must not make it a branch. Otherwise the twist
  // appears on a node with nothing to reveal and the right arrow does nothing.
  const [, , jumble] = flattenTree(tree, ['jumble']);
  assert.equal(jumble.isBranch, false);
  assert.equal(jumble.isExpanded, false);
});

test('the parent is the nearest row above at a smaller depth', () => {
  const rows = flattenTree(tree, ['private']);
  assert.equal(parentRow(rows, 2), 0, 'renovation should belong to private');
  assert.equal(parentRow(rows, 0), null, 'a top-level node has no parent');
});

// --- what a key means -------------------------------------------------------------------------

test('the right arrow opens a closed branch and then walks into it', () => {
  assert.deepEqual(treeIntent('ArrowRight', flattenTree(tree, []), 0), { kind: 'expand' });
  assert.deepEqual(treeIntent('ArrowRight', flattenTree(tree, ['private']), 0), {
    kind: 'focus', index: 1,
  });
});

test('the left arrow closes an open branch and then goes up to the parent', () => {
  const open = flattenTree(tree, ['private']);
  assert.deepEqual(treeIntent('ArrowLeft', open, 0), { kind: 'collapse' });
  assert.deepEqual(treeIntent('ArrowLeft', open, 2), { kind: 'focus', index: 0 });
});

test('in RTL the arrow towards the children is the one pointing left', () => {
  const closed = flattenTree(tree, []);
  // The whole reason the direction is a parameter: a tree that read `ArrowRight` as "open" in
  // Arabic would open on the key that points away from where the children are drawn.
  assert.deepEqual(treeIntent('ArrowLeft', closed, 0, 'rtl'), { kind: 'expand' });
  assert.equal(treeIntent('ArrowRight', closed, 0, 'rtl'), null);
});

test('a tree does not wrap, unlike a menu', () => {
  const rows = flattenTree(tree, []);
  // A list has a shape, and running off the end of it loses the reader's place in that shape.
  assert.equal(treeIntent('ArrowUp', rows, 0), null);
  assert.equal(treeIntent('ArrowDown', rows, rows.length - 1), null);
});

test('Home and End reach the ends of the visible list, not of the tree', () => {
  const rows = flattenTree(tree, ['private']);
  assert.deepEqual(treeIntent('Home', rows, 3), { kind: 'focus', index: 0 });
  // The last *visible* row. With `work` collapsed, `hubtask` is not one of them.
  assert.deepEqual(treeIntent('End', rows, 0), { kind: 'focus', index: rows.length - 1 });
  assert.equal(ids(rows).at(-1), 'jumble');
});

test('a key the tree does not answer is left alone', () => {
  // The component calls `preventDefault` only when an intent comes back, so a null here is what
  // keeps `Tab` and typing from being swallowed.
  assert.equal(treeIntent('Tab', flattenTree(tree, []), 0), null);
  assert.equal(treeIntent('a', flattenTree(tree, []), 0), null);
});

test('a node may declare itself a branch before its children are loaded', () => {
  // A level fetched on demand: the hub is open, its collections are still on the way, and it must
  // not stop being a branch in the meantime — a node that collapsed itself under the reader would
  // take the twist away at the moment they pressed it.
  const pending = [{ id: 'hub', label: 'Private', isBranch: true, children: [] }];
  assert.equal(flattenTree(pending, ['hub'])[0].isBranch, true);
  assert.deepEqual(treeIntent('ArrowLeft', flattenTree(pending, ['hub']), 0), { kind: 'collapse' });

  // And the ordinary case is unchanged: children still answer it when nobody says otherwise.
  assert.equal(flattenTree([{ id: 'leaf', label: 'Jumble' }], [])[0].isBranch, false);
});
