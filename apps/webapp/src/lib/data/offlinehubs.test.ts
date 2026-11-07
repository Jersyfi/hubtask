// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MAX_REMEMBERED, forgetHubVisits, noteHubVisit, offlineHubs, type VisitStore } from './offlinehubs.ts';

const NOW = Date.UTC(2026, 10, 4);
const DAY = 24 * 60 * 60 * 1000;

function memory(): VisitStore {
  const held = new Map<string, string>();
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => void held.set(key, value),
    removeItem: (key) => void held.delete(key),
  };
}

test('the hubs opened in the last thirty days, most recent first, per account', () => {
  const store = memory();
  noteHubVisit('a-1', 'h-old', NOW - 31 * DAY, store);
  noteHubVisit('a-1', 'h-2', NOW - 3 * DAY, store);
  noteHubVisit('a-1', 'h-1', NOW - DAY, store);
  noteHubVisit('a-1', 'h-2', NOW - 2 * DAY, store);
  noteHubVisit('a-2', 'h-9', NOW, store);
  assert.deepEqual(offlineHubs('a-1', NOW, store), ['h-1', 'h-2']);
  assert.deepEqual(offlineHubs('a-2', NOW, store), ['h-9']);
  assert.deepEqual(offlineHubs(undefined, NOW, store), []);
});

test('at most the cap is remembered, and the session\'s end forgets them', () => {
  const store = memory();
  for (let i = 0; i < MAX_REMEMBERED + 5; i += 1) noteHubVisit('a-1', `h-${i}`, NOW - (MAX_REMEMBERED + 5 - i) * 1000, store);
  const held = offlineHubs('a-1', NOW, store);
  assert.equal(held.length, MAX_REMEMBERED);
  assert.equal(held[0], `h-${MAX_REMEMBERED + 4}`, 'the most recent first');
  forgetHubVisits('a-1', store);
  assert.deepEqual(offlineHubs('a-1', NOW, store), []);
});

test('a value another version wrote is read as nothing rather than thrown at', () => {
  const store = memory();
  store.setItem('hubtask.hub-visits.a-1', '[1,2]');
  assert.deepEqual(offlineHubs('a-1', NOW, store), []);
  store.setItem('hubtask.hub-visits.a-1', 'not json');
  assert.deepEqual(offlineHubs('a-1', NOW, store), []);
});
