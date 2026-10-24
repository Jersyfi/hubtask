// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Container, WorkItem } from '@hubtask/sync-engine';

import { bandsOf, greetingOf, isDueByTonight, isListFigure, isProjectFigure, nextStepQuery } from './today.ts';

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
