// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The query's path, which is the one thing about the audit screen that can be wrong silently: a
// filter that is dropped answers a wider trail than somebody asked for, and a trail that is wider
// than asked for reads as an answer rather than as a mistake.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { auditPath, readVerification } from './audit.ts';

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

test('a chain that holds and a chain with a break are two different findings', () => {
  // The property the screen turns on. A break rendered as a failed request would hide the finding
  // the whole trail exists to produce.
  const holds = readVerification({ valid: true, checked: 34 });
  assert.equal(holds.kind, 'holds');
  assert.equal(holds.checked, 34);

  const broken = readVerification({
    valid: false,
    checked: 34,
    first_broken_seq: 17,
    gaps: [18, 19, 20],
    gap_count: 3,
  });
  assert.equal(broken.kind, 'broken');
  if (broken.kind !== 'broken') return;
  assert.equal(broken.firstBrokenSeq, 17, 'the break names where an investigation starts');
  assert.equal(broken.gapCount, 3);
  assert.deepEqual([...broken.gaps], [18, 19, 20]);
});

test('anything but an explicit yes is a break', () => {
  // Fail-closed: an answer this client did not understand must not become "the chain holds".
  for (const answer of [{}, { valid: undefined }, { checked: 5 }]) {
    assert.equal(readVerification(answer).kind, 'broken', JSON.stringify(answer));
  }
});

test('a break that named no sequence still counts what is missing', () => {
  // The contract cuts the listed gaps at a hundred, so the count is the number that matters and
  // the list is the sample. A break with neither is still a break.
  const broken = readVerification({ valid: false, gap_count: 1_000_000 });
  assert.equal(broken.kind, 'broken');
  if (broken.kind !== 'broken') return;
  assert.equal(broken.firstBrokenSeq, undefined);
  assert.equal(broken.gapCount, 1_000_000);
  assert.deepEqual([...broken.gaps], []);
});

test('the anchor is reported only when there is one', () => {
  // "Never anchored outside this database" is a different sentence from a date, and it is the
  // honest one for every installation until external anchoring exists.
  assert.equal(readVerification({ valid: true, checked: 1 }).kind, 'holds');
  const anchored = readVerification({ valid: true, checked: 1, sealed_until: '2026-09-01T00:00:00Z' });
  assert.equal(anchored.kind === 'holds' ? anchored.sealedUntil : undefined, '2026-09-01T00:00:00Z');
  const never = readVerification({ valid: true, checked: 1, sealed_until: null });
  assert.equal(never.kind === 'holds' ? never.sealedUntil : 'set', undefined);
});
