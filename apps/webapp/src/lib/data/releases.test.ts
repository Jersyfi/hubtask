// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Release } from '@hubtask/sync-engine';

import { releaseAxis, releaseCandidates, releaseGroups, releaseShown, releaseTargets } from './releases.ts';

const release = (id: string, state: Release['state'], release_on: string): Release => ({
  id, hub_id: 'h', name: id, description: '', release_on, state, released_at: null, notes_page_id: null, version: 1,
});

// UC-PLN-08 checks 3 and 7: open work moves into another planned release, the soonest first.
test('a release carries its open work into another planned one', () => {
  const list = [release('out', 'RELEASED', '2026-10-01'), release('late', 'PLANNED', '2026-12-31'), release('next', 'PLANNED', '2026-11-30')];
  assert.deepEqual(releaseTargets(list, 'next').map((r) => r.id), ['late']);
  assert.deepEqual(releaseTargets(list, '').map((r) => r.id), ['next', 'late']);
  assert.equal(releaseShown(list)?.id, 'next');
  assert.equal(releaseShown([release('a', 'RELEASED', '2026-09-01'), release('b', 'RELEASED', '2026-10-01')])?.id, 'b');
  assert.equal(releaseShown([]), undefined);
});

// UC-PLN-08 check 7: an entry already in the release is not offered again.
test('the entries offered are those not in the release', () => {
  assert.deepEqual(releaseCandidates([{ id: 'a' }, { id: 'b' }], ['a']).map((e) => e.id), ['b']);
});

// UC-PLN-08 check 7: the list says what is coming first, soonest first, a planned release past its
// day marked late, and what went out after it, the latest first.
test('the releases are grouped coming, then gone out, a past planned one marked late', () => {
  const list = [
    release('old', 'RELEASED', '2026-09-01'), release('late', 'PLANNED', '2026-10-30'), release('next', 'PLANNED', '2026-11-30'),
    release('recent', 'RELEASED', '2026-10-01'), release('today', 'PLANNED', '2026-11-04'),
  ];
  const groups = releaseGroups(list, '2026-11-04');
  assert.deepEqual(groups.planned.map((r) => r.id), ['late', 'today', 'next']);
  assert.deepEqual(groups.released.map((r) => r.id), ['recent', 'old']);
  assert.deepEqual([...groups.late], ['late']);
  assert.deepEqual(releaseGroups([], '2026-11-04'), { planned: [], released: [], late: new Set() });
});

test('the releases stand on an axis of months, today marked, a crowded one on the next lane (scene planrel 2)', () => {
  const r = (id: string, release_on: string, state: 'PLANNED' | 'RELEASED' = 'PLANNED', released_at: string | null = null) =>
    ({ id, hub_id: 'h', name: id, description: '', release_on, state, released_at, notes_page_id: null, version: 1 }) as Release;
  const axis = releaseAxis([r('a', '2026-11-25'), r('b', '2026-10-20', 'RELEASED', '2026-10-28T10:00:00Z'), r('c', '2026-11-27')], '2026-11-12');
  assert.deepEqual(axis.months.map((each) => each.day), ['2026-10-01', '2026-11-01']);
  assert.equal(axis.months[0]?.at, 0);
  assert.ok(axis.today > 0.6 && axis.today < 0.7, `today at ${axis.today}`);
  const lane = Object.fromEntries(axis.marks.map((each) => [each.id, each.lane]));
  assert.deepEqual(lane, { b: 0, a: 0, c: 1 }, 'two releases two days apart share no lane');
  assert.ok((axis.marks.find((each) => each.id === 'b')?.at ?? 0) > 0.85 * (27 / 61), 'a release gone out stands at the day it went out');
});
