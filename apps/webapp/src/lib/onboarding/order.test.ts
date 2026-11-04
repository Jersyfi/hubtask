// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { tourMayStart } from './order.ts';

test('the tour waits for the starter and never opens beside it (user-experience.md §3.1)', () => {
  assert.equal(tourMayStart({ starterOffered: true, starterOpen: false }), false);
  assert.equal(tourMayStart({ starterOffered: false, starterOpen: true }), false);
  assert.equal(tourMayStart({ starterOffered: false, starterOpen: false }), true);
});
