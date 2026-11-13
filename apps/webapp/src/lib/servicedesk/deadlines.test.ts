// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A ticket's deadlines by urgency and the head's one chip (ET-6, UC-SVC-02 check 6).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EntrySlaGoal } from '@hubtask/sync-engine';
import { byUrgency, chipOf, shareOf, timeOf, urgencyOf } from './deadlines.ts';

const now = Date.parse('2026-11-12T12:00:00Z');
const goal = (id: string, standing: string, extra: Partial<EntrySlaGoal> = {}): EntrySlaGoal =>
  ({
    goal: 'RESOLUTION', kind: 'SLA', policy_key: id, assignee_group: null, standing, target_seconds: 3600,
    armed_at: '2026-11-12T10:00:00Z', due_at: '2026-11-12T14:00:00Z', at_risk_at: null, met_at: null, breached_at: null,
    ...extra,
  }) as EntrySlaGoal;

test('breached first, then at risk, then by the share used, kept ones last', () => {
  const sorted = byUrgency(
    [
      goal('met', 'MET', { met_at: '2026-11-12T11:00:00Z' }),
      goal('late', 'RUNNING', { armed_at: '2026-11-12T08:00:00Z' }),
      goal('risk', 'RUNNING', { at_risk_at: '2026-11-12T11:30:00Z' }),
      goal('early', 'RUNNING'),
      goal('broken', 'BREACHED', { breached_at: '2026-11-12T09:00:00Z' }),
    ],
    now,
  );
  assert.deepEqual(sorted.map((each) => each.policy_key), ['broken', 'risk', 'late', 'early', 'met']);
});

test('the share of time used', () => {
  assert.equal(shareOf(goal('a', 'RUNNING'), now), 50);
  assert.equal(shareOf(goal('a', 'BREACHED'), now), 100);
  assert.equal(shareOf(goal('a', 'MET', { met_at: '2026-11-12T11:00:00Z' }), now), 25);
});

test('the chip names what is breached, else the next running one, else nothing', () => {
  assert.equal(chipOf([goal('a', 'RUNNING'), goal('b', 'BREACHED')], now)?.goal.policy_key, 'b');
  assert.equal(chipOf([goal('a', 'RUNNING')], now)?.isBreached, false);
  assert.equal(chipOf([goal('a', 'MET'), goal('b', 'PAUSED')], now), undefined);
});

test('a row says what is left or how long it is overdue, never the moment alone', () => {
  assert.deepEqual(timeOf(goal('a', 'RUNNING'), now), { isOverdue: false, span: 2 * 3_600_000 });
  assert.deepEqual(timeOf(goal('a', 'BREACHED', { due_at: '2026-11-12T11:40:00Z', breached_at: '2026-11-12T11:41:00Z' }), now), { isOverdue: true, span: 20 * 60_000 });
  // Past due while the sweep has not marked it yet: overdue, and breached in tone.
  const late = goal('a', 'RUNNING', { due_at: '2026-11-12T11:50:00Z' });
  assert.deepEqual(timeOf(late, now), { isOverdue: true, span: 10 * 60_000 });
  assert.equal(urgencyOf(late, now), 'breached');
  assert.equal(timeOf(goal('a', 'PAUSED'), now), undefined);
  assert.equal(timeOf(goal('a', 'MET'), now), undefined);
});
