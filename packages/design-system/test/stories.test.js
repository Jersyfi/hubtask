// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The workbench's gate, run where the `Workspace` job in ci.yml already runs the package's tests.
//
// Selftest first, per the habit `gate-selftest` established on the Go side: a checker that cannot
// fail proves nothing by passing, and this one reads markdown prose - exactly the kind of reading
// that stops working quietly.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { checkTree, readInventory, readAxisIds, readStatuses, selftest } from '../build/check-stories.js';

test('the checker catches every violation it claims to catch', () => {
  assert.equal(selftest(), true);
});

test('every component has a story, and every story is well formed', () => {
  const { problems } = checkTree();
  assert.deepEqual(problems, []);
});

test('design-system.md §4 still reads as an inventory', () => {
  const waves = readInventory();
  // Present, not exhaustive: §4 may gain a wave, and a test that forbids that would be a test
  // about the specification's shape rather than about the reading working.
  for (const wave of ['Wave 1', 'Wave 2', 'Wave 3', 'Wave 4']) {
    assert.ok(waves.has(wave), `${wave} is no longer readable from design-system.md §4`);
  }
  // Wave 1 is the one F1 builds, and §4 names its eighteen by hand. If this number moves, a
  // component was added or removed in the specification and the waves need re-reading.
  assert.equal(waves.get('Wave 1').length, 18, 'wave 1 is no longer the eighteen §4 lists');
});

test('the axis ids and the statuses are read from the files that declare them', () => {
  // Not an assertion about the values - those are the source. An assertion that there is exactly
  // one place each list lives, which is what these two readers exist to guarantee.
  assert.ok(readAxisIds().includes('theme'));
  assert.ok(readStatuses().includes('draft'));
});
