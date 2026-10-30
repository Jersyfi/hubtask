// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { Release } from '@hubtask/sync-engine';

import { releaseCandidates, releaseShown, releaseTargets } from './releases.ts';

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
