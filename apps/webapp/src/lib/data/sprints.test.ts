// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Sprint } from '@hubtask/sync-engine';

import { burndownLine, burndownTop, carryTargets, estimateOf, estimateText, sprintDays, sprintShown } from './sprints.ts';

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
});

// UC-PLN-06 checks 3 and 7: only a planned sprint takes the unfinished; the running one opens.
test('a closer carries into a planned sprint, and the running one is shown first', () => {
  const all = [sprint('late', 'PLANNED', '2026-12-01'), sprint('now', 'ACTIVE', '2026-11-02'), sprint('next', 'PLANNED', '2026-11-16'), sprint('old', 'CLOSED', '2026-10-01')];
  assert.deepEqual(carryTargets(all, 'now').map((s) => s.id), ['next', 'late']);
  assert.equal(sprintShown(all)?.id, 'now');
  assert.equal(sprintShown(all.filter((s) => s.state !== 'ACTIVE'))?.id, 'next');
  assert.equal(sprintShown([]), undefined);
});
