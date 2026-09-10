// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The query's path, which is the one thing about the audit screen that can be wrong silently: a
// filter that is dropped answers a wider trail than somebody asked for, and a trail that is wider
// than asked for reads as an answer rather than as a mistake.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { auditPath } from './audit.ts';

test('an unfiltered query is the bare path', () => {
  assert.equal(auditPath(), '/audit');
  assert.equal(auditPath({}), '/audit');
});

test('every filter the contract declares reaches the query string', () => {
  const path = auditPath({
    from: '2026-01-01T00:00:00Z',
    to: '2026-02-01T00:00:00Z',
    action: 'membership.',
    actorId: 'a1',
    targetType: 'container',
    targetId: 't1',
    outcome: 'DENIED',
  });
  for (const expected of [
    'from=2026-01-01T00%3A00%3A00Z',
    'to=2026-02-01T00%3A00%3A00Z',
    'action=membership.',
    'actor_id=a1',
    'target_type=container',
    'target_id=t1',
    'outcome=DENIED',
  ]) {
    assert.ok(path.includes(expected), `${expected} is missing from ${path}`);
  }
});

test('an empty filter is not sent as an empty filter', () => {
  // `action=` is not "no action filter" to every server, and sending one would be this client
  // asking a question it did not mean.
  assert.equal(auditPath({ action: '', outcome: '' }), '/audit');
});

test('the cursor is a parameter like the rest, and the same query keeps its key', () => {
  const query = { outcome: 'SUCCESS' as const };
  assert.equal(auditPath(query), '/audit?outcome=SUCCESS');
  assert.ok(auditPath(query, 'abc').includes('cursor=abc'));
  // The held page is keyed by the uncursored path, so a second page lands beside the first.
  assert.notEqual(auditPath(query), auditPath(query, 'abc'));
});
