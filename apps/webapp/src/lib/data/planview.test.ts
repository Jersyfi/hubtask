// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { headParts, isPlanView, keptRows, planChanges, planGroupingOf, planSetupOf, planViewDefault, scheduleOfEntries, slackEnd, tonesOf } from './planview.ts';

test('a plan view saved and read back is the same setup (UC-PLN-08)', () => {
  const setup = { ...planViewDefault('timeline'), groupBy: 'assignee' as const, baselineId: 'b1', columns: ['start', 'slip'], bars: { ...planViewDefault().bars, slack: false } };
  const grouping = planGroupingOf(setup);
  assert.ok(isPlanView({ layout: 'TIMELINE', grouping }));
  assert.deepEqual(planSetupOf({ grouping, visible_fields: setup.columns }), setup);
});

test('a view that is no plan view is not taken for one, and an unknown value falls back', () => {
  assert.equal(isPlanView({ layout: 'TIMELINE', grouping: { field: 'status' } }), false);
  assert.equal(isPlanView({ layout: 'KANBAN', grouping: { plan: {} } }), false);
  const read = planSetupOf({ grouping: { plan: { presentation: 'pie', group_by: 'moon' } } });
  assert.equal(read.presentation, 'gantt');
  assert.equal(read.groupBy, 'outline');
  assert.deepEqual(read.columns, planViewDefault().columns);
});

test('the changes are counted by part, the presentation among them', () => {
  const base = planViewDefault();
  assert.equal(planChanges(base, base), 0);
  assert.equal(planChanges({ ...base, presentation: 'timeline', line: 'who: me ' }, base), 2);
  assert.equal(planChanges({ ...base, bars: { ...base.bars, weekends: false } }, base), 1);
});

test('the head sentence says the end, the path, the slip, the tolerance and the undated', () => {
  const parts = headParts({ finish: '14 Dec', critical: 4, slipDays: 3, baselineName: 'Sept', toleranceDays: 10, undated: 2 });
  assert.deepEqual(parts.map((part) => part.code), ['app.planning.figure_ends', 'app.planning.figure_critical', 'app.planning.figure_slip', 'app.planning.figure_tolerance', 'app.planning.figure_undated']);
  assert.equal(parts[2]?.isWarning, true);
  assert.deepEqual(headParts({ critical: 0, undated: 0, slipDays: 0, baselineName: 'x' }).map((part) => part.code), ['app.planning.no_dates']);
});

test('what the line finds is kept with every phase above it; done hides', () => {
  const rows = [
    { id: 'p' },
    { id: 'a', parentId: 'p', isDone: true },
    { id: 'b', parentId: 'p' },
    { id: 'c' },
  ];
  assert.deepEqual(keptRows(rows, new Set(['b']), 'show').map((row) => row.id), ['p', 'b']);
  assert.deepEqual(keptRows(rows, undefined, 'hide').map((row) => row.id), ['p', 'b', 'c']);
});

test('slack ends after its working days, over a weekend', () => {
  assert.equal(slackEnd('2026-11-13', 1), '2026-11-16');
  assert.equal(slackEnd('2026-11-13', 0), undefined);
  assert.equal(slackEnd(undefined, 3), undefined);
});

test('a colour per value, in the order they come, round the palette', () => {
  const tones = tonesOf(['a', 'b', undefined, 'a', 'c', 'd', 'e', 'f', 'g', 'h']);
  assert.equal(tones.get('a'), 1);
  assert.equal(tones.get('b'), 2);
  assert.equal(tones.get('h'), 1);
});

test('without the module the plan is the entries own dates: a due alone is a point, no date is undated (E-16)', () => {
  const day = (instant: string) => instant.slice(0, 10);
  const plan = scheduleOfEntries(
    [
      { id: 'a', title: 'A', collection_id: 'c', start_at: '2026-11-02T00:00:00Z', due_at: '2026-11-06T00:00:00Z' },
      { id: 'b', title: 'B', collection_id: 'c', due_at: '2026-11-10T00:00:00Z', completion: { is_completed: true } },
      { id: 'c', title: 'C', collection_id: 'c', parent_id: 'a' },
    ],
    [{ id: 'l', waitedFor: 'a', waiting: 'b', isEarly: false }, { id: 'm', waitedFor: 'a', waiting: 'c', isEarly: true }],
    day,
    'UTC',
  );
  assert.equal(plan.finish, '2026-11-10');
  assert.deepEqual(plan.entries.map((each) => [each.item_id, each.start, each.finish, each.due_only, each.done, each.duration_days]), [
    ['a', '2026-11-02', '2026-11-06', false, false, 5],
    ['b', '2026-11-10', '2026-11-10', true, true, 1],
  ]);
  assert.equal(plan.entries[0]?.summary, true, 'a parent among the entries is a summary');
  assert.deepEqual(plan.undated, [{ item_id: 'c', title: 'C' }]);
  assert.deepEqual(plan.dependencies.map((each) => [each.predecessor_id, each.successor_id]), [['a', 'b']], 'an undated end draws no arrow');
  assert.deepEqual(plan.conflicts, []);
});
