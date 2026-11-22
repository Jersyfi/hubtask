// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { ModuleState } from '@hubtask/sync-engine';

import { asksFirst, isSetHere, modulesHere, moduleCodes, offReach, unavailableBecause } from './modules.ts';

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

test('a hub never switches on what the workspace has off, whatever it stores itself (UC-INS-21 check 3)', () => {
  assert.equal(unavailableBecause(state({ on: false, source: 'WORKSPACE' }), 'HUB', []), 'app.modules.off_in_workspace');
  // A workspace's switch leaves each hub a value of its own, so "set here" says nothing about the lock.
  assert.equal(unavailableBecause(state({ on: false, source: 'HUB' }), 'HUB', []), 'app.modules.off_in_workspace');
  assert.equal(unavailableBecause(state({ on: false, source: 'HUB' }), 'HUB', ['TIME']), 'app.modules.off_in_workspace');
  // Before the workspace's set is read nothing is offered.
  assert.equal(unavailableBecause(state({ on: false, source: 'HUB' }), 'HUB', undefined), 'app.modules.off_in_workspace');
});

test('a hub decides for itself what the workspace has on (UC-INS-21 check 3)', () => {
  assert.equal(unavailableBecause(state({ on: false, source: 'HUB' }), 'HUB', ['PLANNING']), undefined);
  assert.equal(unavailableBecause(state({ on: true, source: 'HUB' }), 'HUB', ['PLANNING']), undefined);
  assert.equal(unavailableBecause(state({ on: true, source: 'WORKSPACE' }), 'HUB', ['PLANNING']), undefined);
  assert.equal(unavailableBecause(state({ on: true, source: 'WORKSPACE', locked_by: 'WORKSPACE' }), 'HUB', ['PLANNING']), 'app.modules.locked_workspace');
  // The workspace's own switch does not ask its own set.
  assert.equal(unavailableBecause(state({ on: false, source: 'DEFAULT' }), 'WORKSPACE'), undefined);
});

test('only a value set here is handed back to the level above', () => {
  assert.equal(isSetHere(state({ on: true, source: 'WORKSPACE' }), 'WORKSPACE'), true);
  assert.equal(isSetHere(state({ on: true, source: 'PLAN' }), 'WORKSPACE'), false);
  assert.equal(isSetHere(state({ on: false, source: 'HUB' }), 'HUB'), true);
  assert.deepEqual(moduleCodes(state({ module: 'SERVICE_DESK' })), {
    name: 'app.modules.name_service_desk', hint: 'app.modules.hint_service_desk',
  });
});

test("a place's screens are drawn for its hub's set, never wider than the workspace's (UC-INS-21 check 3)", () => {
  assert.deepEqual(modulesHere(['PLANNING', 'TIME'], ['TIME']), ['TIME']);
  assert.deepEqual(modulesHere(['TIME'], ['PLANNING', 'TIME']), ['TIME']);
  assert.deepEqual(modulesHere(['PLANNING', 'TIME'], undefined), ['PLANNING', 'TIME']);
  assert.deepEqual(modulesHere(undefined, undefined), []);
});

test('the switch-off question says whom and which rules it reaches, as counted (check 4)', () => {
  assert.deepEqual(offReach({ module: 'WORKFLOWS', on: true, people: 4, rules: 2 }), [
    { code: 'app.modules.off_people', params: { count: 4 } },
    { code: 'app.modules.off_rules', params: { count: 2 } },
  ]);
  assert.deepEqual(offReach({ module: 'WORKFLOWS', on: true, people: 0, rules: 0 }), []);
  assert.deepEqual(offReach({ module: 'WORKFLOWS', on: false, people: 0 }), []);
  assert.deepEqual(offReach(undefined), []);
});
