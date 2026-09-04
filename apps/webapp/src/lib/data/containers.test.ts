// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Container } from '@hubtask/sync-engine';

import { archivalOf } from './containers.ts';

const collection = (id: string, name: string, parent: string, extra: Partial<Container> = {}): Container =>
  ({ id, type: 'COLLECTION', name, parent_id: parent, order_key: 'a0', version: 1, ...extra }) as Container;

// The tree-shaping tests that used to sit here went with `groupByHub`. The API reads one level at
// a time, so there is no flat list to group and nothing to assert about grouping one.

// --- the archive ------------------------------------------------------------------------------

test('archived and inside an archived hub are different answers', () => {
  // The schema separates the two precisely so a client can tell them apart, and they are different
  // offers: the first has an unarchive control, the second has one on the hub above and none here.
  assert.equal(archivalOf(collection('c', 'Shopping', 'h')), 'active');
  assert.equal(
    archivalOf(collection('c', 'Shopping', 'h', { archived_at: '2026-09-01T00:00:00Z', effective_archived: true })),
    'archived',
  );
  assert.equal(
    archivalOf(collection('c', 'Shopping', 'h', { archived_at: null, effective_archived: true })),
    'inherited',
  );
});

test('reading effective_archived alone would offer the wrong control', () => {
  // The regression this function exists to prevent: both of these are read-only, and only one of
  // them can be unarchived where the reader is standing.
  const own = collection('c', 'A', 'h', { archived_at: '2026-09-01T00:00:00Z', effective_archived: true });
  const inherited = collection('d', 'B', 'h', { archived_at: null, effective_archived: true });
  assert.notEqual(archivalOf(own), archivalOf(inherited));
});

// The ranking tests moved to `rank.test.ts` with `siblingBefore`, which is `anchorFor` now:
// entries rank the way containers do, and the assertion was always about the arithmetic.
