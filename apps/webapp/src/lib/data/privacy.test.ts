// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The deadline, which is the column that matters, and the ordering that follows from it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { byDeadline, producesArchive, standingOf, KINDS, SOON_MS, type Request } from './privacy.ts';

const NOW = Date.UTC(2026, 8, 11, 12, 0, 0);

function aCase(over: Partial<Request> = {}): Request {
  return {
    id: 'r1',
    kind: 'ACCESS',
    status: 'RECEIVED',
    received_at: new Date(NOW).toISOString(),
    due_at: new Date(NOW + 30 * 24 * 3600 * 1000).toISOString(),
    ...over,
  };
}

test('a deadline that has passed is overdue', () => {
  const standing = standingOf(aCase({ due_at: new Date(NOW - 1000).toISOString() }), NOW);
  assert.equal(standing, 'overdue');
});

test('the moment the deadline falls is already overdue', () => {
  // A statutory period is answered *by* the date, so the instant itself is not still ahead.
  assert.equal(standingOf(aCase({ due_at: new Date(NOW).toISOString() }), NOW), 'overdue');
});

test('a deadline inside the near window is near, and one beyond it is not', () => {
  assert.equal(standingOf(aCase({ due_at: new Date(NOW + SOON_MS - 1).toISOString() }), NOW), 'soon');
  assert.equal(standingOf(aCase({ due_at: new Date(NOW + SOON_MS + 1).toISOString() }), NOW), 'ahead');
});

test('a case that is closed reports no standing', () => {
  // A deadline that has been answered is not a deadline. An overdue mark on a completed case would
  // be the list telling somebody to act on something that is finished.
  for (const status of ['COMPLETED', 'REJECTED'] as const) {
    const closed = aCase({ status, due_at: new Date(NOW - 90 * 24 * 3600 * 1000).toISOString() });
    assert.equal(standingOf(closed, NOW), undefined, status);
  }
});

test('a deadline nobody can read reports no standing rather than "overdue"', () => {
  assert.equal(standingOf(aCase({ due_at: 'not a date' }), NOW), undefined);
});

test('the list is ordered by what is closest to its deadline', () => {
  const far = aCase({ id: 'far', due_at: new Date(NOW + 20 * 24 * 3600 * 1000).toISOString() });
  const near = aCase({ id: 'near', due_at: new Date(NOW + 1 * 24 * 3600 * 1000).toISOString() });
  const past = aCase({ id: 'past', due_at: new Date(NOW - 5 * 24 * 3600 * 1000).toISOString() });
  assert.deepEqual(byDeadline([far, near, past]).map((each) => each.id), ['past', 'near', 'far']);
});

test('closed cases go last, whatever their deadline was', () => {
  const done = aCase({
    id: 'done',
    status: 'COMPLETED',
    due_at: new Date(NOW - 90 * 24 * 3600 * 1000).toISOString(),
  });
  const open = aCase({ id: 'open', due_at: new Date(NOW + 25 * 24 * 3600 * 1000).toISOString() });
  assert.deepEqual(byDeadline([done, open]).map((each) => each.id), ['open', 'done']);
});

test('ordering does not disturb the list it was given', () => {
  const cases = [aCase({ id: 'b', due_at: new Date(NOW + 2000).toISOString() }), aCase({ id: 'a' })];
  byDeadline(cases);
  assert.deepEqual(cases.map((each) => each.id), ['b', 'a']);
});

test('the two kinds that produce an archive are the two that need a target', () => {
  assert.ok(producesArchive('ACCESS'));
  assert.ok(producesArchive('PORTABILITY'));
  for (const kind of ['ERASURE', 'RESTRICTION', 'OBJECTION', 'RECTIFICATION'] as const) {
    assert.ok(!producesArchive(kind), kind);
  }
});

test('every kind the contract names is offered, and erasure is last', () => {
  assert.equal(KINDS.length, 6);
  assert.equal(KINDS.at(-1), 'ERASURE', 'the one that removes things is not the first choice');
});
