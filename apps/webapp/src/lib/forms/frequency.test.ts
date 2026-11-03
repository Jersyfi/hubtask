// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { frequencyOf, ruleOf } from './frequency.ts';

test('a daily, weekly or monthly rule at a fixed time is read into the choices', () => {
  assert.deepEqual(frequencyOf('FREQ=DAILY;BYHOUR=3;BYMINUTE=0'), { freq: 'DAILY', weekday: 'MO', monthDay: 1, hour: 3, minute: 0 });
  assert.deepEqual(frequencyOf('freq=weekly;byday=su;byhour=22;byminute=30'), { freq: 'WEEKLY', weekday: 'SU', monthDay: 1, hour: 22, minute: 30 });
  assert.deepEqual(frequencyOf('FREQ=MONTHLY;BYMONTHDAY=15;BYHOUR=7;BYMINUTE=5;INTERVAL=1'), { freq: 'MONTHLY', weekday: 'MO', monthDay: 15, hour: 7, minute: 5 });
});

test('a rule the choices cannot say stays the rule itself, never rewritten', () => {
  // A rule read into the choices and written back must be the same schedule; anything the
  // choices would drop - an interval, two days, no fixed hour - is left to the raw field.
  for (const rule of [
    '',
    'FREQ=DAILY',
    'FREQ=DAILY;BYHOUR=3',
    'FREQ=DAILY;INTERVAL=2;BYHOUR=3;BYMINUTE=0',
    'FREQ=WEEKLY;BYDAY=MO,TH;BYHOUR=9;BYMINUTE=0',
    'FREQ=WEEKLY;BYHOUR=9;BYMINUTE=0',
    'FREQ=MONTHLY;BYDAY=-1FR;BYHOUR=9;BYMINUTE=0',
    'FREQ=YEARLY;BYHOUR=9;BYMINUTE=0',
    'FREQ=DAILY;BYHOUR=24;BYMINUTE=0',
    'FREQ=DAILY;BYHOUR=3;BYMINUTE=0;COUNT=4',
    'FREQ=DAILY;BYDAY=MO;BYHOUR=3;BYMINUTE=0',
    'not a rule',
  ]) {
    assert.equal(frequencyOf(rule), undefined, rule);
  }
});

test('the choices write the rule they were read from', () => {
  for (const rule of ['FREQ=DAILY;BYHOUR=3;BYMINUTE=0', 'FREQ=WEEKLY;BYDAY=SU;BYHOUR=22;BYMINUTE=30', 'FREQ=MONTHLY;BYMONTHDAY=28;BYHOUR=0;BYMINUTE=15']) {
    const read = frequencyOf(rule);
    assert.ok(read, rule);
    assert.equal(ruleOf(read), rule);
  }
});
