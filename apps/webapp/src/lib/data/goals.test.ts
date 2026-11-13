// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { DashboardWidget, Goal } from '@hubtask/sync-engine';

import {
  alignmentTree, barLayout, currentQuarter, needingALook, hubOf, clampBox, csvOf, figureRows, goalWrite, linePoints, matrixOf, moved, nextBox, parentChoices,
  folded, leftRows, lineDots, percentText, scaleTop, pieSlices, readingOrder, resized, runningRows, seriesBars, seriesLines, seriesOf, seriesTable, totalOf,
  widgetWrite, numberFigure, targetState, hbars, hpairs, ringParts, matrixSteps, textPieces, conditionsOf, inheritedFilter, listFilter, searchFilter,
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

// UC-GOL-04 check 8: a group is named in words and a period is the reader's date, sorted as the
// server's values sort (user-experience.md §5.6, §5.10).
test('a widget’s groups and periods are named, never keys or ISO dates', () => {
  const naming = { group: (value: string) => ({ bug: 'Bug', change_request: 'Change' })[value] ?? value, period: (day: string) => `day ${day.slice(8)}/${day.slice(5, 7)}` };
  const rows = figureRows([
    { group: 'change_request', period: null, count: 2, value: null },
    { group: 'bug', period: null, count: 1, value: null },
    { group: null, period: null, count: 1, value: null },
  ], naming);
  assert.deepEqual(rows.map((row) => row.label), ['Change', 'Bug', '']);
  const weeks = figureRows([{ group: null, period: '2026-11-02', count: 8, value: null }], naming);
  assert.equal(weeks[0]?.label, 'day 02/11');
  const matrix = matrixOf(figureRows([
    { group: 'bug', period: '2026-11-02', count: 1, value: null },
    { group: 'bug', period: '2026-10-26', count: 2, value: null },
  ], naming));
  assert.deepEqual(matrix.rows, ['Bug']);
  assert.deepEqual(matrix.columns, ['day 26/10', 'day 02/11'], 'periods sort by their day, not by their words');
  const table = seriesTable([{ name: 'a', rows: figureRows([
    { group: null, period: '2026-11-02', count: 1, value: null },
    { group: null, period: '2026-10-26', count: 2, value: null },
  ], naming) }, { name: 'left', rows: leftRows([{ group: null, period: '2026-10-26', value: '3' }], naming) }]);
  assert.deepEqual(table.labels, ['day 26/10', 'day 02/11']);
  assert.deepEqual(table.series[1]?.texts, ['3', '']);
});

// A line of one period draws a dot, and the value axis reaches the highest value.
test('a line of one period is a dot, and the scale has a top', () => {
  const one = figureRows([{ group: null, period: '2026-11-02', count: 8, value: null }]);
  assert.equal(lineDots(linePoints(one, 400, 160)), 'M200,0h0');
  assert.equal(lineDots('0,25 100,0'), 'M0,25h0M100,0h0');
  assert.equal(lineDots(''), '');
  assert.equal(scaleTop([3, undefined, 8]), 8);
  assert.equal(scaleTop([]), 0);
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
  assert.equal(bars[1]?.series, '1', 'one series is one colour (E-7)');
  assert.equal(seriesOf(1), '2');
  assert.equal(seriesOf(7), '1', 'the seven chart colours repeat in their fixed order');
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
  const widget: DashboardWidget = { id: 'w', title: 'Open', chart: 'NUMBER', source: 'VIEW', view_id: 'v', query: null, goal_ids: [], compare: null, cumulative: false, x: 0, y: 0, w: 3, h: 2 };
  assert.deepEqual(widgetWrite(widget), { id: 'w', title: 'Open', chart: 'NUMBER', source: 'VIEW', view_id: 'v', x: 0, y: 0, w: 3, h: 2 });
});

// UC-GOL-04 check 8: a widget's table is handed over as CSV, a formula never running.
test('a table becomes CSV with formulas defused', () => {
  assert.equal(csvOf(['Group', 'Count'], [['=SUM(A1)', 2], ['a, b', 3]]), "Group,Count\r\n'=SUM(A1),2\r\n\"a, b\",3\r\n");
});

// UC-GOL-04 check 15: created beside resolved, each as its running total, and what is left.
test('two series and what is left line up by period', () => {
  const created = runningRows(figureRows([
    { group: null, period: '2026-10-01', count: 5, value: null, running: '5' },
    { group: null, period: '2026-10-03', count: 2, value: null, running: '7' },
  ]));
  const resolved = runningRows(figureRows([
    { group: null, period: '2026-10-02', count: 1, value: null, running: '1' },
    { group: null, period: '2026-10-03', count: 4, value: null, running: '5' },
  ]));
  const left = leftRows([{ group: null, period: '2026-10-01', value: '5' }, { group: null, period: '2026-10-02', value: '4' }, { group: null, period: '2026-10-03', value: '2' }]);
  assert.deepEqual(created.map((row) => row.value), [5, 7]);
  const table = seriesTable([{ name: 'Created', rows: created, isRunning: true }, { name: 'Resolved', rows: resolved, isRunning: true }, { name: 'Left', rows: left }]);
  assert.deepEqual(table.labels, ['2026-10-01', '2026-10-02', '2026-10-03']);
  assert.deepEqual(table.series[0]?.texts, ['5', '5', '7'], 'a running total carries over a day without a row');
  assert.deepEqual(table.series[1]?.texts, ['', '1', '5']);
  assert.deepEqual(table.series[2]?.values, [5, 4, 2]);
  const lines = seriesLines(table, 100, 70);
  assert.equal(lines.length, 3);
  assert.equal(lines[1], '50,60 100,20');
  const bars = seriesBars(table, 300, 70);
  assert.equal(bars.length, 9);
  assert.equal(bars[0]?.height, 50);
  const plain = seriesTable([{ name: 'a', rows: figureRows([{ group: 'x', period: null, count: 1, value: null }]) }, { name: 'b', rows: [] }]);
  assert.deepEqual(plain.series[1]?.texts, ['']);
});

test('a widget keeps its second series and running total when written back', () => {
  const widget = {
    id: 'w', title: 'Flow', chart: 'LINE', source: 'QUERY', view_id: null, query: { aggregate: {} }, goal_ids: [],
    compare: { first_title: 'Created', title: 'Resolved', query: { aggregate: {} } }, cumulative: true, x: 0, y: 0, w: 6, h: 4,
  } as DashboardWidget;
  const write = widgetWrite(widget);
  assert.equal(write.cumulative, true);
  assert.equal(write.compare?.title, 'Resolved');
  assert.equal(widgetWrite({ ...widget, compare: null, cumulative: false }).compare, undefined);
});

// A workspace goal or dashboard reads as the workspace's, never as a hub nobody reads.
test('hubOf reads null and the nil UUID as the workspace', () => {
  assert.equal(hubOf({ hub_id: null }), null);
  assert.equal(hubOf({}), null);
  assert.equal(hubOf(undefined), null);
  assert.equal(hubOf({ hub_id: '00000000-0000-0000-0000-000000000000' }), null);
  assert.equal(hubOf({ hub_id: '01a0e2e0-0000-7000-8000-000000000001' }), '01a0e2e0-0000-7000-8000-000000000001');
});

// user-experience.md §4, §7.7: the current quarter is the period offered first, and the head says
// how many goals need a look.
test('the current quarter runs from its first to its last day', () => {
  assert.deepEqual(currentQuarter('2026-11-04'), { year: 2026, quarter: 4, start_on: '2026-10-01', end_on: '2026-12-31' });
  assert.deepEqual(currentQuarter('2027-01-01'), { year: 2027, quarter: 1, start_on: '2027-01-01', end_on: '2027-03-31' });
  assert.deepEqual(currentQuarter('2028-05-31'), { year: 2028, quarter: 2, start_on: '2028-04-01', end_on: '2028-06-30' });
  assert.deepEqual(currentQuarter('2026-09-30'), { year: 2026, quarter: 3, start_on: '2026-07-01', end_on: '2026-09-30' });
});

test('goals at risk or off track need a look', () => {
  const of = (status: Goal['status']) => ({ status }) as Goal;
  assert.equal(needingALook([of('ON_TRACK'), of('AT_RISK'), of('OFF_TRACK'), of('DONE'), of('NONE')]), 2);
  assert.equal(needingALook([]), 0);
});

// UC-GOL-04 check 19 (E-7): a chart draws at most as many parts as it may tell apart - the rest
// is one row, *Others*, holding their sum.
test('a chart folds what is past its parts into one row', () => {
  const rows = figureRows(['a', 'b', 'c', 'd', 'e', 'f'].map((group, i) => ({ group, period: null, count: i + 1, value: null })));
  const ring = folded(rows, 4, 'Others');
  assert.deepEqual(ring.map((row) => row.label), ['f', 'e', 'd', 'Others']);
  assert.equal(ring[3]?.value, 6);
  assert.equal(folded(rows, 8, 'Others').length, 6);
});

// UC-GOL-04 check 19: each of the nine kinds is drawn in its own shape (wcat).
test('a number over periods is the last period, its change and eight periods as a line', () => {
  const rows = figureRows(['2026-01-05', '2026-01-12', '2026-01-19'].map((period, i) => ({ period, count: [30, 44, 38][i] })));
  const figure = numberFigure(rows);
  assert.equal(figure.text, '38');
  assert.equal(figure.change, -6);
  assert.equal(figure.spark.split(' ').length, 3);
  assert.deepEqual(numberFigure(figureRows([{ count: 4 }, { count: 5 }])), { value: 9, text: '9', spark: '' });
});

test('a target with a warning line above it means lower is better, below it higher', () => {
  assert.equal(targetState(28, 30, 40), 'met');
  assert.equal(targetState(35, 30, 40), 'between');
  assert.equal(targetState(41, 30, 40), 'warn');
  assert.equal(targetState(92, 90, 80), 'met');
  assert.equal(targetState(75, 90, 80), 'warn');
  assert.equal(targetState(5, undefined, 3), undefined);
});

test('bars lie down at most eight, the longest full, the rest as Sonstige', () => {
  const rows = figureRows(Array.from({ length: 10 }, (_, i) => ({ group: `g${i}`, count: 10 - i })));
  const bars = hbars(rows, 'Sonstige');
  assert.equal(bars.length, 8);
  assert.equal(bars[0]!.share, 100);
  assert.equal(bars[7]!.row.label, 'Sonstige');
  const pairs = hpairs(seriesTable([{ name: 'a', rows: figureRows([{ group: 'x', count: 14 }]) }, { name: 'b', rows: figureRows([{ group: 'x', count: 11 }]) }]));
  assert.deepEqual(pairs[0]!.texts, ['14', '11']);
  assert.equal(pairs[0]!.shares.reduce((a, b) => a + b, 0), 100);
});

test('a ring holds at most four parts, each starting where the last ended', () => {
  const parts = ringParts(figureRows([{ group: 'a', count: 47 }, { group: 'b', count: 9 }, { group: 'c', count: 4 }, { group: 'd', count: 1 }, { group: 'e', count: 1 }, { group: 'f', count: 0 }]), 'Sonstige');
  assert.equal(parts.length, 4);
  assert.equal(parts[0]!.start, 0);
  assert.equal(parts[3]!.row.label, 'Sonstige');
  assert.equal(parts[3]!.row.value, 2);
  assert.deepEqual(parts.map((part) => part.series), ['1', '2', '3', '4']);
});

test('a matrix cell steps from one to five over the whole matrix, nothing as zero', () => {
  assert.deepEqual(matrixSteps({ rows: ['a'], columns: ['1', '2', '3'], cells: [['10', '', '2']] }), [[5, 0, 1]]);
});

test('a text is bold and links to app paths, never a link elsewhere', () => {
  assert.deepEqual(textPieces('Zuerst **SA-7**, siehe [Plan](/pages/1) oder [hier](https://x.example).'), [
    { kind: 'plain', text: 'Zuerst ' }, { kind: 'bold', text: 'SA-7' }, { kind: 'plain', text: ', siehe ' },
    { kind: 'link', text: 'Plan', href: '/pages/1' }, { kind: 'plain', text: ' oder ' }, { kind: 'plain', text: 'hier' }, { kind: 'plain', text: '.' },
  ]);
  assert.equal(textPieces('[x](//evil)')[0]!.kind, 'plain');
});

test('a figure widget writes its target and warning line back, another kind never', () => {
  const base = { id: 'w', title: 'Offen', source: 'QUERY', view_id: null, query: { aggregate: { function: 'COUNT' } }, goal_ids: [], compare: null, cumulative: false, text: '', column: '', released: [], x: 0, y: 0, w: 3, h: 2 };
  const number = widgetWrite({ ...base, chart: 'NUMBER', target: 30, warn_at: 40 } as unknown as DashboardWidget);
  assert.equal(number.target, 30);
  assert.equal(number.warn_at, 40);
  const bars = widgetWrite({ ...base, chart: 'BAR', target: 30, warn_at: null } as unknown as DashboardWidget);
  assert.equal(bars.target, undefined);
});

// UC-GOL-04 check 18: a widget takes the dashboard's filter but the conditions on fields it released,
// as the server does; a condition ORed together is one and stays whole.
test('a widget inherits the dashboard filter but what it released', () => {
  const hub = { field: 'hub_id', op: 'EQ', value: 'h1' };
  const open = { field: 'is_completed', op: 'EQ', value: false };
  const either = { op: 'OR', nodes: [{ field: 'status', op: 'EQ', value: 'todo' }, { field: 'status', op: 'EQ', value: 'done' }] };
  const filter = { op: 'AND', nodes: [hub, open, either] };
  assert.equal(conditionsOf(filter).length, 3);
  assert.deepEqual(inheritedFilter(filter, ['is_completed', 'status']), { op: 'AND', nodes: [hub, either] });
  assert.deepEqual(inheritedFilter(hub, ['hub_id']), undefined);
  assert.deepEqual(inheritedFilter(filter, []), filter);
});

// UC-GOL-04 check 22: *Als Liste öffnen* hands over the inherited filter and the widget's own,
// without the hub the collection already lies in.
test('the list a widget opens carries its own line and what it inherits', () => {
  const hub = { field: 'hub_id', op: 'EQ', value: 'h1' };
  const open = { field: 'is_completed', op: 'EQ', value: false };
  const mine = { field: 'priority', op: 'EQ', value: 'high' };
  assert.deepEqual(listFilter({ op: 'AND', nodes: [hub, open] }, [], mine, 'h1'), { op: 'AND', nodes: [open, mine] });
  assert.deepEqual(listFilter({ op: 'AND', nodes: [hub, open] }, ['is_completed'], undefined, 'h2'), hub);
  assert.equal(listFilter(undefined, [], undefined, 'h1'), undefined);
});

// UC-GOL-04 check 22: a widget counting a hub or the whole workspace opens the search with its
// line - the hub it counts in named once, beside what it inherits and its own.
test('the search a widget opens names the hub it counts in once', () => {
  const hub = { field: 'hub_id', op: 'EQ', value: 'h1' };
  const open = { field: 'is_completed', op: 'EQ', value: false };
  const mine = { field: 'priority', op: 'EQ', value: 'high' };
  assert.deepEqual(searchFilter(open, [], mine, 'h1'), { op: 'AND', nodes: [hub, open, mine] });
  assert.deepEqual(searchFilter({ op: 'AND', nodes: [hub, open] }, [], undefined, 'h1'), { op: 'AND', nodes: [hub, open] });
  assert.deepEqual(searchFilter(open, [], undefined, undefined), open);
  assert.equal(searchFilter(undefined, [], undefined, undefined), undefined);
});
