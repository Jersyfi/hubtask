// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A plan's reach as a person reads it before applying (UC-CFG-01 check 6): every count the server
// answered is a line, and a reach withheld from the reader counts nothing.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { reachLines } from './configreach.ts';

test('every count of a plan is a line of its reach', () => {
  const lines = reachLines([
    { path: 'workflows.service.shape', entries: 4 },
    { path: 'hubs.support.structure', hidden_entries: 2, hidden_values: 3 },
    { path: 'fields.cost', hidden_values: 7 },
    { path: 'roles.lead', accounts: 5, groups: 1, gained: ['STRUCTURE'], lost: [] },
    { path: 'modules.TIME', people: 12 },
  ]);
  assert.deepEqual(lines.map(({ code }) => code), [
    'app.changes.reach_entries',
    'app.changes.reach_hidden',
    'app.changes.reach_values',
    'app.changes.reach_people',
    'app.changes.reach_module',
  ]);
  assert.deepEqual(lines[2]?.params, { path: 'fields.cost', count: 7 });
  assert.deepEqual(lines[3]?.params, { path: 'roles.lead', accounts: 5, groups: 1, gained: 1, lost: 0 });
  assert.deepEqual(lines[4]?.params, { path: 'modules.TIME', count: 12 });
});

test('a reach withheld from the reader is said once and counts nothing', () => {
  const lines = reachLines([{ path: 'workflows.service.shape', withheld: true, entries: 4 }]);
  assert.deepEqual(lines, [{ code: 'app.changes.reach_withheld', params: { path: 'workflows.service.shape' } }]);
  assert.deepEqual(reachLines(undefined), []);
});
