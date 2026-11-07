// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { TimePolicy } from '@hubtask/sync-engine';

import { lockedAbove, partOf } from './policy.ts';

test('a part the plan or the installation locked is locked above the workspace', () => {
  const policy: Pick<TimePolicy, 'settings'> = { settings: {
    'time.visibility': { value: 'AGGREGATED', source: 'PLAN', locked_by: 'PLAN' },
    'time.locked_through': { value: '2026-09-30', source: 'WORKSPACE', locked_by: null },
  } };
  assert.equal(lockedAbove(policy, 'time.visibility'), 'PLAN');
  assert.equal(lockedAbove(policy, 'time.locked_through'), undefined);
  assert.equal(partOf(policy, 'time.locked_through').source, 'WORKSPACE');
});

test('a policy from a server or a copy that names no sources locks nothing', () => {
  assert.equal(lockedAbove({}, 'time.visibility'), undefined);
  assert.equal(lockedAbove(undefined, 'time.locked_through'), undefined);
  assert.deepEqual(partOf({}, 'time.visibility'), {});
});
