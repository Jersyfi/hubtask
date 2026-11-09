// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { DiscardModel } from './discard.ts';

test('a dialog without changes closes on dismiss and asks nothing', () => {
  const model = new DiscardModel(() => false);
  assert.equal(model.dismiss(), true);
  assert.equal(model.isAsking, false);
});

test('a dialog with changes stays open and asks whether to discard them', () => {
  const model = new DiscardModel(() => true);
  assert.equal(model.dismiss(), false);
  assert.equal(model.isAsking, true);
  model.keep();
  assert.equal(model.isAsking, false);
});

test('discarding closes the confirmation and then the dialog, once', () => {
  const model = new DiscardModel(() => true);
  model.dismiss();
  let closed = 0;
  model.discard(() => (closed += 1));
  assert.equal(closed, 1);
  assert.equal(model.isAsking, false);
});
