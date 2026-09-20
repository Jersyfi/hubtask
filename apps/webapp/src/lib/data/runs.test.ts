// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { runsPath } from './runs.ts';

test('the listing path carries every filter the contract takes, and nothing for an empty one', () => {
  assert.equal(runsPath(), '/automation/runs');
  assert.equal(
    runsPath({ ruleId: 'r1', status: 'FAILED', trigger: 'MANUAL', from: '2026-09-13T00:00:00Z', to: '2026-09-20T00:00:00Z' }, 'c2'),
    '/automation/runs?rule_id=r1&status=FAILED&trigger=MANUAL&from=2026-09-13T00%3A00%3A00Z&to=2026-09-20T00%3A00%3A00Z&cursor=c2',
  );
});

test('the window travels alone when it is the only filter', () => {
  assert.equal(runsPath({ from: '2026-09-13T00:00:00Z' }), '/automation/runs?from=2026-09-13T00%3A00%3A00Z');
});
