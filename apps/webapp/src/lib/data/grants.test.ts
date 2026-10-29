// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The rows of Apps you have allowed, as UC-SYN-07/3 and ADR-0096 draw them.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { grantOrder, grantUse, initialsOf } from './grants.ts';

test('the installed apps come first, each group by name', () => {
  const rows = grantOrder([
    { client_name: 'Timesheet reporter', app_platform: null },
    { client_name: "Lena's phone", app_platform: 'android' },
    { client_name: 'Alarm bridge' },
    { client_name: 'LENA-LAPTOP', app_platform: 'windows' },
  ]);
  assert.deepEqual(rows.map((row) => row.client_name), ['LENA-LAPTOP', "Lena's phone", 'Alarm bridge', 'Timesheet reporter']);
});

test('a third-party app is marked by its first and last word', () => {
  assert.equal(initialsOf('Timesheet reporter'), 'TR');
  assert.equal(initialsOf('alexa'), 'A');
  assert.equal(initialsOf('  '), '');
});

test('the last use is today with its time, days within the week, the day further back', () => {
  const now = new Date(2026, 9, 28, 15, 0).getTime();
  const at = (day: number, hour: number) => new Date(2026, 9, day, hour, 32).toISOString();
  assert.deepEqual(grantUse(at(28, 14), 'en', now), { kind: 'today', at: at(28, 14) });
  // Last night is yesterday, though fewer than twenty-four hours have passed.
  assert.deepEqual(grantUse(at(27, 23), 'en', now), { kind: 'days', phrase: 'yesterday' });
  assert.deepEqual(grantUse(at(26, 9), 'en', now), { kind: 'days', phrase: '2 days ago' });
  assert.deepEqual(grantUse(at(20, 9), 'en', now), { kind: 'day', at: at(20, 9) });
  assert.deepEqual(grantUse(null, 'en', now), { kind: 'never' });
  assert.deepEqual(grantUse('not a time', 'en', now), { kind: 'never' });
});
