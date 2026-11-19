// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MAX_READ, forgetPagesRead, notePageRead, pagesRead } from './reading.ts';

function store() {
  const held = new Map<string, string>();
  return { getItem: (key: string) => held.get(key) ?? null, setItem: (key: string, value: string) => void held.set(key, value), removeItem: (key: string) => void held.delete(key) };
}

test('the pages read on this device are kept per account, the last opened first, and end with the session', () => {
  const kept = store();
  notePageRead('a', 'p1', 1, kept);
  notePageRead('a', 'p2', 2, kept);
  notePageRead('a', 'p1', 3, kept);
  notePageRead('b', 'p9', 4, kept);
  assert.deepEqual(pagesRead('a', kept), ['p1', 'p2']);
  assert.deepEqual(pagesRead('b', kept), ['p9'], 'another account reads its own');
  assert.deepEqual(pagesRead(undefined, kept), []);
  forgetPagesRead('a', kept);
  assert.deepEqual(pagesRead('a', kept), []);
});

test('the pages read are capped, the oldest let go', () => {
  const kept = store();
  for (let at = 0; at < MAX_READ + 3; at += 1) notePageRead('a', `p${at}`, at, kept);
  const read = pagesRead('a', kept);
  assert.equal(read.length, MAX_READ);
  assert.equal(read[0], `p${MAX_READ + 2}`);
  assert.ok(!read.includes('p0'));
});
