// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { offlineHubs } from './offlinehubs.ts';

const NOW = Date.UTC(2026, 10, 4);
const DAY = 24 * 60 * 60 * 1000;

test('the hubs opened in the last thirty days, a collection counting for its hub, most recent first', () => {
  const hubs = offlineHubs([
    { kind: 'hub', id: 'h-old', title: 'Old', at: NOW - 31 * DAY },
    { kind: 'collection', id: 'c-1', title: 'Inbox', placeId: 'h-2', at: NOW - 2 * DAY },
    { kind: 'hub', id: 'h-1', title: 'Engines', at: NOW - DAY },
    { kind: 'item', id: 'i-1', title: 'Buy milk', placeId: 'c-9', at: NOW },
    { kind: 'hub', id: 'h-2', title: 'Rail', at: NOW - 3 * DAY },
    { kind: 'hub', id: 'h-undated', title: 'Kept before dates' },
  ], NOW);
  assert.deepEqual(hubs, ['h-1', 'h-2']);
});

test('nothing opened lately holds no hub', () => {
  assert.deepEqual(offlineHubs([], NOW), []);
});
