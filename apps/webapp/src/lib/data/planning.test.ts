// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import type { BaselineComparison, PlanSchedule, PlanScheduleEntry } from '@hubtask/sync-engine';

import { PLAN_CSV_HEADS, dependencyDrawn, fitWindow, ganttRows, inTreeOrder, lagOf, planCsvRows, planRolePath, planXlsxOf, projectFormatOf, projectPlanOf, proposedMoves } from './planning.ts';

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

// UC-PLN-09 check 6: the plan saved as a table the import reads back - the outline as its level,
// a milestone marked, and what each row waits on by the row's number with its kind and lag.
test('the plan as a CSV keeps the outline, the milestones and the predecessors', () => {
  const plan = schedule([
    entry('phase', { summary: true, finish: '2026-11-20' }),
    entry('design', { parent_id: 'phase' }),
    entry('build', { parent_id: 'phase', critical: true }),
    entry('launch', { milestone: true, start: '2026-11-20', finish: '2026-11-20' }),
  ]);
  plan.dependencies = [
    { link_id: 'l1', type: 'blocks', predecessor_id: 'design', successor_id: 'build', dependency_kind: 'SS', lag_days: 2 },
    { link_id: 'l2', type: 'blocks', predecessor_id: 'build', successor_id: 'launch', dependency_kind: 'FS', lag_days: 0 },
    { link_id: 'l3', type: 'blocks', predecessor_id: 'design', successor_id: 'launch', dependency_kind: 'FF', lag_days: -1 },
  ];
  const rows = planCsvRows(ganttRows(plan, undefined, (instant) => instant.slice(0, 10)), plan);
  assert.equal(PLAN_CSV_HEADS.length, rows[0]?.length);
  assert.deepEqual(rows.map((row) => [row[0], row[4], row[5], row[6]]), [
    [1, 1, '', ''], [2, 2, '', ''], [3, 2, '', '2SS+2d'], [4, 1, 'yes', '3;2FF-1d'],
  ]);
  assert.equal(rows[3]?.[2], '', 'a milestone has no start');
  assert.equal(rows[2]?.[9], 'yes');
  const sheet = new TextDecoder().decode(planXlsxOf(ganttRows(plan, undefined, (instant) => instant.slice(0, 10)), plan));
  assert.ok(sheet.includes('xl/worksheets/sheet1.xml'), 'the workbook is a spreadsheet');
  for (const head of PLAN_CSV_HEADS) assert.ok(sheet.includes(`<t xml:space="preserve">${head}</t>`), `the workbook carries ${head}`);
  assert.ok(sheet.includes('<t xml:space="preserve">3;2FF-1d</t>'), 'the workbook carries the predecessors the CSV carries');
});

test("a project file's plan is said by what a planner counts, in the tense of the step", () => {
  const words = {
    t: (code: string, params: Record<string, string> = {}) => [code.replace(/^app\.(import|planning)\./u, ''), ...Object.values(params)].join(' '),
    list: (parts: readonly string[]) => parts.join(' + '),
  };
  const answer = { tasks: 3, work_packages: 0, activities: 2, milestones: 1, dependencies: 4, flattened: 1 };
  assert.deepEqual(projectPlanOf(answer, true, words), [
    { tone: 'add', text: 'plan_creates_dry count_tasks 3 + count_activities 2 + count_milestones 1 + count_dependencies 4' },
    { tone: 'change', text: 'import_flattened_dry 1' },
  ]);
  assert.deepEqual(projectPlanOf(answer, false, words).map((line) => line.text.split(' ')[0]), ['plan_creates_done', 'import_flattened_done']);
  assert.deepEqual(projectPlanOf({ tasks: 0, work_packages: 0, activities: 0, milestones: 0, dependencies: 0, flattened: 0 }, true, words), []);
});

test("a project file's format is its name's, else its first character's", () => {
  assert.equal(projectFormatOf('plan.XML', ''), 'MSPROJECT_XML');
  assert.equal(projectFormatOf('plan.csv', '<x'), 'CSV');
  assert.equal(projectFormatOf('plan', '  <?xml'), 'MSPROJECT_XML');
  assert.equal(projectFormatOf('plan', 'ID,Name'), 'CSV');
});

// A hub's plan asks the hub's memberships, never a collection's under the hub's id.
test('planRolePath names the hub for a hub and waits for a place not read', () => {
  assert.equal(planRolePath('h', undefined), undefined);
  assert.deepEqual(planRolePath('h', { type: 'HUB', parent_id: null }), { hubId: 'h' });
  assert.deepEqual(planRolePath('c', { type: 'COLLECTION', parent_id: 'h' }), { hubId: 'h', collectionId: 'c' });
});

// user-experience.md §7.5: an entry with only a due date is placed as a milestone, a point rather
// than a one-day bar it never had.
test('an entry with only a due date is drawn as a milestone', () => {
  const rows = ganttRows(schedule([entry('due', { start: '2026-11-04', finish: '2026-11-04', due_only: true }), entry('span')]), undefined, (instant) => instant.slice(0, 10));
  assert.equal(rows[0]?.kind, 'milestone');
  assert.equal(rows[1]?.kind, undefined);
});

// user-experience.md §7.5: the chart opens on the plan's whole range, at the finest scale that
// holds it, a little room on each side; a plan without dates has no range to fit.
test('the window fits the plan, the scale chosen by its length', () => {
  assert.equal(fitWindow([]), undefined);
  assert.deepEqual(fitWindow([{ start: '2026-11-02', due: '2026-11-20' }]), { scale: 'day', from: '2026-11-01', to: '2026-11-21' });
  const quarter = fitWindow([{ start: '2026-01-05', due: '2026-03-01' }, { start: '2026-02-01', due: '2026-04-30' }]);
  assert.equal(quarter?.scale, 'month');
  assert.ok(quarter && quarter.from < '2026-01-05' && quarter.to > '2026-04-30');
  assert.equal(fitWindow([{ start: '2026-01-01', due: '2028-12-31' }])?.scale, 'year');
});
