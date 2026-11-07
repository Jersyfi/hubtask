// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Two versions side by side: what *Changes* lists between them, block by block.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { compareDocuments, differenceWords } from './configdiff.ts';

test('two versions differ by the blocks added, changed and removed between them', () => {
  const before = {
    types: { incident: { level: 'TASK', name: 'Incident' }, task: { level: 'TASK' } },
    roles: { lead: { name: 'Lead', permissions: ['READ'] } },
  };
  const after = {
    types: { incident: { name: 'Incident', level: 'TASK' }, task: { level: 'TASK', name: 'Task' } },
    fields: { cost: { kind: 'MONEY' } },
  };
  assert.deepEqual(
    compareDocuments(before, after).map(({ path, kind }) => [path, kind]),
    [
      ['fields.cost', 'ADDED'],
      ['roles.lead', 'REMOVED'],
      ['types.task', 'CHANGED'],
    ],
  );
});

test('a version compared with itself differs in nothing', () => {
  const document = { modules: { workflows: { switch: 'ON' } } };
  assert.deepEqual(compareDocuments(document, document), []);
});

test('a difference reads as what happened to which part, by its name where it has one', () => {
  assert.deepEqual(
    differenceWords({ path: 'types.incident', kind: 'CHANGED', before: { name: 'Incident' }, after: { name: 'Incident', level: 'TASK' } }),
    { code: 'app.templates.change_change', section: 'app.templates.section_types', key: 'Incident' },
  );
  assert.deepEqual(
    differenceWords({ path: 'fields.cost', kind: 'ADDED', after: { kind: 'MONEY' } }),
    { code: 'app.templates.change_add', section: 'app.templates.section_fields', key: 'cost' },
  );
  assert.equal(differenceWords({ path: 'roles.lead', kind: 'REMOVED', before: { name: 'Lead' } }).code, 'app.templates.change_remove');
  assert.equal(
    differenceWords({ path: 'modules.knowledge', kind: 'CHANGED', before: { switch: 'OFF' }, after: { switch: 'ON' } }).code,
    'app.templates.change_module_on',
  );
});
