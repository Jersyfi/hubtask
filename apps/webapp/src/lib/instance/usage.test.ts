// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { latestOf, usageRange } from './usage.ts';

test('the range ends today and spans thirty days', () => {
  assert.deepEqual(usageRange(Date.parse('2026-11-04T10:00:00Z')), { from: '2026-10-06', to: '2026-11-04' });
});

test('a row shows each metric on its latest day, whatever order the days arrive in', () => {
  const latest = latestOf({
    workspace_id: '01a0e2e0-0000-7000-8000-0000000005a1', purged: false,
    metrics: {
      items: [{ day: '2026-11-04', value: 120 }, { day: '2026-11-03', value: 100 }],
      accounts: [{ day: '2026-11-04', value: 7 }],
      ai_tokens_per_day: [{ day: '2026-11-04', value: 9 }],
    },
  });
  assert.deepEqual(latest, { items: 120, accounts: 7 });
});
