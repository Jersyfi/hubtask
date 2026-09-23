// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import test from 'node:test';
import assert from 'node:assert/strict';

import { anchorOf, daysMoved, draggedTo, gridOf, placedBetween, scaleOf, windowOf } from './schedule.ts';

/** Monday, as `firstWeekday` answers for an account that says nothing. */
const MONDAY = 1;

test('a scale is one of three, and anything else is the middle one', () => {
  assert.equal(scaleOf('day'), 'day');
  assert.equal(scaleOf('month'), 'month');
  assert.equal(scaleOf('quarter'), 'week');
  assert.equal(scaleOf(undefined), 'week');
});

test('each scale draws its own length, and each begins before the anchor', () => {
  // A fortnight, ruled by days.
  assert.deepEqual(windowOf('2026-09-22', 'day', MONDAY), { from: '2026-09-19', to: '2026-10-02' });
  // Eight weeks, beginning on the reader's first weekday a week back - 2026-09-22 is a Tuesday.
  assert.deepEqual(windowOf('2026-09-22', 'week', MONDAY), { from: '2026-09-14', to: '2026-11-08' });
  // Six months, beginning with the one before.
  assert.deepEqual(windowOf('2026-09-22', 'month', MONDAY), { from: '2026-08-01', to: '2027-01-31' });
});

test('six months over a year end is still six months', () => {
  assert.deepEqual(windowOf('2026-12-15', 'month', MONDAY), { from: '2026-11-01', to: '2027-04-30' });
});

test('a window that holds no work opens on the work instead', () => {
  // The walk of 2026-09-22: two dated entries, one on the 25th and one on 2 October. The day
  // scale reaches both, so today is where it opens.
  assert.equal(anchorOf(['2026-09-25', '2026-10-02'], '2026-09-22', 'day', MONDAY), '2026-09-22');
  // A collection whose only dated entry is a season away opens on it rather than on today.
  assert.equal(anchorOf(['2027-03-04'], '2026-09-22', 'day', MONDAY), '2027-03-04');
  // Nothing dated at all: today, because there is nothing else to show.
  assert.equal(anchorOf([], '2026-09-22', 'month', MONDAY), '2026-09-22');
});

test('the nearest day wins, and the earlier of two equals', () => {
  // Fifteen days either side of a fortnight's window, so neither is in it: the earlier one, so a
  // window is not decided by the order of a list.
  assert.equal(anchorOf(['2026-10-04', '2026-09-04'], '2026-09-19', 'day', MONDAY), '2026-09-04');
});

test('the ruling is every day, every week, or every month', () => {
  const day = windowOf('2026-09-22', 'day', MONDAY);
  assert.equal(gridOf(day, 'day', MONDAY).length, 14);

  // Eight Mondays, and the window begins on one, so no line is added at its edge.
  const week = windowOf('2026-09-22', 'week', MONDAY);
  const lines = gridOf(week, 'week', MONDAY);
  assert.equal(lines.length, 8);
  assert.equal(lines[0], week.from);

  assert.deepEqual(gridOf(windowOf('2026-09-22', 'month', MONDAY), 'month', MONDAY), [
    '2026-08-01', '2026-09-01', '2026-10-01', '2026-11-01', '2026-12-01', '2027-01-01',
  ]);
});

test('a window that begins mid-week carries a line at its own edge', () => {
  // A day-scale window ruled by weeks: its first day is a Saturday, and a scale with no line until
  // the following Monday would not say where it starts.
  const window = { from: '2026-09-19', to: '2026-10-02' };
  assert.equal(gridOf(window, 'week', MONDAY)[0], '2026-09-19');
});

test('a travelled distance is days, and an unknown column moves nothing', () => {
  assert.equal(daysMoved(50, 24), 2);
  assert.equal(daysMoved(-36, 24), -2);
  // Under half a column is not a day yet.
  assert.equal(daysMoved(11, 24), 0);
  assert.equal(daysMoved(50, 0), 0);
  assert.equal(daysMoved(Number.NaN, 24), 0);
});

test('a bar moves both dates and keeps its length', () => {
  assert.deepEqual(
    draggedTo({ start: '2026-09-20', due: '2026-09-25' }, 'bar', 3),
    { start: '2026-09-23', due: '2026-09-28' },
  );
});

test('a bar drag invents no date the entry did not have', () => {
  // An entry with only a due date is a point, and moving it moves the point. It does not acquire a
  // start because somebody dragged it.
  assert.deepEqual(draggedTo({ due: '2026-09-25' }, 'bar', -2), { start: undefined, due: '2026-09-23' });
});

test('an end moves one date and may not pass the other', () => {
  assert.deepEqual(
    draggedTo({ start: '2026-09-20', due: '2026-09-25' }, 'start', 2),
    { start: '2026-09-22', due: '2026-09-25' },
  );
  // Dragged past the due date, the start stops on it: a span that ends before it begins has no
  // reading, and clamping is the honest answer to a gesture that asked for one.
  assert.deepEqual(
    draggedTo({ start: '2026-09-20', due: '2026-09-25' }, 'start', 9),
    { start: '2026-09-25', due: '2026-09-25' },
  );
  assert.deepEqual(
    draggedTo({ start: '2026-09-20', due: '2026-09-25' }, 'due', -9),
    { start: '2026-09-20', due: '2026-09-20' },
  );
});

test('a drag that went nowhere changes nothing', () => {
  const span = { start: '2026-09-20', due: '2026-09-25' };
  assert.equal(draggedTo(span, 'bar', 0), span);
});

test('a drag across the axis gives an undated entry both dates, either way round', () => {
  assert.deepEqual(placedBetween('2026-09-20', '2026-09-25'), { start: '2026-09-20', due: '2026-09-25' });
  assert.deepEqual(placedBetween('2026-09-25', '2026-09-20'), { start: '2026-09-20', due: '2026-09-25' });
  assert.deepEqual(placedBetween('2026-09-20', '2026-09-20'), { start: '2026-09-20', due: '2026-09-20' });
});
