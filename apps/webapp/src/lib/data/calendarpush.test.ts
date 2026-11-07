// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { CalendarPush } from '@hubtask/sync-engine';

import { calendarPushPath, providerAnswerOf, pushColumnFigure, pushFigure, pushesOf } from './calendarpush.ts';

const push = (id: string, feed: string, provider: CalendarPush['provider']): CalendarPush => ({
  id, feed_id: feed, provider, address: 'ada@example.org', state: 'ACTIVE', created_at: '2026-10-30T10:00:00Z',
});

test('UC-INT-16: what the provider sent back is read, and a page opened without a state is no answer', () => {
  assert.deepEqual(providerAnswerOf({ state: 's', code: 'c' }), { state: 's', code: 'c' });
  assert.deepEqual(providerAnswerOf({ state: 's', error: 'access_denied' }), { state: 's', error: 'access_denied' });
  assert.equal(providerAnswerOf({ code: 'c' }), undefined);
});

test("UC-INT-16: a feed's pushes are answered by provider", () => {
  const all = [push('p1', 'f1', 'GOOGLE'), push('p2', 'f2', 'MICROSOFT'), push('p3', 'f1', 'MICROSOFT')];
  const mine = pushesOf(all, 'f1');
  assert.equal(mine.GOOGLE?.id, 'p1');
  assert.equal(mine.MICROSOFT?.id, 'p3');
  assert.deepEqual(pushesOf(all, 'f9'), {});
  assert.equal(calendarPushPath('p1'), '/integrations/calendar-pushes/p1');
});

test('the head says how many write and how many stopped, and the column counts only the stopped', () => {
  const stopped = { ...push('p2', 'f1', 'MICROSOFT'), state: 'FAILED' as const };
  assert.deepEqual(pushFigure([push('p1', 'f1', 'GOOGLE'), stopped]), { active: 1, failed: 1 });
  assert.deepEqual(pushFigure([]), { active: 0, failed: 0 });
  const t = (code: string, params: Record<string, string> = {}) => [code, ...Object.values(params)].join(' ');
  assert.equal(pushColumnFigure([push('p1', 'f1', 'GOOGLE'), stopped], t), 'app.pushes.column_failed 1');
  assert.equal(pushColumnFigure([push('p1', 'f1', 'GOOGLE')], t), undefined, 'nothing while every one writes');
});
