// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The sidebar's tree drag, keys and peek as numbers (the navigation concept's §5.4), the way
// `reorder.test.js` checks a flat list.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  MAX_DEPTH,
  TREE_DRAG_THRESHOLD_PX,
  ancestorsOf,
  applyPlacement,
  autoScrollStep,
  createPeek,
  dropTarget,
  indentOf,
  levelAtInline,
  moveIntent,
  positionOf,
  rowsOf,
  zoneAt,
} from '../src/navtree.ts';

// The mockup's §6.2 tree: Start, Team (open: Onboarding, How we work, Retro board, Holiday), Roadmap board.
const tree = [
  { id: 'start', label: 'Start', part: 'contents' },
  {
    id: 'team',
    label: 'Team',
    part: 'contents',
    children: [
      { id: 'onboarding', label: 'Onboarding handbook', part: 'contents' },
      { id: 'how', label: 'How we work', part: 'contents' },
      { id: 'retro', label: 'Retro sprint 41', part: 'contents', canHoldChildren: false },
      { id: 'holiday', label: 'Holiday rules', part: 'contents' },
    ],
  },
  { id: 'roadmap', label: 'Roadmap workshop', part: 'contents', canHoldChildren: false },
];
const rows = rowsOf(tree, ['team']);
const at = (id) => rows.findIndex((row) => row.node.id === id);
const node = (id) => rows[at(id)].node;

test('a drag starts after 4 px, as the concept says', () => {
  assert.equal(TREE_DRAG_THRESHOLD_PX, 4);
});

test('the upper quarter is before, the lower after, the middle inside where the row takes children', () => {
  assert.equal(zoneAt(5, 30, true), 'before');
  assert.equal(zoneAt(15, 30, true), 'inside');
  assert.equal(zoneAt(25, 30, true), 'after');
  // A whiteboard has no inside: its middle splits at the half.
  assert.equal(zoneAt(12, 30, false), 'before');
  assert.equal(zoneAt(18, 30, false), 'after');
});

test('the indent grows to the sixth level and stops', () => {
  assert.deepEqual([0, 1, 5, 6, 7, 11].map(indentOf), [0, 1, 5, 6, 6, 6]);
  assert.equal(levelAtInline(30, 12), 2);
});

test('before a row is after its previous sibling, with the line at its level', () => {
  const target = dropTarget(rows, node('start'), at('retro'), 'before');
  assert.deepEqual(target, { kind: 'line', afterIndex: at('how'), level: 1, placement: { parentId: 'team', afterId: 'how' } });
});

test('inside a page outlines it; inside a whiteboard is refused with its reason', () => {
  assert.deepEqual(dropTarget(rows, node('start'), at('how'), 'inside'), {
    kind: 'inside',
    index: at('how'),
    placement: { parentId: 'how', afterId: null },
  });
  assert.deepEqual(dropTarget(rows, node('start'), at('retro'), 'inside'), { kind: 'refused', index: at('retro'), refusal: 'no-children' });
});

test('a page is never dropped into itself or its own subtree', () => {
  assert.deepEqual(dropTarget(rows, node('team'), at('how'), 'inside'), { kind: 'refused', index: at('how'), refusal: 'self' });
  assert.deepEqual(dropTarget(rows, node('team'), at('team'), 'inside'), { kind: 'unchanged' });
});

test('a drop where the row already stands changes nothing', () => {
  assert.deepEqual(dropTarget(rows, node('how'), at('onboarding'), 'after'), { kind: 'unchanged' });
  assert.deepEqual(dropTarget(rows, node('how'), at('retro'), 'before'), { kind: 'unchanged' });
});

test('after an open branch is its first child', () => {
  const target = dropTarget(rows, node('roadmap'), at('team'), 'after');
  assert.equal(target.kind, 'line');
  assert.deepEqual(target.placement, { parentId: 'team', afterId: null });
  assert.equal(target.level, 1);
});

test('below the last child, moving to the start outdents', () => {
  const stay = dropTarget(rows, node('start'), at('holiday'), 'after', 5);
  assert.deepEqual(stay.placement, { parentId: 'team', afterId: 'holiday' });
  const out = dropTarget(rows, node('start'), at('holiday'), 'after', 0);
  assert.equal(out.level, 0);
  assert.deepEqual(out.placement, { parentId: null, afterId: 'team' });
  // Not below a last child: the next row is a sibling, so the level stays.
  assert.equal(dropTarget(rows, node('start'), at('how'), 'after', 0).level, 1);
});

