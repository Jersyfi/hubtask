// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ROUTES } from '../routes.ts';
import { isFrameEmergencyNoticeShown } from './emergencyplacement.ts';

test('the frame draws the emergency notice on the start view', () => {
  assert.equal(isFrameEmergencyNoticeShown('home'), true);
});

test('the frame draws no emergency notice in a work view, the administration or the hub', () => {
  // The hub is not here because it shows its own notice (`ContainerView`).
  for (const name of ['whiteboard', 'item', 'page', 'collection', 'administration', 'hub', null]) {
    assert.equal(isFrameEmergencyNoticeShown(name), false, String(name));
  }
});

test('the names it is tested with are routes of the table', () => {
  const names = ROUTES.map((each) => each.name);
  for (const name of ['home', 'whiteboard', 'item', 'page', 'collection', 'administration', 'hub']) {
    assert.ok(names.includes(name), name);
  }
});
