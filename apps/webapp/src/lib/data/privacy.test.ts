// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The deadline, which is the column that matters, and the ordering that follows from it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  byDeadline,
  canExtend,
  deadlinePhrase,
  extensionPayload,
  extensionPhrase,
  producesArchive,
  standingOf,
  KINDS,
  SOON_MS,
  type Request,
} from './privacy.ts';

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

// The extension (UC-PRV-01 checks 9 and 10): offered where the server says it can succeed, both
// dates on the row, the reason as a sentence, and three facts in the payload.

const DAY_MS = 24 * 3600 * 1000;

test('the extension is offered exactly while the case answers a bound', () => {
  assert.equal(canExtend(aCase({ extendable_until: '2026-12-11' })), true);
  // Extended, closed, overdue: the server answers no bound, and the screen offers nothing.
  assert.equal(canExtend(aCase()), false);
  assert.equal(canExtend(aCase({ extendable_until: '' })), false);
});

test('an installation-wide case is never extended from this screen', () => {
  // The operator's, through the API or hubctl (UC-PRV-06), even where the server answers a bound.
  assert.equal(canExtend(aCase({ scope: 'INSTALLATION', extendable_until: '2026-12-11' })), false);
});

test('an extended case names both dates, the one in force first', () => {
  const extended = aCase({
    due_at: '2026-12-11T22:59:59Z',
    original_due_at: '2026-10-11T12:00:00Z',
  });
  assert.deepEqual(deadlinePhrase(extended, (iso) => `<${iso}>`), {
    code: 'app.privacy.due_extended',
    params: { at: '<2026-12-11T22:59:59Z>', original: '<2026-10-11T12:00:00Z>' },
  });
  assert.deepEqual(deadlinePhrase(aCase(), (iso) => iso), {
    code: 'app.privacy.due',
    params: { at: aCase().due_at },
  });
});

test('the reason reads as a sentence with the day the person was told', () => {
  for (const reason of ['COMPLEXITY', 'NUMBER_OF_REQUESTS'] as const) {
    const extended = aCase({ extension_reason: reason, informed_on: '2026-09-12' });
    assert.deepEqual(extensionPhrase(extended, (day) => `[${day}]`), {
      code: `app.privacy.extended_${reason.toLowerCase()}`,
      params: { informed: '[2026-09-12]' },
    });
  }
  assert.equal(extensionPhrase(aCase(), (day) => day), undefined);
});

test('owed soon reads the extended deadline, not the original', () => {
  // Past the original deadline and well before the extended one: neither overdue nor soon.
  const extended = aCase({
    original_due_at: new Date(NOW - DAY_MS).toISOString(),
    due_at: new Date(NOW + 30 * DAY_MS).toISOString(),
  });
  assert.equal(standingOf(extended, NOW), 'ahead');
});

test('the payload carries three facts, each required, both days as days', () => {
  assert.deepEqual(
    extensionPayload({ dueOn: '2026-12-11', reason: 'NUMBER_OF_REQUESTS', informedOn: '2026-09-11' }),
    { due_on: '2026-12-11', reason: 'NUMBER_OF_REQUESTS', informed_on: '2026-09-11' },
  );
  for (const draft of [
    { dueOn: '', reason: 'COMPLEXITY', informedOn: '2026-09-11' },
    { dueOn: '2026-12-11', reason: '', informedOn: '2026-09-11' },
    { dueOn: '2026-12-11', reason: 'HOLIDAYS', informedOn: '2026-09-11' },
    { dueOn: '2026-12-11', reason: 'COMPLEXITY', informedOn: '' },
    { dueOn: '11.12.2026', reason: 'COMPLEXITY', informedOn: '2026-09-11' },
  ]) {
    assert.equal(extensionPayload(draft), undefined, JSON.stringify(draft));
  }
});
