// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Two versions side by side: what *Changes* lists between them, block by block.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { compareDocuments } from './configdiff.ts';

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
