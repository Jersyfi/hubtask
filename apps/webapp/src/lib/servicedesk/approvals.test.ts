// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { decidedOnOlderVersion, delegationProblem, levelStanding, stateTone, trailOf } from './approvals.ts';

const decision = (level: number, verdict: 'APPROVE' | 'DECLINE', at: string, version = 3) => ({
  level, approver_id: `p${level}${at}`, on_behalf_of: null, verdict, reason: '', item_version: version, decided_at: at,
});

test('a level counts its yeses against what passes it (UC-SVC-09 check 2)', () => {
  const approval = {
    levels: [{ rule: 'N_OF' as const, count: 2, approvers: ['a', 'b', 'c'], roles: [], needed: 2 }],
    decisions: [decision(1, 'APPROVE', '2026-10-30T10:00:00Z'), decision(1, 'DECLINE', '2026-10-30T09:00:00Z')],
  };
  assert.deepEqual(levelStanding(approval, 1), { yes: 1, needed: 2 });
  assert.deepEqual(levelStanding(approval, 2), { yes: 0, needed: 0 });
});

test('the trail reads oldest first and marks a decision on another version (UC-SVC-09 check 4)', () => {
  const approval = {
    item_version: 3,
    decisions: [decision(2, 'APPROVE', '2026-10-30T11:00:00Z', 4), decision(1, 'APPROVE', '2026-10-30T10:00:00Z')],
  };
  const trail = trailOf(approval);
  assert.deepEqual(trail.map((each) => each.level), [1, 2]);
  assert.equal(decidedOnOlderVersion(approval, trail[1]!), true);
  assert.equal(decidedOnOlderVersion(approval, trail[0]!), false);
});

test('a badge says where an approval stands', () => {
  assert.equal(stateTone('PENDING'), 'info');
  assert.equal(stateTone('APPROVED'), 'success');
  assert.equal(stateTone('DECLINED'), 'danger');
  assert.equal(stateTone('WITHDRAWN'), 'neutral');
});

test('the delegation form refuses what the server would (UC-SVC-10 check 1)', () => {
  const from = '2026-11-01T00:00:00Z';
  assert.equal(delegationProblem('', 'me', from, '2026-11-08T00:00:00Z'), 'app.approvals.delegate_first');
  assert.equal(delegationProblem('me', 'me', from, '2026-11-08T00:00:00Z'), 'app.approvals.delegate_not_self');
  assert.equal(delegationProblem('you', 'me', from, from), 'app.approvals.delegate_dates');
  assert.equal(delegationProblem('you', 'me', from, '2028-01-01T00:00:00Z'), 'app.approvals.delegate_year');
  assert.equal(delegationProblem('you', 'me', from, '2026-11-08T00:00:00Z'), undefined);
});
