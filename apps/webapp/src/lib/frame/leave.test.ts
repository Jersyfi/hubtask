// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { LeaveModel } from './leave.ts';

test('a screen without unsaved changes lets every move go', () => {
  const guard = new LeaveModel();
  guard.hold(() => 0);
  assert.equal(guard.admits('/administration'), true);
  assert.equal(guard.pending, undefined);
});

test('a screen with unsaved changes holds the move until the reader answers', () => {
  const guard = new LeaveModel();
  guard.hold(() => 2);
  assert.equal(guard.admits('/administration'), false);
  assert.equal(guard.pending, '/administration');
  guard.stay();
  assert.equal(guard.pending, undefined);
});

test('leaving makes the held move once, past the guard, and the next move is asked again', () => {
  const guard = new LeaveModel();
  guard.hold(() => 1);
  guard.admits('/administration/people');
  const went: string[] = [];
  guard.leave((path) => {
    // The router asks again while it makes the move; the reader already answered.
    assert.equal(guard.admits(path), true);
    went.push(path);
  });
  assert.deepEqual(went, ['/administration/people']);
  assert.equal(guard.admits('/today'), false);
});

test('a screen that lets go no longer holds anything', () => {
  const guard = new LeaveModel();
  const release = guard.hold(() => 3);
  assert.equal(guard.unsaved, 3);
  release();
  assert.equal(guard.unsaved, 0);
  assert.equal(guard.admits('/today'), true);
});

test('leaving without a held move makes none', () => {
  const guard = new LeaveModel();
  let moved = false;
  guard.leave(() => (moved = true));
  assert.equal(moved, false);
});
