// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one thing about a restore worth asserting without a server: which modes this client can
// compose at all, and which of them needs all three doors.
//
// `INSTANCE` and `NEW_TENANT` cross or create a tenant, which is the installation operator's
// business rather than one workspace's. They are absent from the type rather than filtered at the
// end, so a screen cannot compose one by accident — and this test is what says so out loud.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { isDestructive, mayRunForReal, MODES, offeredModes, type Mode } from './restore.ts';

test('replacing the workspace is the destructive mode, and the other three are not', () => {
  assert.ok(isDestructive('REPLACE_TENANT'));
  for (const mode of ['INSPECT', 'SELECTIVE', 'MERGE'] as Mode[]) {
    assert.ok(!isDestructive(mode), mode);
  }
});

test('replacing the workspace is offered only to a role that may destroy what is there', () => {
  // The server asks DELETE_CONTAINER of the destructive mode (backup-restore.md §8.2): offering it
  // to an administrator would be a button that leads to a refusal the screen knew in advance (P-05).
  assert.deepEqual([...offeredModes(true)], ['INSPECT', 'SELECTIVE', 'MERGE', 'REPLACE_TENANT']);
  assert.deepEqual([...offeredModes(false)], ['INSPECT', 'SELECTIVE', 'MERGE']);
});

test('under a legal hold a replace may be rehearsed and not run, and the other modes run', () => {
  // The server refuses the real replace while any hold is in force (backup-restore.md §8.2); the
  // rehearsal writes nothing and stays possible.
  assert.ok(!mayRunForReal('REPLACE_TENANT', true));
  assert.ok(mayRunForReal('REPLACE_TENANT', false));
  for (const mode of ['INSPECT', 'SELECTIVE', 'MERGE'] as Mode[]) {
    assert.ok(mayRunForReal(mode, true), mode);
  }
});

test('the two modes that cross or create a tenant are not among the ones offered', () => {
  // Not filtered at the end: absent from the set, so a screen built from it cannot compose one.
  assert.deepEqual([...MODES], ['INSPECT', 'SELECTIVE', 'MERGE', 'REPLACE_TENANT']);
  for (const operators of ['INSTANCE', 'NEW_TENANT']) {
    assert.ok(!(MODES as readonly string[]).includes(operators), operators);
  }
});

test('exactly one of the modes offered is destructive', () => {
  // The property the three doors hang on. Two destructive modes would mean a screen deciding
  // which of them earns the confirmation, and it would eventually decide wrongly.
  assert.equal(MODES.filter(isDestructive).length, 1);
});

test('the safest mode is first', () => {
  // `INSPECT` reads and writes nothing. A picker whose first entry replaces the workspace is a
  // picker somebody submits without reading.
  assert.equal(MODES[0], 'INSPECT');
  assert.ok(!isDestructive(MODES[0] as Mode));
});
