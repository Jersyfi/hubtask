// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { followedViewport, peersOf, stillFollowing } from './peers.ts';

const states = new Map<number, unknown>([
  [1, { name: 'Me', cursor: { x: 0, y: 0 } }],
  [2, { name: 'Ada', cursor: { x: 10, y: 20 }, view: { x: 500, y: 300, zoom: 2 } }],
  [3, { cursor: { x: 5, y: 5 } }],
  [4, null],
]);

test('UC-CNV-05 check 3: the others are listed with their names and cursors, never the reader', () => {
  const peers = peersOf(states, 1, 'Someone');
  assert.deepEqual(peers.map((p) => p.name), ['Ada', 'Someone']);
  assert.deepEqual(peers[0]?.cursor, { x: 10, y: 20 });
});

test('UC-CNV-05 check 3: following centres on the person\'s view until they leave', () => {
  const peers = peersOf(states, 1, 'Someone');
  assert.deepEqual(followedViewport(peers, 2, { width: 800, height: 400 }), { zoom: 2, x: 300, y: 200 });
  assert.equal(followedViewport(peers, 3, { width: 800, height: 400 }), undefined, 'no view said yet, nothing moves');
  assert.equal(stillFollowing(peers, 2), 2);
  assert.equal(stillFollowing(peersOf(new Map([[1, {}]]), 1, 'Someone'), 2), undefined, 'a person who left is followed no more');
});
