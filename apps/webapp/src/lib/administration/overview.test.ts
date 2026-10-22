// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the administration's overview says needs attention (UC-ID-24 check 2).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { backupState, limitLines, newestArchive, owedSoon, pageOfFeature, ruleLines } from './overview.ts';

test('limits: fullest first, reached and near told apart, no line without a ceiling or a count', () => {
  const lines = limitLines([
    { quota: 'items', limit: 1000, used: 120, ratio: 0.12 },
    { quota: 'accounts', limit: 30, used: 28, ratio: 28 / 30 },
    { quota: 'webhook_targets', limit: 5, used: 5, ratio: 1 },
    { quota: 'api_requests_per_minute', limit: 600, used: null, ratio: null },
    { quota: 'media_bytes', limit: 0, used: 10 },
  ]);
  assert.deepEqual(lines.map((line) => [line.quota, line.state]), [
    ['webhook_targets', 'reached'],
    ['accounts', 'near'],
    ['items', 'room'],
  ]);
  assert.deepEqual(lines[1], { quota: 'accounts', limit: 30, used: 28, state: 'near' });
});

test('limits: a ratio the server leaves out is computed from use and ceiling', () => {
  assert.equal(limitLines([{ quota: 'items', limit: 10, used: 9 }])[0]?.state, 'near');
});

test('rules: failing first by failures, then broken, then attention; a quiet rule is no line', () => {
  const lines = ruleLines([
    { id: 'a', name: 'Quiet', enabled: true, failure_count: 0, findings: [] },
    { id: 'b', name: 'Look at me', enabled: true, failure_count: 0, findings: [{ level: 'ATTENTION' }] },
    { id: 'c', name: 'Fails once', enabled: true, failure_count: 1 },
    { id: 'd', name: 'Fails thrice', enabled: true, failure_count: 3 },
    { id: 'e', name: 'Broken', enabled: true, failure_count: 0, findings: [{ level: 'ATTENTION' }, { level: 'BROKEN' }] },
    { id: 'f', name: 'Off, worth a look', enabled: false, failure_count: 0, findings: [{ level: 'ATTENTION' }] },
  ]);
  assert.deepEqual(lines.map((line) => [line.id, line.state]), [
    ['d', 'failing'], ['c', 'failing'], ['e', 'broken'], ['b', 'attention'],
  ]);
});

test('backup: the newest archive across targets, and whether it opened', () => {
  const newest = newestArchive({
    t1: [{ archive_id: 'old', created_at: '2026-10-01T00:00:00Z' }],
    t2: [{ archive_id: 'new', created_at: '2026-10-20T00:00:00Z' }, { archive_id: 'mid', created_at: '2026-10-10T00:00:00Z' }],
  });
  assert.deepEqual(newest, { targetId: 't2', archive: { archive_id: 'new', created_at: '2026-10-20T00:00:00Z' } });
  assert.equal(newestArchive({}), undefined);

  const archive = { archive_id: 'x', created_at: '2026-10-20T00:00:00Z' };
  assert.equal(backupState(archive, { verified_at: '2026-10-21T00:00:00Z', verify_ok: true }), 'opened');
  assert.equal(backupState(archive, { verified_at: '2026-10-21T00:00:00Z', verify_ok: false }), 'broken');
  assert.equal(backupState(archive, { verified_at: null }), 'unchecked');
  assert.equal(backupState(archive, undefined), 'unchecked');
  assert.equal(backupState({ ...archive, complete: false }, { verified_at: '2026-10-21T00:00:00Z', verify_ok: true }), 'incomplete');
});

test('requests: overdue and soon only, soonest first', () => {
  const standing = { a: 'ahead', b: 'soon', c: 'overdue', d: undefined } as const;
  const owed = owedSoon(
    [
      { id: 'a', status: 'RECEIVED', due_at: '2026-12-01T00:00:00Z' },
      { id: 'b', status: 'RECEIVED', due_at: '2026-10-25T00:00:00Z' },
      { id: 'c', status: 'IN_PROGRESS', due_at: '2026-10-20T00:00:00Z' },
      { id: 'd', status: 'COMPLETED', due_at: '2026-10-01T00:00:00Z' },
    ],
    (request) => standing[request.id as keyof typeof standing],
  );
  assert.deepEqual(owed.map((request) => request.id), ['c', 'b']);
});

test('a degraded feature links to its page only where the workspace sets it', () => {
  assert.equal(pageOfFeature('ai'), '/administration/ai');
  assert.equal(pageOfFeature('media'), undefined);
});
