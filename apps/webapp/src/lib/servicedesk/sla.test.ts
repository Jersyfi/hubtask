// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { EntrySlaList } from '@hubtask/sync-engine';

import { firstToBreak, queueSla, spanOf } from './sla.ts';

type Goal = EntrySlaList['data'][number];
const now = new Date('2026-11-04T10:00:00Z');
const goal = (over: Partial<Goal>): Goal => ({
  goal: 'RESOLUTION', kind: 'SLA', policy_key: 'p', assignee_group: null, standing: 'RUNNING', target_seconds: 3600,
  armed_at: '2026-11-04T09:00:00Z', due_at: '2026-11-04T11:00:00Z', at_risk_at: null, met_at: null, breached_at: null, ...over,
});

// user-experience.md §7.4: the row shows what breaks first - a breach before time left, the
// nearest due first, the requester's promise before a team's.
test('the goal a row stands for is the one that breaks first, the requester\'s before a team\'s', () => {
  const later = goal({ goal: 'RESOLUTION', due_at: '2026-11-04T12:00:00Z' });
  const sooner = goal({ goal: 'FIRST_RESPONSE', due_at: '2026-11-04T10:30:00Z' });
  const breached = goal({ goal: 'NEXT_RESPONSE', standing: 'BREACHED', due_at: null, breached_at: '2026-11-04T09:30:00Z' });
  const ola = goal({ kind: 'OLA', standing: 'BREACHED', due_at: null, breached_at: '2026-11-04T08:00:00Z' });
  assert.equal(firstToBreak([later, sooner])?.goal, 'FIRST_RESPONSE');
  assert.equal(firstToBreak([later, sooner, breached])?.goal, 'NEXT_RESPONSE');
  assert.equal(firstToBreak([later, ola])?.kind, 'SLA');
  assert.equal(firstToBreak([ola])?.kind, 'OLA');
  assert.equal(firstToBreak([]), undefined);
  assert.equal(queueSla([], now), undefined);
});

// UX-7: the state is said in words and a mark; the bar is the share of the time used.
test('a running goal says the time left and how much is used, at risk once its warning passed', () => {
  const running = queueSla([goal({})], now);
  assert.equal(running?.state, 'running');
  assert.equal(running?.code, 'app.desk_queues.sla_left');
  assert.equal(running?.span, 3_600_000);
  assert.equal(running?.used, 50);
  assert.equal(queueSla([goal({ at_risk_at: '2026-11-04T09:50:00Z' })], now)?.state, 'at_risk');
});

test('a goal past due reads as breached even before the sweep marked it; paused and kept say so', () => {
  const overdue = queueSla([goal({ due_at: '2026-11-04T09:42:00Z' })], now);
  assert.equal(overdue?.state, 'breached');
  assert.equal(overdue?.span, 18 * 60_000);
  assert.equal(overdue?.used, 100);
  assert.equal(queueSla([goal({ standing: 'BREACHED', due_at: null, breached_at: '2026-11-04T09:00:00Z' })], now)?.span, 3_600_000);
  assert.equal(queueSla([goal({ standing: 'PAUSED', due_at: null })], now)?.state, 'paused');
  assert.equal(queueSla([goal({ standing: 'MET', due_at: null, met_at: '2026-11-04T09:10:00Z' })], now)?.state, 'met');
});

test('a span reads in minutes, hours and minutes, or days and hours', () => {
  assert.deepEqual(spanOf(42 * 60_000), { code: 'app.desk_queues.span_minutes', params: { minutes: '42' } });
  assert.deepEqual(spanOf(10_000), { code: 'app.desk_queues.span_minutes', params: { minutes: '1' } });
  assert.deepEqual(spanOf((5 * 60 + 10) * 60_000), { code: 'app.desk_queues.span_hours', params: { hours: '5', minutes: '10' } });
  assert.deepEqual(spanOf((50 * 60) * 60_000), { code: 'app.desk_queues.span_days', params: { days: '2', hours: '2' } });
});
