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
    { path: 'modules.WORKFLOWS', people: 3, rules: 2 },
    { path: 'modules.GOALS', people: 1, rules: 0 },
  ]);
  assert.deepEqual(lines.map(({ code }) => code), [
    'app.changes.reach_entries',
    'app.changes.reach_hidden',
    'app.changes.reach_values',
    'app.changes.reach_people',
    'app.changes.reach_module',
    'app.changes.reach_module',
    'app.changes.reach_rules',
    'app.changes.reach_module',
  ]);
  assert.deepEqual(lines[2]?.params, { path: 'fields.cost', count: 7 });
  assert.deepEqual(lines[3]?.params, { path: 'roles.lead', accounts: 5, groups: 1, gained: 1, lost: 0 });
  assert.deepEqual(lines[4]?.params, { path: 'modules.TIME', count: 12 });
  assert.deepEqual(lines[6]?.params, { path: 'modules.WORKFLOWS', count: 2 });
});

test('a form that closes an unattended door says which door and which fields', () => {
  const lines = reachLines([
    { path: 'types.incident', intake: [{ channel: 'RULE', fields: ['custom_fields.severity', 'due_date'] }, { channel: 'MAIL', fields: ['due_date'] }] },
  ]);
  assert.deepEqual(lines, [
    { code: 'app.changes.reach_intake_rule', params: { path: 'types.incident', fields: 'custom_fields.severity, due_date' } },
    { code: 'app.changes.reach_intake_mail', params: { path: 'types.incident', fields: 'due_date' } },
  ]);
});

test('a door the server counted says how many rules or mailboxes it is', () => {
  const lines = reachLines([
    { path: 'types.task', intake: [{ channel: 'RULE', fields: ['due_date'], doors: 3 }, { channel: 'MAIL', fields: ['due_date'], doors: 0 }, { channel: 'IMPORT', fields: ['due_date'] }] },
  ]);
  assert.deepEqual(lines, [
    { code: 'app.changes.reach_intake_rule_counted', params: { path: 'types.task', fields: 'due_date', count: 3 } },
    { code: 'app.changes.reach_intake_mail_counted', params: { path: 'types.task', fields: 'due_date', count: 0 } },
    { code: 'app.changes.reach_intake_import', params: { path: 'types.task', fields: 'due_date' } },
  ]);
});

test('a reach withheld from the reader is said once and counts nothing', () => {
  const lines = reachLines([{ path: 'workflows.service.shape', withheld: true, entries: 4 }]);
  assert.deepEqual(lines, [{ code: 'app.changes.reach_withheld', params: { path: 'workflows.service.shape' } }]);
  assert.deepEqual(reachLines(undefined), []);
});

test('a reach names its block by the section and key, a module by its product', async () => {
  const { reachPlace } = await import('./configreach.ts');
  assert.deepEqual(reachPlace('workflows.service.shape'), { section: 'app.templates.section_workflows', key: 'service' });
  assert.deepEqual(reachPlace('modules.TIME'), { section: 'app.templates.section_modules', module: 'app.modules.name_time' });
  assert.deepEqual(reachPlace('roles'), { section: 'app.templates.section_roles' });
  assert.deepEqual(reachPlace('/fields/cost'), { section: 'app.templates.section_fields', key: 'cost' });
});