test('a drag stays inside its part; an entry lands only on a collection', () => {
  const collection = { id: 'web', label: 'Web app', part: 'collections', acceptsEntries: true };
  const both = rowsOf([collection, ...tree], ['team']);
  assert.deepEqual(dropTarget(both, node('start'), 0, 'after'), { kind: 'refused', index: 0, refusal: 'other-part' });
  const entry = { id: 'e1', label: 'Sign in with a passkey', part: 'entries' };
  assert.deepEqual(dropTarget(both, entry, 0, 'before'), { kind: 'inside', index: 0, placement: { parentId: 'web', afterId: null } });
  assert.equal(dropTarget(both, entry, 1, 'inside').kind, 'refused');
});

test('a move past the twelve levels the server allows is refused', () => {
  let deep = { id: 'leaf', label: 'leaf', part: 'contents' };
  for (let level = MAX_DEPTH - 1; level > 0; level--) deep = { id: `n${level}`, label: 'n', part: 'contents', children: [deep] };
  const chain = rowsOf([deep, { id: 'x', label: 'x', part: 'contents', children: [{ id: 'y', label: 'y', part: 'contents' }] }], []);
  assert.equal(dropTarget(chain, chain[0].node, 1, 'inside').refusal, 'too-deep');
});

test('within 48 px of an edge the region scrolls, faster nearer the edge', () => {
  assert.equal(autoScrollStep(300, 0, 600), 0);
  assert.ok(autoScrollStep(10, 0, 600) < autoScrollStep(40, 0, 600));
  assert.ok(autoScrollStep(40, 0, 600) < 0);
  assert.ok(autoScrollStep(590, 0, 600) > 0);
});

test('Alt and the arrows order, indent and outdent', () => {
  const alt = (key) => ({ key, altKey: true });
  assert.deepEqual(moveIntent(alt('ArrowUp'), tree, 'how'), { placement: { parentId: 'team', afterId: null } });
  assert.deepEqual(moveIntent(alt('ArrowDown'), tree, 'how'), { placement: { parentId: 'team', afterId: 'retro' } });
  assert.equal(moveIntent(alt('ArrowUp'), tree, 'onboarding'), null);
  assert.deepEqual(moveIntent(alt('ArrowRight'), tree, 'how'), { placement: { parentId: 'onboarding', afterId: null } });
  assert.deepEqual(moveIntent(alt('ArrowRight'), tree, 'holiday'), { refusal: 'no-children' });
  assert.deepEqual(moveIntent(alt('ArrowLeft'), tree, 'how'), { placement: { parentId: null, afterId: 'team' } });
  assert.deepEqual(moveIntent(alt('ArrowLeft'), tree, 'how', 'rtl'), { placement: { parentId: 'onboarding', afterId: null } });
  assert.equal(moveIntent({ key: 'ArrowUp', altKey: false }, tree, 'how'), null);
  assert.equal(moveIntent({ key: 'ArrowUp', altKey: true, ctrlKey: true }, tree, 'how'), null);
});

test('the position is announced among the siblings', () => {
  const position = positionOf(tree, 'how');
  assert.equal(position.position, 2);
  assert.equal(position.of, 4);
  assert.equal(position.parent.label, 'Team');
});

test('a placement applied at once moves the node with its subtree', () => {
  const moved = applyPlacement(tree, 'team', { parentId: null, afterId: 'roadmap' });
  assert.deepEqual(moved.map((n) => n.id), ['start', 'roadmap', 'team']);
  assert.equal(moved[2].children.length, 4);
  const under = applyPlacement(tree, 'start', { parentId: 'team', afterId: 'how' });
  assert.deepEqual(under[0].children.map((n) => n.id), ['onboarding', 'how', 'start', 'retro', 'holiday']);
  assert.deepEqual(ancestorsOf(tree, 'retro'), ['team']);
});

test('the peek opens after its delay and closes when the pointer leaves', () => {
  const queue = [];
  const timers = { set: (fn) => (queue.push(fn), queue.length), clear: (handle) => (queue[handle - 1] = null) };
  const seen = [];
  const peek = createPeek(300, (open) => seen.push(open), timers);
  peek.enter();
  assert.equal(peek.isOpen, false);
  queue.shift()?.();
  assert.equal(peek.isOpen, true);
  peek.leave();
  assert.deepEqual(seen, [true, false]);
  // Passing over it opens nothing.
  peek.enter();
  peek.leave();
  for (const fn of queue) fn?.();
  assert.deepEqual(seen, [true, false]);
});
