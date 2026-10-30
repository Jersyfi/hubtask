// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import type { BaselineComparison, PlanSchedule, PlanScheduleEntry } from '@hubtask/sync-engine';

import { dependencyDrawn, ganttRows, inTreeOrder, lagOf, proposedMoves } from './planning.ts';

const entry = (id: string, more: Partial<PlanScheduleEntry> = {}): PlanScheduleEntry => ({
  item_id: id, title: id, parent_id: null, collection_id: 'c', start: '2026-11-02', finish: '2026-11-04',
  duration_days: 3, moved: false, total_float_days: 0, free_float_days: 0, critical: false, summary: false,
  external: false, mode: 'MANUAL', milestone: false, done: false, ...more,
});

const schedule = (entries: PlanScheduleEntry[]): PlanSchedule => ({
  zone: 'UTC', finish: '2026-11-06', entries, dependencies: [], conflicts: [], unscheduled: [], critical_path: [],
});

// UC-PLN-02 checks 5 and 7: a parent's summary bar is drawn above its children, indented by depth.
test('the rows follow the tree, a child under its parent, an orphan at the top', () => {
  const rows = inTreeOrder([
    entry('child', { parent_id: 'parent' }),
    entry('parent', { summary: true }),
    entry('grandchild', { parent_id: 'child' }),
    entry('orphan', { parent_id: 'gone' }),
  ]);
  assert.deepEqual(rows.map((row) => [row.entry.item_id, row.depth]), [
    ['parent', 0], ['child', 1], ['grandchild', 2], ['orphan', 0],
  ]);
});

test('a parent loop still draws every entry once', () => {
  const rows = inTreeOrder([entry('a', { parent_id: 'b' }), entry('b', { parent_id: 'a' })]);
  assert.equal(rows.length, 2);
});

// UC-PLN-02 check 7, UC-PLN-04, UC-PLN-05: a milestone, a summary, the critical chain and the baseline.
test('a row says what kind of bar it is and where the baseline held it', () => {
  const comparison: BaselineComparison = {
    baseline: { id: 'b', plan_id: 'p', name: 'Agreed', taken_at: '2026-10-01T00:00:00Z', taken_by: null, entry_count: 2 },
    entries: [
      {
        item_id: 'build', in_baseline: true, in_plan: true, baseline_start_at: '2026-11-01T23:00:00Z',
        baseline_due_at: '2026-11-03T23:00:00Z', start_at: null, due_at: null, start_variance_days: 2, finish_variance_days: 2,
      },
      {
        item_id: 'launch', in_baseline: false, in_plan: true, baseline_start_at: null, baseline_due_at: null,
        start_at: null, due_at: null, start_variance_days: null, finish_variance_days: null,
      },
    ],
  };
  const rows = ganttRows(
    schedule([entry('build', { critical: true }), entry('launch', { milestone: true }), entry('relaunch', { summary: true })]),
    comparison,
    (instant) => instant.slice(0, 10),
  );
  assert.equal(rows[0]?.isCritical, true);
  assert.deepEqual(rows[0]?.baseline, { start: '2026-11-01', due: '2026-11-03' });
  assert.equal(rows[1]?.kind, 'milestone');
  assert.equal(rows[1]?.baseline, undefined, 'an entry added since the baseline has none');
  assert.equal(rows[2]?.kind, 'summary');
});

// UC-PLN-02 check 4: only what the plan proposes inside the plan is offered to accept.
test('the moves offered are the proposed ones inside the plan and not done', () => {
  const offered = proposedMoves(
    schedule([
      entry('build', { moved: true }),
      entry('outside', { moved: true, external: true }),
      entry('finished', { moved: true, done: true }),
      entry('kept'),
    ]),
  );
  assert.deepEqual(offered, ['build']);
});

// UC-PLN-02 check 1: a lag or a lead of up to 999 working days.
test('a lag is a whole number of working days within the bound', () => {
  assert.equal(lagOf('2'), 2);
  assert.equal(lagOf(' -3 '), -3);
  assert.equal(lagOf('+999'), 999);
  assert.equal(lagOf('1000'), undefined);
  assert.equal(lagOf('1.5'), undefined);
  assert.equal(lagOf(''), undefined);
});

// UC-PLN-02 check 7: a dependency drawn from a bar onto a row makes the row wait for the bar.
test('a dependency drawn makes the row let go over wait for the bar it began on', () => {
  assert.deepEqual(dependencyDrawn('design', 'build'), { sourceId: 'build', targetId: 'design', type: 'depends_on' });
  assert.equal(dependencyDrawn('design', 'design'), undefined);
});
