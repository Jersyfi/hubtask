// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { isRunning, recentAccesses } from './privatehubs.ts';

const access = (opened: string, ends: string) => ({
  id: opened, hub_id: 'h', opened_by: null, reason: 'r', opened_at: opened, ends_at: ends,
});

test('a hub says something about the accesses of the last thirty days and no older one', () => {
  const now = Date.parse('2026-10-12T12:00:00Z');
  const kept = recentAccesses([
    access('2026-10-12T11:00:00Z', '2026-10-12T12:00:00Z'),
    access('2026-09-12T12:00:00Z', '2026-09-12T13:00:00Z'),
    access('2026-09-12T11:59:59Z', '2026-09-12T12:59:59Z'),
  ], now);
  assert.deepEqual(kept.map((each) => each.opened_at), ['2026-10-12T11:00:00Z', '2026-09-12T12:00:00Z']);
});

test('an access runs until its end and not a moment after', () => {
  const opened = access('2026-10-12T11:00:00Z', '2026-10-12T12:00:00Z');
  assert.equal(isRunning(opened, Date.parse('2026-10-12T11:59:59Z')), true);
  assert.equal(isRunning(opened, Date.parse('2026-10-12T12:00:00Z')), false);
});
