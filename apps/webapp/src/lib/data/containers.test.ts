// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Container } from '@hubtask/sync-engine';

import { archivalOf, siblingBefore } from './containers.ts';

const hub = (id: string, name: string, extra: Partial<Container> = {}): Container =>
  ({ id, type: 'HUB', name, order_key: 'a0', version: 1, ...extra }) as Container;

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

// --- ranking ----------------------------------------------------------------------------------

const siblings = [hub('a', 'A'), hub('b', 'B'), hub('c', 'C')];

test('moving up lands before the sibling at that position', () => {
  assert.equal(siblingBefore(siblings, 'c', 0), 'a');
  assert.equal(siblingBefore(siblings, 'c', 1), 'b');
});

test('moving down past its own position lands before the one after it', () => {
  // The case that is easy to get wrong. `a` moving to position 1 must land before `c`, not before
  // `b` — because with `a` taken out of the list, position 1 *is* `c`.
  assert.equal(siblingBefore(siblings, 'a', 1), 'c');
});

test('the end of the list appends rather than naming a sibling', () => {
  // Null is the API's own word for "the end", so there is nothing to invent here.
  assert.equal(siblingBefore(siblings, 'a', 2), null);
  assert.equal(siblingBefore(siblings, 'a', 99), null);
});
