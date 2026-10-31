// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// UC-SVC-03 check 4: who else has the entry open, and who is writing a reply.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { othersHere, presencePath } from './presence.ts';

test('the reader is never among the others, in this tab or another', () => {
  const states = new Map<number, unknown>([
    [1, { account_id: 'me', name: 'Me' }],
    [2, { account_id: 'me', name: 'Me', replying: true }],
    [3, { account_id: 'jonas', name: 'Jonas' }],
  ]);
  assert.deepEqual(othersHere(states, 1, 'me'), [{ accountId: 'jonas', name: 'Jonas', replying: false }]);
});

test('one person in two tabs is one row, replying when either tab is, and the repliers come first', () => {
  const states = new Map<number, unknown>([
    [2, { account_id: 'ada', name: 'Ada' }],
    [3, { account_id: 'jonas', name: 'Jonas' }],
    [4, { account_id: 'jonas', replying: true }],
  ]);
  assert.deepEqual(othersHere(states, 1, 'me'), [
    { accountId: 'jonas', name: 'Jonas', replying: true },
    { accountId: 'ada', name: 'Ada', replying: false },
  ]);
});

test('a connection without an account still counts, and an empty state does not', () => {
  const states = new Map<number, unknown>([[5, { name: 'Somebody' }], [6, null], [7, 'nonsense']]);
  assert.deepEqual(othersHere(states, 1, 'me'), [{ accountId: undefined, name: 'Somebody', replying: false }]);
});

test('the presence document is the entry kind of the live documents', () => {
  assert.equal(presencePath('a/b'), '/realtime/documents/entry/a%2Fb');
});
