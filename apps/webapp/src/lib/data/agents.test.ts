// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  aiFields, expectedFrom, jobTarget, lifecyclePresses, mayDecide, mayStartJob, meteringPath, meteringTotals, previewOf, requestsPath, retentionOf, scopesFrom, selfEvaluation,
} from './agents.ts';

const admin = { administers: true, accountId: 'root' };
const ana = { administers: false, accountId: 'ana' };
const ben = { administers: false, accountId: 'ben' };
const agent = (status: 'DRAFT' | 'APPROVED' | 'ACTIVE' | 'SUSPENDED' | 'RETIRED', needs = false, owner = 'ana') => ({
  status, needs_approval: needs, owner_id: owner,
});

test('UC-AGT-01 check 9: an administrator approves, puts live, stops and retires an agent as its state allows', () => {
  assert.deepEqual(lifecyclePresses(agent('DRAFT', true), admin), ['approve', 'retire']);
  assert.deepEqual(lifecyclePresses(agent('APPROVED'), admin), ['activate', 'suspend', 'retire']);
  assert.deepEqual(lifecyclePresses(agent('ACTIVE'), admin), ['suspend', 'retire']);
  assert.deepEqual(lifecyclePresses(agent('SUSPENDED'), admin), ['activate', 'retire']);
  assert.deepEqual(lifecyclePresses(agent('RETIRED'), admin), []);
  // A model that moved since the approval asks for it again before the agent goes live.
  assert.deepEqual(lifecyclePresses(agent('SUSPENDED', true), admin), ['approve', 'retire']);
});

test('UC-AGT-01 check 9: the person an agent acts for stops it and never approves it', () => {
  assert.deepEqual(lifecyclePresses(agent('ACTIVE'), ana), ['suspend']);
  assert.deepEqual(lifecyclePresses(agent('DRAFT', true), ana), []);
  assert.deepEqual(lifecyclePresses(agent('ACTIVE'), ben), []);
  // An administrator whom the agent acts for is its owner first: four eyes.
  assert.deepEqual(lifecyclePresses(agent('DRAFT', true, 'root'), admin), ['retire']);
});

test('UC-AGT-07 check 7: Run on… is offered on a live shipped agent to its owner or an administrator', () => {
  assert.equal(jobTarget('TICKET_TRIAGE'), 'ENTRY');
  assert.equal(jobTarget('KNOWLEDGE_DRAFT'), 'ENTRY');
  assert.equal(jobTarget('DUPLICATE_REVIEW'), 'HUB');
  assert.equal(jobTarget('EXTERNAL'), undefined);
  const live = { status: 'ACTIVE' as const, kind: 'TICKET_TRIAGE' as const, owner_id: 'ana' };
  assert.equal(mayStartJob(live, admin), true);
  assert.equal(mayStartJob(live, ana), true);
  assert.equal(mayStartJob(live, ben), false);
  assert.equal(mayStartJob({ ...live, status: 'APPROVED' }, admin), false);
  assert.equal(mayStartJob({ ...live, kind: 'EXTERNAL' }, admin), false);
});

test('UC-AGT-02 check 9: a held act is previewed field by field before a person decides it', () => {
  const lines = previewOf({ item_id: 'e1', patch: { title: 'Printer down', labels: ['hardware'] }, empty: {}, note: null });
  assert.deepEqual(lines, [
    { key: 'item_id', value: 'e1' },
    { key: 'patch.title', value: 'Printer down' },
    { key: 'patch.labels', value: '["hardware"]' },
    { key: 'empty', value: '{}' },
    { key: 'note', value: '—' },
  ]);
  const long = previewOf({ notes: 'x'.repeat(400) })[0]?.value ?? '';
  assert.ok(long.length <= 161 && long.endsWith('…'));
  assert.equal(mayDecide({ owner_id: 'ana' }, ana), true);
  assert.equal(mayDecide({ owner_id: 'ana' }, ben), false);
  assert.equal(mayDecide(undefined, admin), true);
  assert.equal(requestsPath(), '/agent-requests?status=PENDING');
  assert.equal(requestsPath('FAILED', 'a1'), '/agent-requests?status=FAILED&agent_id=a1');
});

test('UC-AGT-02 check 9: the metering is summed per agent, the most tokens first', () => {
  const totals = meteringTotals([
    { agent_id: 'a', day: '2026-10-28', allowed: 2, held: 1, refused: 0, tokens: 100 },
    { agent_id: 'b', day: '2026-10-28', allowed: 1, held: 0, refused: 3, tokens: 900 },
    { agent_id: 'a', day: '2026-10-29', allowed: 4, held: 0, refused: 1, tokens: 50 },
  ]);
  assert.deepEqual(totals, [
    { agentId: 'b', allowed: 1, held: 0, refused: 3, tokens: 900, days: 1 },
    { agentId: 'a', allowed: 6, held: 1, refused: 1, tokens: 150, days: 2 },
  ]);
  assert.equal(meteringPath(), '/agents:metering');
  assert.equal(meteringPath('a', 7), '/agents:metering?agent_id=a&days=7');
});

test('UC-AGT-04 check 6: a case is added with an expected answer that is an object', () => {
  assert.deepEqual(expectedFrom('{"labels":["hardware"]}'), { value: { labels: ['hardware'] } });
  assert.deepEqual(expectedFrom('[1]'), { problem: 'not_object' });
  assert.deepEqual(expectedFrom('null'), { problem: 'not_object' });
  assert.deepEqual(expectedFrom('{labels'), { problem: 'not_json' });
  assert.deepEqual(scopesFrom('items:read, items:write  items:read'), ['items:read', 'items:write']);
});

test('UC-AGT-05 check 5: the entry names each value that came from AI once, never the value', () => {
  assert.deepEqual(
    aiFields([
      { fields: ['labels', 'title'], accepted_at: '2026-10-29T10:00:00Z' },
      { fields: ['notes'], accepted_at: '2026-10-28T09:00:00Z' },
      { fields: ['title'], accepted_at: '2026-10-30T08:00:00Z' },
    ]),
    ['notes', 'labels', 'title'],
  );
  assert.deepEqual(aiFields([]), []);
});

test('UC-AGT-03 check 7: a model says how long its provider keeps what it is sent, and an unknown retention as unknown', () => {
  assert.deepEqual(retentionOf(null), { code: 'app.agents.retention_unknown', params: {} });
  assert.deepEqual(retentionOf(0), { code: 'app.agents.retention_none', params: {} });
  assert.deepEqual(retentionOf(30), { code: 'app.agents.retention_days', params: { days: '30' } });
});

test('UC-AGT-04 check 7: a shipped agent answers its own set, a CI suggestion and a triage against a hub', () => {
  assert.equal(selfEvaluation('KNOWLEDGE_DRAFT'), 'ready');
  assert.equal(selfEvaluation('CI_SUGGESTION'), 'hub');
  assert.equal(selfEvaluation('TICKET_TRIAGE'), 'hub');
  assert.equal(selfEvaluation('DUPLICATE_REVIEW'), undefined);
  assert.equal(selfEvaluation('EXTERNAL'), undefined);
});
