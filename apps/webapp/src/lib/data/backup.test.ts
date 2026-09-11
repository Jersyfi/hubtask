// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Reading a backup schedule's rule, and the generation plan behind it.
//
// The property that matters is not that the common rules read correctly — it is that an uncommon
// one is reported as partially read rather than described wrongly. A sentence that quietly drops
// `BYSETPOS=-1` describes a schedule that does not exist, and nobody notices until the night it
// does not run.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { generations, readRule } from './backup.ts';

test('the rule the contract gives as its own example reads as what it means', () => {
  const reading = readRule('FREQ=DAILY;BYHOUR=3;BYMINUTE=0');
  assert.equal(reading.frequency, 'DAILY');
  assert.equal(reading.interval, 1);
  assert.deepEqual(reading.at, { hour: 3, minute: 0 });
  assert.equal(reading.partial, false);
});

test('a weekly rule keeps its days, and the RRULE: prefix is not one of them', () => {
  const reading = readRule('RRULE:FREQ=WEEKLY;BYDAY=SU,WE;INTERVAL=2');
  assert.equal(reading.frequency, 'WEEKLY');
  assert.equal(reading.interval, 2);
  assert.deepEqual(reading.weekdays, ['SU', 'WE']);
  assert.equal(reading.partial, false);
});

test('an hour without a minute fires on the hour', () => {
  assert.deepEqual(readRule('FREQ=DAILY;BYHOUR=3').at, { hour: 3, minute: 0 });
});

test('a rule that pins no time says so rather than inventing midnight', () => {
  assert.equal(readRule('FREQ=WEEKLY;BYDAY=SU').at, undefined);
});

test('an ordinal weekday is not read as that weekday', () => {
  // `BYSETPOS`, `2MO` and friends: the sentence would name the wrong day three weeks in four.
  const reading = readRule('FREQ=MONTHLY;BYDAY=2MO');
  assert.equal(reading.partial, true, 'an ordinal weekday must not read as a plain one');
  assert.deepEqual(reading.weekdays, []);
});

test('a part this reading does not cover marks the rule as partly read', () => {
  assert.equal(readRule('FREQ=MONTHLY;BYDAY=FR;BYSETPOS=-1').partial, true);
  assert.equal(readRule('FREQ=DAILY;COUNT=5').partial, true);
  assert.equal(readRule('FREQ=HOURLY').partial, true, 'a frequency with no sentence is not read');
});

test('two hours are two schedules and neither is "the" time', () => {
  assert.equal(readRule('FREQ=DAILY;BYHOUR=3,15').partial, true);
  assert.equal(readRule('FREQ=DAILY;BYHOUR=3,15').at, undefined);
});

test('an empty rule reads as nothing rather than as a daily one', () => {
  const reading = readRule('  ');
  assert.equal(reading.frequency, undefined);
  assert.equal(reading.partial, true);
});

test('WKST changes no sentence and does not make a rule partly read', () => {
  assert.equal(readRule('FREQ=WEEKLY;BYDAY=MO;WKST=SU').partial, false);
});

test('the generation plan lists what is kept and skips what is not', () => {
  const lines = generations({ keep_last: 7, keep_daily: 14, keep_weekly: 0, keep_monthly: 12 });
  assert.deepEqual(
    lines.map((line) => line.kind),
    ['last', 'daily', 'monthly'],
  );
  assert.equal(lines[0]?.keep, 7);
});

test('min_keep is not a generation', () => {
  // It is a floor under all of them — `backup-restore.md` §6 — and listing it beside the others
  // would read as a sixth kind of archive.
  const lines = generations({ keep_last: 3, min_keep: 3 });
  assert.deepEqual(lines.map((line) => line.kind), ['last']);
});

test('a schedule with no plan at all lists nothing', () => {
  assert.deepEqual(generations(undefined), []);
});
