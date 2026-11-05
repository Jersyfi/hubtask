// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import test from 'node:test';

import * as Y from 'yjs';

import { Facilitation } from '../src/index.ts';

/** Two devices' documents that exchange what each wrote only when synced - offline until then. */
function apart(): { a: Y.Doc; b: Y.Doc; sync: () => void } {
  const a = new Y.Doc();
  const b = new Y.Doc();
  return {
    a,
    b,
    sync: () => {
      Y.applyUpdate(b, Y.encodeStateAsUpdate(a, Y.encodeStateVector(b)));
      Y.applyUpdate(a, Y.encodeStateAsUpdate(b, Y.encodeStateVector(a)));
    },
  };
}

test('one voter on two devices at the same instant keeps both votes', () => {
  const { a, b, sync } = apart();
  new Facilitation(a).startVoting({ id: 's1', perPerson: 3, anonymous: false, startedBy: 'ana' });
  sync();
  assert.equal(new Facilitation(a).vote('ana', 'sticky-1'), true);
  assert.equal(new Facilitation(b).vote('ana', 'sticky-2'), true);
  sync();
  const seen = new Facilitation(a);
  assert.deepEqual([...seen.votesOf('ana')].sort(), ['sticky-1', 'sticky-2']);
  assert.deepEqual([...seen.results().counts].sort(), [['sticky-1', 1], ['sticky-2', 1]]);
  assert.equal(seen.results().voters, 1);
});

test('votes given at once on two devices count no more than the voter has', () => {
  const { a, b, sync } = apart();
  new Facilitation(a).startVoting({ id: 's1', perPerson: 2, anonymous: false, startedBy: 'ana' });
  sync();
  new Facilitation(a).vote('ana', 'x');
  new Facilitation(a).vote('ana', 'x');
  new Facilitation(b).vote('ana', 'y');
  sync();
  const total = [...new Facilitation(a).results().counts.values()].reduce((sum, n) => sum + n, 0);
  assert.equal(total, 2);
  assert.deepEqual([...new Facilitation(b).results().counts], [...new Facilitation(a).results().counts]);
});

test('a vote taken back is one vote, and a session started again clears them', () => {
  const doc = new Y.Doc();
  const session = new Facilitation(doc);
  session.startVoting({ id: 's1', perPerson: 3, anonymous: true, startedBy: 'ana' });
  session.vote('device-key', 'x');
  session.vote('device-key', 'x');
  assert.equal(session.unvote('device-key', 'x'), true);
  assert.deepEqual(session.votesOf('device-key'), ['x']);
  session.startVoting({ id: 's2', perPerson: 3, anonymous: true, startedBy: 'ana' });
  assert.deepEqual(session.votesOf('device-key'), []);
  assert.equal(session.results().voters, 0);
});

test('votes a board kept as one list per voter are still read and taken back', () => {
  const doc = new Y.Doc();
  const session = new Facilitation(doc);
  session.startVoting({ id: 's1', perPerson: 3, anonymous: false, startedBy: 'ana' });
  doc.getMap<string[]>('votes').set('s1:ben', ['x', 'y']);
  assert.deepEqual([...session.votesOf('ben')].sort(), ['x', 'y']);
  assert.equal(session.vote('ben', 'z'), true);
  assert.equal(session.vote('ben', 'w'), false);
  assert.equal(session.unvote('ben', 'x'), true);
  assert.deepEqual([...session.votesOf('ben')].sort(), ['y', 'z']);
});
