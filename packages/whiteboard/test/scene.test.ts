// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import * as Y from 'yjs';

import { Board, MAX_ELEMENTS, type Element } from '../src/index.ts';

const sticky = (id: string, x = 0): Element => ({ id, kind: 'STICKY', x, y: 0, width: 160, height: 160, text: id, color_token: 'amber' });

/** Two devices' documents kept in step, as the live document's connection does. */
function pair(): [Board, Board] {
  const a = new Y.Doc();
  const b = new Y.Doc();
  a.on('update', (update: Uint8Array) => Y.applyUpdate(b, update, 'remote'));
  b.on('update', (update: Uint8Array, origin: unknown) => {
    if (origin !== 'remote') Y.applyUpdate(a, update, 'remote');
  });
  return [new Board(a), new Board(b)];
}

test('objects are drawn in the order they were put on the board, and to the front on asking (UC-CNV-01 check 1)', () => {
  const board = new Board(new Y.Doc());
  board.add(sticky('a'));
  board.add({ id: 'b', kind: 'SHAPE', shape: 'DIAMOND', x: 300, y: 0, width: 120, height: 120 });
  board.add({ id: 'l', kind: 'CONNECTOR', from: 'a', to: 'b', route: 'ELBOW', end: 'ARROW', x: 0, y: 0, width: 0, height: 0 });
  assert.deepEqual(board.elements().map((e) => e.id), ['a', 'b', 'l']);
  board.front('a');
  assert.deepEqual(board.scene().map((e) => e.id), ['b', 'l', 'a']);
  assert.equal(board.get('b')?.shape, 'DIAMOND');
  assert.equal(board.get('l')?.from, 'a');
});

test('two people drawing at the same time both keep their objects and their changes (UC-CNV-01 check 4)', () => {
  const [anna, bert] = pair();
  anna.add(sticky('a'));
  bert.add(sticky('b', 300));
  anna.update('b', { text: 'Anna wrote here' });
  bert.move(['a'], 40, 0);
  for (const board of [anna, bert]) {
    assert.deepEqual(board.elements().map((e) => e.id).sort(), ['a', 'b']);
    assert.equal(board.get('a')?.x, 40);
    assert.equal(board.get('b')?.text, 'Anna wrote here');
  }
});

test('taking an object off the board takes the connectors attached to it; a locked one does not move', () => {
  const board = new Board(new Y.Doc());
  board.add(sticky('a'));
  board.add({ ...sticky('b', 300), locked: true });
  board.add({ id: 'l', kind: 'CONNECTOR', from: 'a', to: 'b', x: 0, y: 0, width: 0, height: 0 });
  board.move(['a', 'b'], 10, 10);
  assert.equal(board.get('a')?.x, 10);
  assert.equal(board.get('b')?.x, 300);
  board.remove(['a']);
  assert.deepEqual(board.elements().map((e) => e.id), ['b']);
});

test('a sticky note becomes the card of its entry in its place, its connectors still attached (UC-CNV-02 check 1)', () => {
  const board = new Board(new Y.Doc());
  board.add(sticky('a', 50));
  board.add(sticky('b', 400));
  board.add({ id: 'l', kind: 'CONNECTOR', from: 'a', to: 'b', x: 0, y: 0, width: 0, height: 0 });
  assert.equal(board.toCard('a', '0192f000-0000-7000-8000-000000000001'), true);
  const card = board.get('a');
  assert.equal(card?.kind, 'CARD');
  assert.deepEqual(card?.target, { kind: 'ENTRY', id: '0192f000-0000-7000-8000-000000000001' });
  assert.equal(card?.x, 50);
  assert.equal(card?.text, undefined);
  assert.equal(board.get('l')?.from, 'a');
  assert.equal(board.toCard('l', 'x'), false, 'only a sticky note becomes a card');
});

test('a saved scene seeds an empty document only, and the board stops at its limit (UC-CNV-01 check 5)', () => {
  const board = new Board(new Y.Doc());
  assert.equal(board.seed([sticky('a'), sticky('b')]), true);
  assert.equal(board.seed([sticky('c')]), false);
  assert.deepEqual(board.scene().map((e) => e.id), ['a', 'b']);
  const full = new Board(new Y.Doc());
  full.seed(Array.from({ length: MAX_ELEMENTS }, (_, i) => sticky(`e${i}`)));
  assert.equal(full.add(sticky('one-more')), false);
  assert.equal(full.size, MAX_ELEMENTS);
});
