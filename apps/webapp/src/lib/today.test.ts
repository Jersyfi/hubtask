// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Container, WorkItem } from '@hubtask/sync-engine';

import { bandsOf, greetingOf, isDueByTonight, isListFigure, isProjectFigure, nextStepQuery, productBandsOf, timeTodayOf, todayRowCountOf, heldActRows } from './today.ts';

const BERLIN = 'Europe/Berlin';
const AUCKLAND = 'Pacific/Auckland';

function item(over: Partial<WorkItem> = {}): WorkItem {
  return { id: 'i-1', title: 'An entry', type: 'TASK', version: 1, collection_id: 'c-1', completion: { is_completed: false }, ...over } as WorkItem;
}

function collection(kind: string | undefined, over: Partial<Container> = {}): Container {
  return { id: 'c-1', type: 'COLLECTION', name: 'A place', version: 1, ...(kind ? { figure: { kind, today: '2026-10-23' } } : {}), ...over } as Container;
}

// 10:00 in Berlin on 23 October 2026 (summer time ends on the 25th).
const NOW = Date.parse('2026-10-23T08:00:00Z');

test('a timed date is due today until the reader\'s next day begins', () => {
  const cases: [string, string, string, boolean][] = [
    ['later this evening in Berlin', '2026-10-23T21:30:00Z', BERLIN, true],
    ['twenty minutes before midnight in Berlin', '2026-10-23T21:40:00Z', BERLIN, true],
    ['midnight in Berlin is tomorrow', '2026-10-23T22:00:00Z', BERLIN, false],
    ['the same instant is already tomorrow in Auckland', '2026-10-23T21:30:00Z', AUCKLAND, false],
  ];
  for (const [name, due, zone, expected] of cases) {
    assert.equal(isDueByTonight(item({ due_at: due }), zone, NOW), expected, name);
  }
});

test('an all-day date is due today on its own day and before it', () => {
  const day = (date: string) => item({ due_at: `${date}T00:00:00Z`, due_date_only: true, due_time_zone: 'UTC' });
  assert.equal(isDueByTonight(day('2026-10-23'), BERLIN, NOW), true, 'today');
  assert.equal(isDueByTonight(day('2026-10-22'), BERLIN, NOW), true, 'yesterday');
  assert.equal(isDueByTonight(day('2026-10-24'), BERLIN, NOW), false, 'tomorrow');
});

test('a finished entry or one without a date is not due', () => {
  assert.equal(isDueByTonight(item({ due_at: '2026-10-23T09:00:00Z', completion: { is_completed: true } } as Partial<WorkItem>), BERLIN, NOW), false);
  assert.equal(isDueByTonight(item(), BERLIN, NOW), false);
});

test('the bands keep the server\'s order and leave out the entries of a list', () => {
  const late = item({ id: 'late', due_at: '2026-10-22T08:00:00Z' });
  const morning = item({ id: 'morning', due_at: '2026-10-23T07:00:00Z' });
  const tonight = item({ id: 'tonight', due_at: '2026-10-23T18:00:00Z' });
  const listed = item({ id: 'listed', collection_id: 'groceries', due_at: '2026-10-23T18:00:00Z' });
  const nextWeek = item({ id: 'next', due_at: '2026-10-30T09:00:00Z' });
  const bands = bandsOf([late, morning, tonight, listed, nextWeek], BERLIN, NOW, (id) => id === 'groceries');
  assert.deepEqual(bands.earlier.map((each) => each.id), ['late', 'morning'], 'what is past is earlier, whenever it was due');
  assert.deepEqual(bands.today.map((each) => each.id), ['tonight']);
  assert.deepEqual(bands.later.map((each) => each.id), ['next']);
});

test('a list counts what is open, a project its progress, and nothing else is either', () => {
  assert.equal(isListFigure(collection('OPEN')), true);
  assert.equal(isProjectFigure(collection('PROGRESS')), true);
  for (const kind of ['FIELD_SUM', 'DUE', 'RHYTHM', undefined]) {
    assert.equal(isListFigure(collection(kind)), false, `${kind} is a list`);
    assert.equal(isProjectFigure(collection(kind)), false, `${kind} is a project`);
  }
  assert.equal(isListFigure(collection('OPEN', { type: 'HUB' })), false, 'a hub is never a tile');
});

test('a project\'s next step is its soonest open dated entry, at any level', () => {
  const query = nextStepQuery('c-9');
  assert.deepEqual(query.scope, { container_id: 'c-9', include_descendants: true });
  assert.deepEqual(query.sort, [{ field: 'due_at', dir: 'ASC' }]);
  assert.equal(query.page.size, 1);
});

