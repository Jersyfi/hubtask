// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// §7's table as a table (F6-13): every row, and the daily cap. Hierarchies rather than heuristics.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { dayOf, momentOf, type CelebrationContext, type HeldContainer, type HeldEntry } from './celebration.ts';

const HUB = 'hub-1';
const COLLECTION = 'col-1';
const OTHER_COLLECTION = 'col-2';

const containers: HeldContainer[] = [
  { id: HUB, type: 'HUB', parent_id: null },
  { id: COLLECTION, type: 'COLLECTION', parent_id: HUB },
  { id: OTHER_COLLECTION, type: 'COLLECTION', parent_id: HUB },
];

const entry = (id: string, over: Partial<HeldEntry> = {}): HeldEntry => ({
  id, collection_id: COLLECTION, parent_id: null, is_completed: false, ...over,
});

const context = (completedId: string, entries: HeldEntry[], over: Partial<CelebrationContext> = {}): CelebrationContext => ({
  completedId, entries, containers, today: '2026-09-18', zone: 'Europe/Berlin', isBeforeOnboarding: false, ...over,
});

test('every completion is tier 1', () => {
  const entries = [entry('a', { is_completed: true }), entry('b')];
  assert.deepEqual(momentOf(context('a', entries)), { tier: 1, reason: 'completion', isCapped: false });
});

test('an entry the copy does not hold, or that is not complete, is tier 1 and nothing else', () => {
  assert.equal(momentOf(context('missing', [entry('b')])).tier, 1);
  assert.equal(momentOf(context('a', [entry('a')])).tier, 1);
});

test('the last open entry under its parent is tier 2: the last activity closes a work package', () => {
  const entries = [
    entry('wp', { is_completed: false }),
    entry('act-1', { parent_id: 'wp', is_completed: true }),
    entry('act-2', { parent_id: 'wp', is_completed: true }),
    entry('other'),
  ];
  assert.deepEqual(momentOf(context('act-2', entries)), { tier: 2, reason: 'parent', isCapped: false });
  // A sibling still open: tier 1.
  assert.equal(momentOf(context('act-1', [...entries.slice(0, 2), entry('act-3', { parent_id: 'wp' })])).tier, 1);
  // A trashed sibling is not open.
  assert.equal(momentOf(context('act-1', [entries[0]!, entries[1]!, entry('gone', { parent_id: 'wp', deleted_at: '2026-09-01T00:00:00Z' })])).tier, 2);
});

test('a collection with nothing open is tier 3, and a hub with nothing open says hub', () => {
  const closed = [entry('a', { is_completed: true }), entry('b', { is_completed: true })];
  assert.deepEqual(momentOf(context('b', [...closed, entry('c', { collection_id: OTHER_COLLECTION })])), { tier: 3, reason: 'collection', isCapped: false });
  assert.deepEqual(momentOf(context('b', [...closed, entry('c', { collection_id: OTHER_COLLECTION, is_completed: true })])), { tier: 3, reason: 'hub', isCapped: false });
  // An open entry elsewhere in the same collection: not the collection's close.
  assert.equal(momentOf(context('a', [...closed, entry('open')])).reason, 'completion');
});

test("the day's close: the last entry due today, across what the copy holds", () => {
  const entries = [
    entry('today-1', { is_completed: true, due_at: '2026-09-18T07:00:00Z' }),
    entry('today-2', { is_completed: true, due_at: '2026-09-18T15:30:00Z', collection_id: OTHER_COLLECTION }),
    entry('tomorrow', { due_at: '2026-09-19T08:00:00Z' }),
    entry('undated', { collection_id: OTHER_COLLECTION }),
  ];
  assert.deepEqual(momentOf(context('today-2', entries)), { tier: 3, reason: 'day', isCapped: false });
  // One due today still open: an ordinary completion.
  assert.equal(momentOf(context('today-1', [...entries.slice(0, 1), entry('today-3', { due_at: '2026-09-18T20:00:00Z' })])).tier, 1);
  // Due today is read where the person is: 23:30 UTC is tomorrow in Berlin.
  assert.equal(momentOf(context('late', [entry('late', { is_completed: true, due_at: '2026-09-18T23:30:00Z' }), entry('open')])).reason, 'completion');
});

test('a completion before the tour ended is the first-ever moment, tier 3 - once', () => {
  const entries = [entry('a', { is_completed: true }), entry('b')];
  assert.deepEqual(momentOf(context('a', entries, { isBeforeOnboarding: true })), { tier: 3, reason: 'first', isCapped: false });
  // The day's moment spent: "that was your first" is not said of the second, which is read like any other.
  assert.deepEqual(momentOf(context('a', entries, { isBeforeOnboarding: true, lastTier3On: '2026-09-18' })), { tier: 1, reason: 'completion', isCapped: false });
  const closed = [entry('a', { is_completed: true }), entry('b', { is_completed: true })];
  assert.deepEqual(momentOf(context('b', closed, { isBeforeOnboarding: true, lastTier3On: '2026-09-18' })), { tier: 2, reason: 'hub', isCapped: true });
});

test('at most one tier 3 a day: a second qualifying moment falls back to 2 and says why', () => {
  const closed = [entry('a', { is_completed: true }), entry('b', { is_completed: true })];
  const capped = momentOf(context('b', closed, { lastTier3On: '2026-09-18' }));
  assert.deepEqual(capped, { tier: 2, reason: 'hub', isCapped: true });
  const yesterday = momentOf(context('b', closed, { lastTier3On: '2026-09-17' }));
  assert.equal(yesterday.tier, 3);
});

test('the day of a due moment is read in the zone', () => {
  assert.equal(dayOf('2026-09-18T23:30:00Z', 'Europe/Berlin'), '2026-09-19');
  assert.equal(dayOf('2026-09-18T23:30:00Z', 'UTC'), '2026-09-18');
  assert.equal(dayOf(null, 'UTC'), undefined);
  assert.equal(dayOf('not a moment', 'UTC'), undefined);
});
