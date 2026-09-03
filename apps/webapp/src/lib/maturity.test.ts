// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MATURITY, shouldAnnounce } from './maturity.ts';

test('the application says so while it is not stable, and stops when it is', () => {
  // ADR-0035 §2: the banner is what "can I rely on this yet?" is answered with until the
  // capability matrix is met. The rule is checked at every stage rather than at the one compiled
  // in, so that convergence removes the banner by changing one line and nothing else.
  assert.equal(shouldAnnounce('experimental'), true);
  assert.equal(shouldAnnounce('preview'), true);
  assert.equal(shouldAnnounce('stable'), false);
});

test('the stage this client was built at is the one F1 is in', () => {
  assert.equal(MATURITY, 'experimental');
  assert.equal(shouldAnnounce(), true);
});
