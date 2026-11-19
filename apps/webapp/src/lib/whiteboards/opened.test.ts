// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MAX_OPENED, boardsOpened, boardsToContinue, forgetBoardsOpened, noteBoardOpened } from './opened.ts';

function store() {
  const held = new Map<string, string>();
  return { getItem: (key: string) => held.get(key) ?? null, setItem: (key: string, value: string) => void held.set(key, value), removeItem: (key: string) => void held.delete(key) };
}

test('the boards opened on this device are kept per account, the last opened first, and end with the session', () => {
  const kept = store();
  noteBoardOpened('a', 'b1', 1, kept);
  noteBoardOpened('a', 'b2', 2, kept);
  noteBoardOpened('a', 'b1', 3, kept);
  noteBoardOpened('b', 'b9', 4, kept);
  assert.deepEqual(boardsOpened('a', kept), ['b1', 'b2']);
  assert.deepEqual(boardsOpened('b', kept), ['b9'], 'another account reads its own');
  assert.deepEqual(boardsOpened(undefined, kept), []);
  forgetBoardsOpened('a', kept);
  assert.deepEqual(boardsOpened('a', kept), []);
});

test('the boards opened are capped, the oldest let go', () => {
  const kept = store();
  for (let at = 0; at < MAX_OPENED + 3; at += 1) noteBoardOpened('a', `b${at}`, at, kept);
  const opened = boardsOpened('a', kept);
  assert.equal(opened.length, MAX_OPENED);
  assert.equal(opened[0], `b${MAX_OPENED + 2}`);
  assert.ok(!opened.includes('b0'));
});

test('the home continues the boards opened here, still readable, else the last changed', () => {
  const all = [{ id: 'x' }, { id: 'y' }, { id: 'z' }, { id: 'w' }];
  const recent = [{ id: 'w' }, { id: 'z' }, { id: 'y' }, { id: 'x' }];
  assert.deepEqual(boardsToContinue(['y', 'gone', 'x'], all, recent, 3), { boards: [{ id: 'y' }, { id: 'x' }], opened: true });
  assert.deepEqual(boardsToContinue(['gone'], all, recent, 3), { boards: [{ id: 'w' }, { id: 'z' }, { id: 'y' }], opened: false });
  assert.deepEqual(boardsToContinue([], all, recent, 2).boards.map((b) => b.id), ['w', 'z']);
});
