// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Sprint } from '@hubtask/sync-engine';

import { burndownLine, burndownTop, carryTargets, estimateOf, estimateText, entriesToRead, planLoad, sprintColumns, sprintDays, sprintShown, valueOf } from './sprints.ts';

const sprint = (id: string, state: Sprint['state'], start_on: string): Sprint => ({
  id, collection_id: 'c', name: id, goal: '', start_on, end_on: start_on, state, unit: 'POINTS',
  committed_value: 10, committed_count: 2, completed_value: 0, completed_count: 0, started_at: null, closed_at: null, version: 1,
});

// UC-PLN-07 check 1: a size, points, hours and a cleared field.
test('an estimate is read from what a member types', () => {
  assert.deepEqual(estimateOf('l'), { unit: 'SIZE', size: 'L' });
  assert.deepEqual(estimateOf('3'), { unit: 'POINTS', value: 3 });
  assert.deepEqual(estimateOf('1,5h'), { unit: 'HOURS', value: 1.5 });
  assert.equal(estimateOf('  '), null);
  assert.equal(estimateOf('-1'), undefined);
  assert.equal(estimateOf('0.125'), undefined);
  assert.equal(estimateOf('XXXL'), undefined);
  assert.equal(estimateText({ unit: 'HOURS', value: 1.5, size: null }), '1.5h');
  assert.equal(estimateText({ unit: 'SIZE', value: 5, size: 'L' }), 'L');
  assert.equal(estimateText(null), '');
});

// UC-PLN-07 checks 3 and 5: the burndown spans the sprint's days, the top holds the commitment.
test('the burndown is laid out across the sprint', () => {
  const points = [
    { day: '2026-11-02', remaining: 10, scope: 10, ideal: 8 },
    { day: '2026-11-03', remaining: 5, scope: 12, ideal: 6 },
  ];
  const s = { ...sprint('a', 'ACTIVE', '2026-11-02'), end_on: '2026-11-06' };
  assert.equal(sprintDays(s), 5);
  assert.equal(burndownTop(s, points), 12);
  assert.equal(burndownLine(points, 'remaining', 5, 10, 400, 100), '0,0 100,50');
  assert.equal(burndownLine([], 'remaining', 5, 10, 400, 100), '');
  // Inset, a line at the top or the bottom keeps its whole stroke inside the frame, and a value
  // past the top or more days than the sprint has never leave it.
  assert.equal(burndownLine(points, 'remaining', 5, 10, 400, 100, 4), '0,4 100,50');
  const over = [...points, { day: '2026-11-04', remaining: 20, scope: 20, ideal: -2 }, ...Array.from({ length: 4 }, (_, i) => ({ day: `2026-11-0${5 + i}`, remaining: 0, scope: 0, ideal: 0 }))];
  for (const key of ['remaining', 'ideal'] as const) {
    for (const pair of burndownLine(over, key, 5, 10, 400, 100, 4).split(' ')) {
      const [x, y] = pair.split(',').map(Number);
      assert.ok(x !== undefined && y !== undefined && x >= 0 && x <= 400 && y >= 4 && y <= 96, `${key} ${pair} leaves the frame`);
    }
  }
});

// UC-PLN-06 checks 3 and 7: only a planned sprint takes the unfinished; the running one opens.
test('a closer carries into a planned sprint, and the running one is shown first', () => {
  const all = [sprint('late', 'PLANNED', '2026-12-01'), sprint('now', 'ACTIVE', '2026-11-02'), sprint('next', 'PLANNED', '2026-11-16'), sprint('old', 'CLOSED', '2026-10-01')];
  assert.deepEqual(carryTargets(all, 'now').map((s) => s.id), ['next', 'late']);
  assert.equal(sprintShown(all)?.id, 'now');
  assert.equal(sprintShown(all.filter((s) => s.state !== 'ACTIVE'))?.id, 'next');
  assert.equal(sprintShown([]), undefined);
});

test('a column adds its estimates in the sprint\'s unit, sizes as points, the other unit as nothing', () => {
  const entries = [
    { estimate: { unit: 'POINTS' as const, value: 3, size: null } },
    { estimate: { unit: 'SIZE' as const, value: null, size: 'L' } },
    { estimate: { unit: 'HOURS' as const, value: 1.5, size: null } },
    { estimate: null },
  ];
  assert.equal(valueOf(entries as never, 'POINTS'), 8);
  assert.equal(valueOf(entries as never, 'HOURS'), 1.5);
  assert.equal(valueOf([], 'POINTS'), 0);
});

