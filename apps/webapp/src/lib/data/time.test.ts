// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { PlacedTimeRecord } from '@hubtask/sync-engine';

import {
  absenceGrid, actualsTree, addDays, addMonths, burnShare, elapsedMinutes, exportCsv, heatOf, heatmap, isChangeable, keyLabel,
  minorOf, minutesOf, minutesOfHours, minutesText, moneyText, monthDays, weekDays, weekGrid, weekStartOf,
} from './time.ts';

const record = (id: string, day: string, minutes: number, entry: string | null, workType = 'dev'): PlacedTimeRecord => ({
  id, account_id: 'a', entry_id: entry, work_type: workType, day, started_at: null, minutes, note: '', billable: true,
  source: 'MANUAL', corrects: null, created_at: '', updated_at: '', version: 1,
  collection_id: null, hub_id: null, entry_title: entry ? `Entry ${entry}` : '',
});

// UC-TIM-02 check 8: the week runs from Monday, as the server counts it.
test('a week starts on the Monday of the day it holds', () => {
  assert.equal(weekStartOf('2026-11-04'), '2026-11-02'); // a Wednesday
  assert.equal(weekStartOf('2026-11-02'), '2026-11-02');
  assert.equal(weekStartOf('2026-11-08'), '2026-11-02'); // a Sunday
  assert.equal(weekStartOf('2026-11'), undefined);
  assert.equal(addDays('2026-11-02', -7), '2026-10-26');
  assert.deepEqual(weekDays('2026-10-26').at(-1), '2026-11-01');
});

// UC-TIM-02 check 8: the grid holds the records on their entries by day; a correction counts signed.
test('the week grid sums a record into its entry, kind of work and day', () => {
  const days = weekDays('2026-11-02');
  const rows = weekGrid([
    record('1', '2026-11-02', 60, 'x'),
    record('2', '2026-11-02', 30, 'x'),
    record('3', '2026-11-04', 15, null, 'meeting'),
    record('4', '2026-11-03', -20, 'x'),
    record('5', '2026-11-09', 45, 'x'),
  ], days);
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[0]?.minutes, [90, -20, 0, 0, 0, 0, 0]);
  assert.equal(rows[0]?.total, 70);
  assert.equal(rows[1]?.entryId, null);
  assert.deepEqual(rows[1]?.minutes, [0, 0, 15, 0, 0, 0, 0]);
});

// UC-TIM-01 check 1, UC-TIM-03 check 3: 1 to 1440 minutes; a correction signed, never 0.
test('minutes are read as a person types them', () => {
  assert.equal(minutesOf('1:30', '.'), 90);
  assert.equal(minutesOf('45', '.'), 45);
  assert.equal(minutesOf('1,5', ','), 90);
  assert.equal(minutesOf('', '.'), null);
  assert.equal(minutesOf('0', '.'), undefined);
  assert.equal(minutesOf('24:01', '.'), undefined);
  assert.equal(minutesOf('-30', '.'), undefined);
  assert.equal(minutesOf('-30', '.', true), -30);
  assert.equal(minutesOf('−0:30', '.', true), -30);
  assert.equal(minutesText(90), '1:30');
  assert.equal(minutesText(-30), '−0:30');
});

// UC-TIM-01 check 8: the running timer shows its whole minutes.
test('a timer counts its whole minutes', () => {
  const start = '2026-11-02T09:00:00Z';
  assert.equal(elapsedMinutes(start, Date.parse('2026-11-02T09:51:59Z')), 51);
  assert.equal(elapsedMinutes(start, Date.parse('2026-11-02T08:00:00Z')), 0);
  assert.equal(elapsedMinutes(undefined, 0), 0);
});

// UC-TIM-02 check 4, UC-TIM-03 checks 3 and 4: an open or sent-back week's unlocked day changes;
// anything else is corrected.
test('a record changes only in an open week outside the locked period', () => {
  const days = [{ day: '2026-11-02', locked: true }, { day: '2026-11-03', locked: false }];
  assert.equal(isChangeable({ day: '2026-11-03', source: 'MANUAL' }, { state: 'OPEN', days }), true);
  assert.equal(isChangeable({ day: '2026-11-03', source: 'MANUAL' }, { state: 'REJECTED', days }), true);
  assert.equal(isChangeable({ day: '2026-11-02', source: 'MANUAL' }, { state: 'OPEN', days }), false);
  assert.equal(isChangeable({ day: '2026-11-03', source: 'MANUAL' }, { state: 'SUBMITTED', days }), false);
  assert.equal(isChangeable({ day: '2026-11-03', source: 'MANUAL' }, { state: 'APPROVED', days }), false);
});

