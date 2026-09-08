// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { WorkItem } from '@hubtask/sync-engine';

import {
  addDays,
  contains,
  daysBetween,
  dueInputFor,
  dueOf,
  firstWeekday,
  isDueSoon,
  isOverdue,
  monthOf,
  nextDay,
  shift,
  weekOf,
} from './due.ts';

const BERLIN = 'Europe/Berlin';
const SAO_PAULO = 'America/Sao_Paulo';

function item(over: Partial<WorkItem> = {}): WorkItem {
  return { id: 'i-1', title: 'An entry', type: 'TASK', version: 1, ...over } as WorkItem;
}

test('a timed due date is read in the entry’s zone, and the zone travels when it is not the reader’s', () => {
  const entry = item({ due_at: '2026-07-15T12:00:00Z', due_date_only: false, due_time_zone: BERLIN });

  assert.deepEqual(dueOf(entry, BERLIN), { date: '2026-07-15', time: '14:00', zone: undefined });
  // The same entry to a reader in São Paulo: their own clock is four hours behind, and the zone is
  // stated because it is not theirs — which is what makes the control draw the note.
  assert.deepEqual(dueOf(entry, SAO_PAULO), { date: '2026-07-15', time: '14:00', zone: BERLIN });
});

test('an all-day date carries no time at all, in any zone', () => {
  // Never `00:00`, which says something different — and never a time drawn from an instant that a
  // reader further west would read as the previous evening.
  const entry = item({ due_at: '2026-07-15T03:00:00Z', due_date_only: true, due_time_zone: SAO_PAULO });
  assert.deepEqual(dueOf(entry, SAO_PAULO), { date: '2026-07-15', time: undefined, zone: undefined });
  assert.equal(dueOf(entry, BERLIN)?.time, undefined);
});

test('an entry with no due date has none, rather than one at the epoch', () => {
  assert.equal(dueOf(item(), BERLIN), null);
  assert.equal(dueOf(undefined, BERLIN), null);
});

test('the three fields go back as three, and the zone is always stated', () => {
  // Including when it is the reader's: the server stores what it is told, and a null zone would
  // make the entry mean "whoever reads it", which is not what anybody chose.
  assert.deepEqual(dueInputFor({ date: '2026-07-15', time: '09:00' }, BERLIN), {
    due_at: '2026-07-15T07:00:00.000Z',
    due_date_only: false,
    due_time_zone: BERLIN,
  });
  assert.deepEqual(dueInputFor({ date: '2026-07-15' }, SAO_PAULO), {
    due_at: '2026-07-15T03:00:00.000Z',
    due_date_only: true,
    due_time_zone: SAO_PAULO,
  });
});

test('a due date survives the round trip it was read from', () => {
  const original = { date: '2026-03-29', time: '03:30', zone: BERLIN };
  const written = dueInputFor(original, SAO_PAULO)!;
  const back = dueOf(item({ ...written }) as WorkItem, SAO_PAULO);
  assert.deepEqual(back, original);
});

test('overdue compares instants, and a finished entry never is', () => {
  const noon = Date.parse('2026-07-15T12:00:00Z');
  const due = item({ due_at: '2026-07-15T11:00:00Z', due_date_only: false, due_time_zone: BERLIN });

  assert.equal(isOverdue(due, BERLIN, noon), true);
  assert.equal(isOverdue(due, SAO_PAULO, noon), true, 'the same moment is past for every reader');
  assert.equal(isOverdue(item({ ...due, completion: { is_completed: true } } as never), BERLIN, noon), false);
  assert.equal(isOverdue(item(), BERLIN, noon), false);
});

test('an all-day date is overdue when its day ends where the date lives', () => {
  // 15 July in Berlin ends at 22:00 UTC. An hour before that it is still today there, and an hour
  // after it is not — whatever the reader's own clock says.
  const allDay = item({ due_at: '2026-07-14T22:00:00Z', due_date_only: true, due_time_zone: BERLIN });
  assert.equal(isOverdue(allDay, SAO_PAULO, Date.parse('2026-07-15T21:00:00Z')), false);
  assert.equal(isOverdue(allDay, SAO_PAULO, Date.parse('2026-07-15T23:00:00Z')), true);
});

test('due soon is neither overdue nor finished', () => {
  const now = Date.parse('2026-07-15T12:00:00Z');
  const hour = 3_600_000;
  const soon = item({ due_at: '2026-07-15T14:00:00Z' });
  assert.equal(isDueSoon(soon, BERLIN, now, 24 * hour), true);
  assert.equal(isDueSoon(soon, BERLIN, now, hour), false);
  assert.equal(isDueSoon(item({ due_at: '2026-07-15T11:00:00Z' }), BERLIN, now, 24 * hour), false);
});

test('a week starts on the day the account says', () => {
  // Wednesday 15 July 2026.
  assert.deepEqual(weekOf('2026-07-15', 'MONDAY'), { from: '2026-07-13', to: '2026-07-19' });
  assert.deepEqual(weekOf('2026-07-15', 'SUNDAY'), { from: '2026-07-12', to: '2026-07-18' });
  assert.deepEqual(weekOf('2026-07-15', 'SATURDAY'), { from: '2026-07-11', to: '2026-07-17' });
  // The field is nullable, and Monday is the stated fallback rather than one of the three at random.
  assert.equal(firstWeekday(null), 1);
  assert.deepEqual(weekOf('2026-07-15', null), weekOf('2026-07-15', 'MONDAY'));
});

test('a month is whole, and a leap February needs no table', () => {
  assert.deepEqual(monthOf('2026-07-15'), { from: '2026-07-01', to: '2026-07-31' });
  assert.deepEqual(monthOf('2026-12-31'), { from: '2026-12-01', to: '2026-12-31' });
  assert.deepEqual(monthOf('2028-02-10'), { from: '2028-02-01', to: '2028-02-29' });
});

test('a window moves by its own length, in both directions', () => {
  const week = { from: '2026-07-13', to: '2026-07-19' };
  assert.deepEqual(shift(week, 1), { from: '2026-07-20', to: '2026-07-26' });
  assert.deepEqual(shift(week, -1), { from: '2026-07-06', to: '2026-07-12' });
  // A month moves by its own length in days, which is what "the next screenful" means on a
  // timeline drawn in day columns — not "the next calendar month".
  assert.equal(daysBetween('2026-07-01', '2026-08-01'), 31);
});

test('a window includes both its ends', () => {
  const week = { from: '2026-07-13', to: '2026-07-19' };
  assert.equal(contains(week, '2026-07-13'), true);
  assert.equal(contains(week, '2026-07-19'), true);
  assert.equal(contains(week, '2026-07-20'), false);
  assert.equal(nextDay('2026-12-31'), '2027-01-01');
  assert.equal(addDays('2026-03-01', -1), '2026-02-28');
});