// E-22: the planned sprint against the velocity of the last three, a share and never a limit.
test('a planned sprint fills its bar against the velocity and says when it is over it', () => {
  assert.deepEqual(planLoad(5, 8), { percent: 63, isOver: false });
  assert.deepEqual(planLoad(8, 8), { percent: 100, isOver: false });
  assert.deepEqual(planLoad(13, 8), { percent: 100, isOver: true });
  assert.equal(planLoad(5, 0), undefined);
});

// §6.2: the sprint's columns are the board's, which carry the workflow's statuses.
test('a sprint\'s entries stand in the board\'s columns, a done one in the done one', () => {
  const columns = [{ id: 'open', name: 'Open' }, { id: 'review', name: 'Review' }, { id: 'done', name: 'Done', is_done_bucket: true }];
  const at: Record<string, string | null> = { a: 'review', b: null, c: 'gone', d: null };
  const entries = [{ item_id: 'a', done: false }, { item_id: 'b', done: true }, { item_id: 'c', done: false }, { item_id: 'd', done: false }, { item_id: 'e', done: true }];
  at['e'] = 'open';
  const drawn = sprintColumns(entries, columns, (id) => at[id], 'No column');
  assert.deepEqual(drawn?.map((column) => [column.id, column.entries.map((e) => e.item_id)]), [['open', []], ['review', ['a']], ['done', ['b', 'e']], [null, ['c', 'd']]]);
  assert.equal(drawn?.[2]?.isDone, true);
  assert.equal(sprintColumns(entries.slice(0, 2), columns, (id) => at[id], 'No column')?.length, 3, 'no column-less column when every entry has one');
  assert.equal(sprintColumns(entries, [], () => null, 'No column'), undefined);
});

// E-22: the backlog is its own tab, so a column of BACKLOG statuses is no column of the sprint; a
// discard (a DONE status other than the first) goes with the done column, folded.
test('under a workflow a sprint drops the backlog column and folds a discard into the done one', () => {
  const workflow = { statuses: [
    { key: 'backlog', category: 'BACKLOG' }, { key: 'todo', category: 'TODO' }, { key: 'doing', category: 'IN_PROGRESS' },
    { key: 'done', category: 'DONE' }, { key: 'dropped', category: 'DONE' },
  ] };
  const columns = [
    { id: 'b', name: 'Backlog', statuses: ['backlog'] }, { id: 't', name: 'To do', statuses: ['todo'] },
    { id: 'p', name: 'Doing', statuses: ['doing'] }, { id: 'd', name: 'Done', statuses: ['done'] }, { id: 'x', name: 'Dropped', statuses: ['dropped'] },
  ];
  const at: Record<string, string | null> = { a: 'b', b: 'p', c: 'x', d: null };
  const entries = [{ item_id: 'a', done: false }, { item_id: 'b', done: false }, { item_id: 'c', done: true }, { item_id: 'd', done: false }];
  const drawn = sprintColumns(entries, columns, (id) => at[id], 'No column', workflow);
  assert.deepEqual(drawn?.map((column) => [column.id, column.entries.map((e) => e.item_id)]), [['t', ['a']], ['p', ['b']], ['d', ['c']], [null, ['d']]]);
  assert.deepEqual(drawn?.map((column) => column.isDone), [false, false, true, false]);
  assert.equal(drawn?.[3]?.isAside, true, 'the column-less column is drawn wide');
  assert.equal(sprintColumns(entries, columns, (id) => at[id], 'No column')?.length, 6, 'without a workflow every column stays');
});

// Every entry of a sprint stands in its column, not only those on the collection's first page.
test('a sprint reads the column of each open entry the collection\'s page did not hold', () => {
  const entries = [{ item_id: 'a', done: false }, { item_id: 'b', done: false }, { item_id: 'c', done: true }];
  assert.deepEqual(entriesToRead(entries, (id) => id === 'a'), ['b']);
  assert.deepEqual(entriesToRead(entries, () => true), []);
});
