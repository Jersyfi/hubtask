// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { switchedOn } from './switchedon.ts';

const MODULES = ['WORKFLOWS', 'PLANNING', 'KNOWLEDGE', 'WHITEBOARD', 'SERVICE_DESK', 'ASSETS', 'TIME', 'GOALS', 'AGENTS'];

test('every module switched on for the workspace says where to find it or what to set up', () => {
  for (const module of MODULES) {
    const next = switchedOn(module);
    assert.ok(next.find !== undefined || next.setup !== undefined, module);
  }
  assert.deepEqual(switchedOn('WHITEBOARD').find, { code: 'app.nav.whiteboards', href: '/whiteboards' });
  assert.equal(switchedOn('SERVICE_DESK').setup?.href, '/administration/service-desk');
});

test('switched on at a hub, it is found on the hub\'s own tab', () => {
  assert.deepEqual(switchedOn('KNOWLEDGE', 'h').find, { code: 'app.pages.title', href: '/hubs/h/pages' });
  // A module the hub's strip has no tab for is found at its home.
  assert.equal(switchedOn('SERVICE_DESK', 'h').find?.href, '/desk');
});
