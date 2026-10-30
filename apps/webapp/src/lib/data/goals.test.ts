// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { DashboardWidget, Goal } from '@hubtask/sync-engine';

import {
  alignmentTree, barLayout, clampBox, csvOf, figureRows, goalWrite, linePoints, matrixOf, moved, nextBox, parentChoices,
  percentText, pieSlices, readingOrder, resized, seriesOf, totalOf, widgetWrite,
} from './goals.ts';

const goal = (id: string, parent: string | null, progress: number | null = null): Goal => ({
  id, period_id: 'p', hub_id: null, parent_id: parent, title: `Goal ${id}`, description: '', owner_id: null, status: 'NONE',
  confidence: null, grade: null, key_results: [], entries: { total: 0, done: 0 }, child_count: 0, progress, version: 1,
});

// UC-GOL-01 check 9: the period's goals as an alignment tree, each under the goal it serves.
test('goals are laid out under the goals they serve', () => {
  const rows = alignmentTree([goal('a', null), goal('b', 'a'), goal('c', null), goal('d', 'b'), goal('e', 'unseen')]);
  assert.deepEqual(rows.map((row) => `${row.goal.id}${row.depth}`), ['a0', 'b1', 'd2', 'c0', 'e0']);
  // A cycle the server never answers is still drawn, once each.
  const cut = alignmentTree([goal('x', 'y'), goal('y', 'x')]);
  assert.deepEqual(cut.map((row) => row.goal.id).sort(), ['x', 'y']);
});

// UC-GOL-01 check 6: a goal is never offered as its own parent or under its own goals.
test('a goal is hung under nothing of its own', () => {
  const all = [goal('a', null), goal('b', 'a'), goal('c', 'b'), goal('d', null)];
  assert.deepEqual(parentChoices(all, 'b').map((g) => g.id), ['a', 'd']);
  assert.deepEqual(parentChoices(all, undefined).length, 4);
});

// UC-GOL-01 check 9: a progress is drawn as the server computed it, rounded down.
test('a share is a whole percentage, rounded down', () => {
  assert.equal(percentText(0.5), '50');
  assert.equal(percentText(0.999), '99');
  assert.equal(percentText(1), '100');
  assert.equal(percentText(0.29), '29');
  assert.equal(percentText(null), '');
  assert.equal(percentText(2), '100');
});

// UC-GOL-01 check 1, UC-GOL-02: a change keeps the key results by their ids and drops an emptied parent.
test('a goal is written back with its key results by id', () => {
  const read = {
    ...goal('a', 'p1'),
    key_results: [{ id: 'k', title: 'Customers', kind: 'NUMBER' as const, start: 0, target: 100, current: 50, unit: '', source: 'MANUAL' as const, view_id: null, entry_id: null, progress: 0.5, live: false }],
  };
  const write = goalWrite(read, { title: 'Win', parent_id: '' });
  assert.equal(write.title, 'Win');
  assert.equal(write.parent_id, undefined);
  assert.equal(write.key_results?.[0]?.id, 'k');
  assert.equal(write.key_results?.[0]?.view_id, undefined);
});

// UC-GOL-04 checks 1 and 8: a summary's rows and a GOALS widget's goals become one shape.
test('a widget’s figures become rows the chart and the table share', () => {
  const summary = figureRows([
    { group: 'false', period: null, count: 3, value: null },
    { group: 'true', period: null, count: 1, value: null },
  ]);
  assert.deepEqual(summary.map((row) => [row.label, row.value]), [['false', 3], ['true', 1]]);
  assert.equal(totalOf(summary), 4);
  const money = figureRows([{ group: null, period: null, count: 2, value: '12.50' }]);
  assert.equal(money[0]?.value, 12.5);
  assert.equal(money[0]?.text, '12.50');
  const goals = figureRows([{ goal_id: 'g', title: 'Grow', status: 'AT_RISK', progress: 0.3 }]);
  assert.deepEqual([goals[0]?.label, goals[0]?.value, goals[0]?.status], ['Grow', 30, 'AT_RISK']);
  const matrix = matrixOf(figureRows([
    { group: 'a', period: '2026-11-02', count: 1, value: null },
    { group: 'b', period: '2026-10-26', count: 2, value: null },
  ]));
  assert.deepEqual(matrix.columns, ['2026-10-26', '2026-11-02']);
  assert.deepEqual(matrix.cells, [['', '1'], ['2', '']]);
});

// UC-GOL-04 check 8: the chart draws what its table lists, in the design system's colours.
test('bars, a line and a pie are drawn from the rows', () => {
  const rows = figureRows([
    { group: 'a', period: null, count: 2, value: null },
    { group: 'b', period: null, count: 4, value: null },
  ]);
  const bars = barLayout(rows, 100, 50);
  assert.equal(bars[1]?.height, 50);
  assert.equal(bars[0]?.height, 25);
  assert.equal(bars[0]?.series, 'blue');
  assert.equal(seriesOf(10), 'blue');
  assert.equal(linePoints(rows, 100, 50), '0,25 100,0');
  const slices = pieSlices(rows);
  assert.equal(slices.length, 2);
  assert.ok(Math.abs((slices[0]?.share ?? 0) - 1 / 3) < 1e-9);
  assert.equal(pieSlices(figureRows([{ group: 'a', period: null, count: 0, value: null }])).length, 0);
});

// UC-GOL-04 check 8: widgets are edited on the twelve-column grid the server holds them to.
test('a widget stays on the twelve-column grid', () => {
  assert.deepEqual(clampBox({ x: 10, y: -1, w: 6, h: 0 }), { x: 6, y: 0, w: 6, h: 1 });
  assert.deepEqual(moved({ x: 0, y: 0, w: 6, h: 4 }, 8, 1), { x: 6, y: 1, w: 6, h: 4 });
  assert.deepEqual(resized({ x: 8, y: 0, w: 4, h: 4 }, 4, -1), { x: 4, y: 0, w: 8, h: 3 });
  assert.deepEqual(nextBox([{ x: 0, y: 0, w: 6, h: 4 }, { x: 6, y: 2, w: 6, h: 4 }]), { x: 0, y: 6, w: 6, h: 4 });
  assert.deepEqual(readingOrder([{ x: 6, y: 0, w: 1, h: 1 }, { x: 0, y: 1, w: 1, h: 1 }, { x: 0, y: 0, w: 1, h: 1 }]).map((b) => `${b.x}${b.y}`), ['00', '60', '01']);
  const widget: DashboardWidget = { id: 'w', title: 'Open', chart: 'NUMBER', source: 'VIEW', view_id: 'v', query: null, goal_ids: [], x: 0, y: 0, w: 3, h: 2 };
  assert.deepEqual(widgetWrite(widget), { id: 'w', title: 'Open', chart: 'NUMBER', source: 'VIEW', view_id: 'v', x: 0, y: 0, w: 3, h: 2 });
});

// UC-GOL-04 check 8: a widget's table is handed over as CSV, a formula never running.
test('a table becomes CSV with formulas defused', () => {
  assert.equal(csvOf(['Group', 'Count'], [['=SUM(A1)', 2], ['a, b', 3]]), "Group,Count\r\n'=SUM(A1),2\r\n\"a, b\",3\r\n");
});