// UC-TIM-04 checks 1 and 6: a report's keys are named for the reader.
test('a report key is named by what it is', () => {
  const names = {
    person: (id: string) => (id === 'p' ? 'Ana' : undefined), place: () => 'Web shop', workType: (k: string) => (k === 'dev' ? 'Development' : undefined),
    yes: 'Billable', no: 'Not billable', none: 'None',
  };
  assert.equal(keyLabel({ dimension: 'PERSON', value: 'p' }, names), 'Ana');
  assert.equal(keyLabel({ dimension: 'ENTRY', value: 'e', label: 'Checkout' }, names), 'Checkout');
  assert.equal(keyLabel({ dimension: 'ENTRY', value: 'e' }, names), 'e');
  assert.equal(keyLabel({ dimension: 'WORK_TYPE', value: 'dev' }, names), 'Development');
  assert.equal(keyLabel({ dimension: 'BILLABLE', value: 'false' }, names), 'Not billable');
  assert.equal(keyLabel({ dimension: 'HUB', value: '' }, names), 'None');
  assert.equal(keyLabel({ dimension: 'WEEK', value: '2026-11-02' }, names), '2026-11-02');
});

// UC-TIM-04 checks 2 and 6: the actuals table is the plan's tree, each parent above its entries.
test('the actuals are laid out as the plan’s tree', () => {
  const row = (entry_id: string, parent_id: string | null) => ({
    entry_id, parent_id, title: entry_id, done: false, estimate_hours: null, minutes: 0,
    rolled_estimate_hours: 0, rolled_minutes: 0, remaining_hours: 0, variance_hours: 0,
  });
  const tree = actualsTree([row('child', 'top'), row('top', null), row('grand', 'child'), row('orphan', 'gone')]);
  assert.deepEqual(tree.map((each) => [each.row.entry_id, each.depth]), [['top', 0], ['child', 1], ['grand', 2], ['orphan', 0]]);
});

// UC-TIM-05: the export's rows become a file a spreadsheet opens without running a note.
test('the export is written as CSV under its columns', () => {
  const csv = exportCsv({
    columns: ['day', 'minutes', 'note'],
    rows: [
      { day: '2026-11-02', minutes: 51, note: 'Checkout, form' } as never,
      { day: '2026-11-03', minutes: -30, note: '=SUM(A1)' } as never,
      { day: '2026-11-04', minutes: 5, note: 'say "hi"' } as never,
    ],
    truncated: false,
  });
  assert.equal(csv, 'day,minutes,note\r\n2026-11-02,51,"Checkout, form"\r\n2026-11-03,-30,\'=SUM(A1)\r\n2026-11-04,5,"say ""hi"""\r\n');
});

// UC-TIM-06 check 7: the heatmap lays people by weeks and colours a week as the server marked it.
test('the workload is a heatmap of people by weeks', () => {
  const cell = (account: string, week: string, over = false, under = false, known = true) => ({
    account_id: account, week, available_minutes: 240, allocated_minutes: 180, known, over, under,
  });
  const { weeks, rows } = heatmap([cell('ana', '2026-11-09'), cell('ana', '2026-11-02', true), cell('ben', '2026-11-02', false, true)]);
  assert.deepEqual(weeks, ['2026-11-02', '2026-11-09']);
  assert.equal(rows.length, 2);
  assert.equal(rows[1]?.cells[1], undefined);
  assert.equal(heatOf(rows[0]!.cells[0]!), 'over');
  assert.equal(heatOf(rows[1]!.cells[0]!), 'room');
  assert.equal(heatOf(cell('x', 'w')), 'fit');
  assert.equal(heatOf(cell('x', 'w', false, false, false)), 'unknown');
});

// UC-TIM-07 check 7: the month lays each person's absences on their days.
test('a month of absences lays each span on its days', () => {
  const days = monthDays('2026-11');
  assert.equal(days.length, 30);
  assert.equal(days[0], '2026-11-01');
  assert.equal(addMonths('2026-12', 1), '2027-01');
  assert.equal(addMonths('2026-01', -1), '2025-12');
  const leave = { id: '1', account_id: 'ana', from: '2026-10-30', to: '2026-11-03', kind: 'LEAVE' as const, half: false, version: 1 };
  const rows = absenceGrid([leave], days);
  assert.equal(rows[0]?.days[2]?.kind, 'LEAVE');
  assert.equal(rows[0]?.days[3], undefined);
});

// UC-TIM-08 check 8: money is typed in the major unit and shown in the reader's way.
test('money goes in the minor unit and comes back as the reader writes it', () => {
  assert.equal(minorOf('90', 'EUR', ','), 9000);
  assert.equal(minorOf('90,5', 'EUR', ','), 9050);
  assert.equal(minorOf('90.55', 'EUR', '.'), 9055);
  assert.equal(minorOf('90.555', 'EUR', '.'), undefined);
  assert.equal(minorOf('1500', 'JPY', '.'), 1500);
  assert.equal(minorOf('', 'EUR', '.'), null);
  assert.equal(minutesOfHours('7,5', ','), 450);
  assert.equal(minutesOfHours('7h', '.'), undefined);
  assert.match(moneyText(21000, 'EUR', 'en'), /210\.00/u);
  assert.equal(burnShare({ budget_minutes: 180, minutes: 210, budget_amount_minor: 0, amount_minor: 5 }, 'time'), 117);
  assert.equal(burnShare({ budget_minutes: 180, minutes: 210, budget_amount_minor: 0, amount_minor: 5 }, 'money'), null);
});