test('the greeting follows the hour where the person is, not where the device is', () => {
  // 2026-10-23 06:30 UTC: 08:30 in Berlin, 19:30 in Auckland.
  const at = Date.UTC(2026, 9, 23, 6, 30);
  assert.equal(greetingOf(BERLIN, at), 'morning');
  assert.equal(greetingOf(AUCKLAND, at), 'evening');
  assert.equal(greetingOf(BERLIN, Date.UTC(2026, 9, 23, 12, 0)), 'day');
  // The edges: five is morning, eleven the day, six in the evening the evening.
  assert.equal(greetingOf('UTC', Date.UTC(2026, 9, 23, 5, 0)), 'morning');
  assert.equal(greetingOf('UTC', Date.UTC(2026, 9, 23, 4, 59)), 'evening');
  assert.equal(greetingOf('UTC', Date.UTC(2026, 9, 23, 11, 0)), 'day');
  assert.equal(greetingOf('UTC', Date.UTC(2026, 9, 23, 18, 0)), 'evening');
});

test('a product band stands only where its module is on, and none before the modules are known', () => {
  assert.deepEqual(productBandsOf(undefined), []);
  assert.deepEqual(productBandsOf([]), []);
  assert.deepEqual(productBandsOf(['KNOWLEDGE', 'GOALS']), []);
  assert.deepEqual(productBandsOf(['TIME']), ['time']);
  assert.deepEqual(productBandsOf(['TIME', 'SERVICE_DESK', 'PLANNING']), ['approvals', 'time']);
  assert.deepEqual(productBandsOf(['AGENTS', 'TIME', 'SERVICE_DESK']), ['approvals', 'agent_acts', 'time']);
  assert.deepEqual(productBandsOf(['AGENTS']), ['agent_acts']);
});

test('time today is the day\'s recorded minutes against the week, with the running timer beside', () => {
  const week = { minutes: 600, days: [{ day: '2026-11-02', minutes: 240, locked: false }, { day: '2026-11-03', minutes: 360, locked: false }] };
  assert.deepEqual(timeTodayOf(week, '2026-11-03', { running: false }), { today: 360, week: 600, running: undefined });
  assert.deepEqual(timeTodayOf(week, '2026-11-04', { running: true, elapsed_minutes: 25, entry_id: 'e1' }), { today: 0, week: 600, running: { minutes: 25, entryId: 'e1' } });
  // The timer's entry by name, where the server answers it (user-experience.md §7.6).
  assert.deepEqual(timeTodayOf(week, '2026-11-04', { running: true, elapsed_minutes: 25, entry_id: 'e1', entry_title: 'Checkout form' })?.running, { minutes: 25, entryId: 'e1', entryTitle: 'Checkout form' });
  assert.equal(timeTodayOf(undefined, '2026-11-04', { running: false }), undefined, 'not drawn before the week is read');
});

// user-experience.md §3.3: the products' bands carry their count on the Today row.
test('the Today row counts the approvals and held acts waiting for the reader\'s yes, and nothing before they are known', () => {
  assert.equal(todayRowCountOf(undefined), undefined, 'not drawn before the approvals are read');
  assert.equal(todayRowCountOf([]), undefined, 'nothing waiting draws no count');
  assert.equal(todayRowCountOf([{ state: 'PENDING' }, { state: 'APPROVED' }, { state: 'PENDING' }]), 2);
  assert.equal(todayRowCountOf([{ state: 'DECLINED' }]), undefined);
  assert.equal(todayRowCountOf(undefined, [{ status: 'PENDING' }]), 1, 'an agent\'s held act waits too');
  assert.equal(todayRowCountOf([{ state: 'PENDING' }], [{ status: 'PENDING' }, { status: 'FAILED' }]), 2);
  // The reader's approvals come a page at a time: a first page with more behind it is a floor.
  const page = Array.from({ length: 50 }, () => ({ state: 'PENDING' }));
  assert.equal(todayRowCountOf(page, undefined, true), '50+', 'a first page was counted as all');
  assert.equal(todayRowCountOf([], undefined, true), undefined, 'nothing pending on the first page says nothing');
});

// An irreversible act is approved where its preview is read, never from a row on Today.
test('a held act is approved at Today\'s row only where it can be undone', () => {
  const acts = [
    { id: 'r1', agent_id: 'a1', class: 'REVERSIBLE', status: 'PENDING', created_at: '2026-11-03T08:00:00Z' },
    { id: 'r2', agent_id: 'a2', class: 'IRREVERSIBLE', status: 'PENDING', created_at: '2026-11-03T09:00:00Z' },
    { id: 'r3', agent_id: 'a1', class: 'REVERSIBLE', status: 'APPROVED', created_at: '2026-11-03T10:00:00Z' },
  ];
  const agents = [{ id: 'a1', name: 'Request sorter', owner_id: 'me' }, { id: 'a2', name: 'Duplicate finder', owner_id: 'other' }];
  assert.deepEqual(heldActRows(acts, agents, 'me', true), [
    { id: 'r1', name: 'Request sorter', at: '2026-11-03T08:00:00Z', mayApproveHere: true, path: '/agents?tab=requests&agent=a1' },
    { id: 'r2', name: 'Duplicate finder', at: '2026-11-03T09:00:00Z', mayApproveHere: false, path: '/agents?tab=requests&agent=a2' },
  ]);
  assert.deepEqual(heldActRows(acts, [], 'me', false).map((row) => [row.name, row.path]), [['', undefined], ['', undefined]], 'an agent not read yet has no name and no way there');
  assert.deepEqual(heldActRows(undefined, agents, 'me', true), []);
});
