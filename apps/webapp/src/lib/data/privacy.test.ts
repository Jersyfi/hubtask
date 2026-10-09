// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The deadline, which is the column that matters, and the ordering that follows from it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  byDeadline,
  canAct,
  canExtend,
  canStart,
  confirmsStart,
  deadlineOfDay,
  deadlinePhrase,
  extensionPayload,
  extensionPhrase,
  keptPartPhrase,
  keptPhrase,
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

test('a row acts on this workspace\'s cases and on no installation-wide one', () => {
  // The operator's (UC-PRV-06/6): start, refuse, the mode and the extension all stay off the row.
  assert.equal(canAct(aCase()), true);
  assert.equal(canAct(aCase({ scope: 'TENANT' })), true);
  assert.equal(canAct(aCase({ scope: 'INSTALLATION' })), false);
});

test('an erasure is started only by whom the server lets start it, and only after a confirmation', () => {
  // UC-PRV-03 checks 1 and 8, P-05: the owner's DELETE_CONTAINER, asked of an erasure alone.
  const erasure = aCase({ kind: 'ERASURE' });
  assert.equal(canStart(erasure, true), true);
  assert.equal(canStart(erasure, false), false);
  assert.equal(canStart(aCase(), false), true);
  assert.equal(canStart(aCase({ status: 'IN_PROGRESS' }), true), false);
  assert.equal(canStart(aCase({ kind: 'ERASURE', scope: 'INSTALLATION' }), true), false);
  assert.equal(confirmsStart(erasure), true);
  assert.equal(confirmsStart(aCase()), false);
});

test('a case a hold kept part of says so, and says when the rest went', () => {
  // UC-PRV-03 checks 11 and 12: partly completed while a hold keeps a part, the rest's day after.
  const part = {
    hold_id: 'h1', hold_scope: { kind: 'CONTAINER' as const }, account: false,
    entries: 1, comments: 2, assignments: 3, intake: 0,
  };
  assert.equal(keptPhrase(aCase()), undefined);
  assert.deepEqual(keptPhrase(aCase({ kept: [part] })), { code: 'app.privacy.kept_partly', params: { holds: 1 } });
  assert.deepEqual(
    keptPhrase(aCase({ kept: [{ ...part, blocked: { code: 'privacy.erasure_blocked_by_rule', params: { rules: '1' } } }] })),
    { code: 'app.privacy.kept_waits', params: { holds: 1, rules: '1' } },
  );
  assert.deepEqual(
    keptPhrase(aCase({ kept: [{ ...part, erased_at: '2026-09-02T10:00:00Z' }, { ...part, hold_id: 'h2', erased_at: '2026-09-03T10:00:00Z' }] })),
    { code: 'app.privacy.kept_rest_erased', params: { at: '2026-09-03T10:00:00Z' } },
  );
  assert.equal(keptPartPhrase(part, 'a hub').code, 'app.privacy.kept_part');
  assert.equal(keptPartPhrase({ ...part, account: true }, 'this person').code, 'app.privacy.kept_part_account');
  assert.deepEqual(keptPartPhrase(part, 'a hub').params, { scope: 'a hub', comments: 2, assignments: 3, entries: 1, intake: 0 });
});

test('owed soon starts seven days before the deadline, when the watch starts warning', () => {
  // UC-PRV-01 check 5: the same moment as the server's WarningWindow, written out by hand here
  // rather than read from SOON_MS, so that the constant cannot agree with itself.
  const day = 24 * 3600 * 1000;
  assert.equal(standingOf(aCase({ due_at: new Date(NOW + 7 * day).toISOString() }), NOW), 'soon');
  assert.equal(standingOf(aCase({ due_at: new Date(NOW + 6 * day).toISOString() }), NOW), 'soon');
  assert.equal(standingOf(aCase({ due_at: new Date(NOW + 7 * day + 1000).toISOString() }), NOW), 'ahead');
});

test('a deadline named as a day ends at the last second of that day in the workspace', () => {
  // UC-PRV-01 check 6, written out by hand: Berlin on both sides of the change to summer time,
  // and a workspace that counts in UTC.
  assert.equal(deadlineOfDay('2027-03-27', 'Europe/Berlin'), '2027-03-27T22:59:59.000Z');
  assert.equal(deadlineOfDay('2027-03-28', 'Europe/Berlin'), '2027-03-28T21:59:59.000Z');
  assert.equal(deadlineOfDay('2026-12-01', 'UTC'), '2026-12-01T23:59:59.000Z');
  assert.equal(deadlineOfDay('', 'UTC'), undefined);
  assert.equal(deadlineOfDay('1.12.2026', 'UTC'), undefined);
});
