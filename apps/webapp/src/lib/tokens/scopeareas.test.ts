// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { areaOf, areaRows, configurationParts, toggleVerb, verbState } from './scopeareas.ts';

const OFFERED = ['items:write', 'items:read', 'containers:read', 'audit:export', 'audit:read', 'config:read', 'config:apply', 'config:apply:roles', 'brand_new:read'];

test('a scope lands in its resource\'s area, an unknown one in other', () => {
  assert.equal(areaOf('items:read'), 'work');
  assert.equal(areaOf('agent:destructive'), 'automation');
  assert.equal(areaOf('config:apply:roles'), 'configuration');
  assert.equal(areaOf('brand_new:read'), 'other');
});

test('the rows are by area, read before write, each verb with every scope it ticks', () => {
  const rows = areaRows(OFFERED);
  assert.deepEqual(rows.map((row) => row.area), ['work', 'records', 'other', 'configuration']);
  assert.deepEqual(rows[0]!.verbs, [
    { verb: 'read', scopes: ['items:read', 'containers:read'] },
    { verb: 'write', scopes: ['items:write'] },
  ]);
  assert.deepEqual(rows[1]!.verbs.map((each) => each.verb), ['read', 'export']);
  // The configuration's parts are offered one by one, not under its verb.
  assert.deepEqual(rows[3]!.verbs.map((each) => each.scopes), [['config:read'], ['config:apply']]);
  assert.deepEqual(configurationParts(OFFERED), ['config:apply:roles']);
});

test('a verb ticks or clears all of its scopes and shows whether some are chosen', () => {
  const scopes = ['items:read', 'containers:read'];
  assert.equal(verbState([], scopes), 'none');
  assert.equal(verbState(['items:read'], scopes), 'some');
  const on = toggleVerb(['audit:read', 'items:read'], scopes, true);
  assert.deepEqual(on, ['audit:read', 'items:read', 'containers:read']);
  assert.equal(verbState(on, scopes), 'all');
  assert.deepEqual(toggleVerb(on, scopes, false), ['audit:read']);
});
