// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Reminder, WorkItem } from '@hubtask/sync-engine';

import {
  belongsToSeries,
  endOf,
  isPending,
  isRelative,
  isWellFormed,
  mayAddAnother,
  relativeRefusal,
  reminderLimitOf,
  splitEnd,
  withEnd,
} from './reminders.ts';

const item = (over: Partial<WorkItem> = {}) => ({ id: 'i-1', title: 'x', ...over }) as WorkItem;
const reminder = (over: Partial<Reminder> = {}) =>
  ({ id: 'r-1', item_id: 'i-1', offset_spec: 'REL:-PT1H', state: 'PENDING', version: 1, ...over }) as Reminder;

test('an offset has two forms and no third', () => {
  assert.equal(isRelative('REL:-PT1H'), true);
  assert.equal(isRelative('ABS:2026-07-15T09:00:00Z'), false);
  assert.equal(isWellFormed('REL:-PT1H'), true);
  assert.equal(isWellFormed('REL:P1D'), true);
  assert.equal(isWellFormed('ABS:2026-07-15T09:00:00Z'), true);
  assert.equal(isWellFormed('PT1H'), false, 'a duration with no prefix is not an offset');
  assert.equal(isWellFormed('ABS:tomorrow'), false);
});

test('years and months are refused, and minutes are not', () => {
  // The contract's own rule: they are calendar arithmetic rather than a length of time, and a
  // reminder that meant two different things in two months would be a promise it cannot keep.
  // The M *after* the T is minutes, which is why the check reads the date half alone.
  assert.equal(isWellFormed('REL:-P1Y'), false);
  assert.equal(isWellFormed('REL:-P2M'), false);
  assert.equal(isWellFormed('REL:-PT30M'), true);
  assert.equal(isWellFormed('REL:-P1DT12H'), true);
  assert.equal(isWellFormed('REL:-P'), false);
});

test('a relative reminder needs a due date, and the reason is the server’s own code', () => {
  // One fact, one sentence — whether this client saw the refusal coming or the server sent it.
  assert.equal(relativeRefusal(item({ due_at: '2026-07-15T09:00:00Z' })), undefined);
  assert.equal(relativeRefusal(item()), 'reminders.due_date_required');
});

test('only a pending reminder is editable, and an unknown state is not pending', () => {
  // Changing the offset of one that has fired would be changing when something already happened.
  assert.equal(isPending(reminder()), true);
  assert.equal(isPending(reminder({ state: 'SENT' })), false);
  // `LAPSED` is not in the contract's enum and is a real state after a restore. Read as a string,
  // it behaves like every other state that is not PENDING rather than crashing a comparison.
  assert.equal(isPending(reminder({ state: 'LAPSED' as never })), false);
  assert.equal(isPending(reminder({ state: 'SOMETHING_NEWER' as never })), false);
});

test('the add control is bounded by the manifest, and an absent bound is not a bound of zero', () => {
  assert.equal(reminderLimitOf({ max_reminders_per_item: 25 }), 25);
  assert.equal(reminderLimitOf({}), undefined);
  assert.equal(mayAddAnother(24, 25), true);
  assert.equal(mayAddAnother(25, 25), false);
  assert.equal(mayAddAnother(9000, undefined), true);
});

test('belonging to a series is one question, because the contract answers one', () => {
  // `recurrence_rule_id` is on the template and on every occurrence alike, so it says "this
  // belongs to a series" and nothing about which end. Telling them apart is a request per row.
  assert.equal(belongsToSeries(item({ recurrence_rule_id: 'r-1' } as never)), true);
  assert.equal(belongsToSeries(item()), false);
});

test('a rule states at most one end', () => {
  // RFC 5545 forbids both and so does the contract, so this answers one thing rather than two
  // booleans a caller would have to reconcile.
  assert.deepEqual(endOf(undefined), { kind: 'never' });
  assert.deepEqual(endOf({}), { kind: 'never' });
  assert.deepEqual(endOf({ ends_at: '2026-12-31T00:00:00Z' }), { kind: 'on', date: '2026-12-31T00:00:00Z' });
  assert.deepEqual(endOf({ max_count: 10 }), { kind: 'after', count: 10 });
});

test('a rule and its end are told apart, because the editor writes them together', () => {
  // RFC 5545 puts the end inside the rule; this API refuses a rule that carries one and takes it
  // as two fields beside it — `recurrence.rrule_carries_end`, found by sending a composed rule at
  // a real server. Both are right, so the translation lives at the seam.
  assert.deepEqual(splitEnd('FREQ=DAILY;INTERVAL=3;COUNT=5'), {
    rrule: 'FREQ=DAILY;INTERVAL=3',
    ends_at: null,
    max_count: 5,
  });
  assert.deepEqual(splitEnd('FREQ=WEEKLY;BYDAY=MO;UNTIL=20261231T000000Z'), {
    rrule: 'FREQ=WEEKLY;BYDAY=MO',
    ends_at: '2026-12-31T00:00:00.000Z',
    max_count: null,
  });
  // No end at all: both stated as null, so changing a series back to endless clears what is stored.
  assert.deepEqual(splitEnd('FREQ=WEEKLY'), { rrule: 'FREQ=WEEKLY', ends_at: null, max_count: null });
});

test('a stored series opens in the editor with its end showing', () => {
  assert.equal(withEnd('FREQ=WEEKLY', undefined), 'FREQ=WEEKLY');
  assert.equal(withEnd('FREQ=WEEKLY', { max_count: 10 }), 'FREQ=WEEKLY;COUNT=10');
  assert.equal(
    withEnd('FREQ=WEEKLY', { ends_at: '2026-12-31T00:00:00.000Z' }),
    'FREQ=WEEKLY;UNTIL=20261231T000000Z',
  );
});

test('the round trip is the one that matters', () => {
  // What the editor composes, split for the wire and composed again, is what the editor had.
  for (const rule of ['FREQ=DAILY;INTERVAL=3;COUNT=5', 'FREQ=WEEKLY;BYDAY=MO;UNTIL=20261231T000000Z', 'FREQ=MONTHLY']) {
    const parts = splitEnd(rule);
    assert.equal(withEnd(parts.rrule, parts), rule, rule);
  }
});
