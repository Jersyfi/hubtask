// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { CalendarPush } from '@hubtask/sync-engine';

import { calendarPushPath, providerAnswerOf, pushesOf } from './calendarpush.ts';

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
