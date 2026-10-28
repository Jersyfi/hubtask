// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { ModuleState } from '@hubtask/sync-engine';

import { asksFirst, isSetHere, moduleCodes, unavailableBecause } from './modules.ts';

const state = (over: Partial<ModuleState>): ModuleState => ({ module: 'PLANNING', on: false, source: 'DEFAULT', ...over });

test('switching a module off asks first, switching it on does not (UC-INS-21 check 4)', () => {
  assert.equal(asksFirst(state({ on: true }), false), true);
  assert.equal(asksFirst(state({ on: false }), true), false);
  assert.equal(asksFirst(state({ on: true }), true), false);
});

test('a lock above offers no switch and says whose it is (UC-INS-21 check 6)', () => {
  assert.equal(unavailableBecause(state({ on: true, source: 'PLAN', locked_by: 'PLAN' }), 'WORKSPACE'), 'app.modules.locked_plan');
  assert.equal(unavailableBecause(state({ source: 'INSTANCE', locked_by: 'INSTANCE' }), 'WORKSPACE'), 'app.modules.locked_installation');
  assert.equal(unavailableBecause(state({ on: true, source: 'WORKSPACE' }), 'WORKSPACE'), undefined);
});

test('a hub never widens what the workspace has off (UC-INS-21 check 3)', () => {
  assert.equal(unavailableBecause(state({ on: false, source: 'WORKSPACE' }), 'HUB'), 'app.modules.off_in_workspace');
  assert.equal(unavailableBecause(state({ on: true, source: 'WORKSPACE' }), 'HUB'), undefined);
  assert.equal(unavailableBecause(state({ on: false, source: 'HUB' }), 'HUB'), undefined);
});

test('only a value set here is handed back to the level above', () => {
  assert.equal(isSetHere(state({ on: true, source: 'WORKSPACE' }), 'WORKSPACE'), true);
  assert.equal(isSetHere(state({ on: true, source: 'PLAN' }), 'WORKSPACE'), false);
  assert.equal(isSetHere(state({ on: false, source: 'HUB' }), 'HUB'), true);
  assert.deepEqual(moduleCodes(state({ module: 'SERVICE_DESK' })), {
    name: 'app.modules.name_service_desk', hint: 'app.modules.hint_service_desk',
  });
});
