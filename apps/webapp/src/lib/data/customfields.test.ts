// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { CustomFieldDefinition } from '@hubtask/sync-engine';

import {
  FILTER_PREFIX,
  appliesTo,
  definitionsFor,
  isValidKey,
  isWorkspaceWide,
  operatorsFor,
  queryFieldsFor,
  takesOptions,
  valueFor,
} from './customfields.ts';

function definition(over: Partial<CustomFieldDefinition> = {}): CustomFieldDefinition {
  return {
    id: 'f-1',
    collection_id: 'c-1',
    key: 'effort',
    kind: 'NUMBER',
    options: [],
    is_required: false,
    applies_to: ['TASK'],
    version: 1,
    ...over,
  } as CustomFieldDefinition;
}

test('a key is an identifier, and the pattern is the contract’s', () => {
  assert.equal(isValidKey('effort'), true);
  assert.equal(isValidKey('effort_2'), true);
  // A leading digit, an upper-case letter, a hyphen and an empty key are each a name no definition
  // can carry — and the grammar would accept it as a filter that could only match nothing.
  assert.equal(isValidKey('2effort'), false);
  assert.equal(isValidKey('Effort'), false);
  assert.equal(isValidKey('effort-2'), false);
  assert.equal(isValidKey(''), false);
  assert.equal(isValidKey('e'.repeat(51)), false);
});

test('a definition with no collection is the workspace’s', () => {
  assert.equal(isWorkspaceWide(definition({ collection_id: null })), true);
  assert.equal(isWorkspaceWide(definition()), false);
});

test('a type carries only the fields that name it', () => {
  const forTasks = definition({ key: 'effort', applies_to: ['TASK'] });
  const forBoth = definition({ key: 'client', applies_to: ['TASK', 'WORK_PACKAGE'] });

  assert.equal(appliesTo(forBoth, 'WORK_PACKAGE'), true);
  assert.equal(appliesTo(forTasks, 'WORK_PACKAGE'), false);
  assert.deepEqual(
    definitionsFor([forTasks, forBoth], 'WORK_PACKAGE').map((d) => d.key),
    ['client'],
  );
});

test('only a SELECT and a MULTI_SELECT have options', () => {
  assert.equal(takesOptions('SELECT'), true);
  assert.equal(takesOptions('MULTI_SELECT'), true);
  assert.equal(takesOptions('TEXT'), false);
});

test('a NUMBER travels as a number, and a number that is not one travels as it was typed', () => {
  // A string sent for a NUMBER is `validation_failed` with the field path. Sending null instead
  // would clear a value nobody asked to clear, which is worse than a refusal that names the field.
  assert.equal(valueFor('NUMBER', '3.5'), 3.5);
  assert.equal(valueFor('NUMBER', 3), 3);
  assert.equal(valueFor('NUMBER', 'three'), 'three');
  assert.equal(valueFor('NUMBER', ''), null);
});

test('an empty value clears the key rather than setting it to nothing', () => {
  // "Not set" and "set to the empty string" are different states, and only the first is what
  // clearing a field means.
  assert.equal(valueFor('TEXT', ''), null);
  assert.equal(valueFor('TEXT', '   '), null);
  assert.equal(valueFor('TEXT', ' hello '), 'hello');
  assert.equal(valueFor('MULTI_SELECT', []), null);
  assert.deepEqual(valueFor('MULTI_SELECT', ['a', 'b']), ['a', 'b']);
  assert.equal(valueFor('BOOL', false), false);
  assert.equal(valueFor('TEXT', null), null);
});

test('the comparisons offered are a subset of the six the grammar permits', () => {
  // `core/domain/model/view/Field.go` offers the same six for every kind, because the kind is not
  // known when a filter is validated. Nothing here may add to them, or the editor would offer a
  // row the server refuses by name.
  const SERVER = new Set(['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL', 'CONTAINS']);
  for (const kind of ['TEXT', 'NUMBER', 'DATE', 'SELECT', 'MULTI_SELECT', 'BOOL', 'USER', 'URL', 'GEO']) {
    for (const op of operatorsFor(kind)) {
      assert.ok(SERVER.has(op), `${kind} offers ${op}, which the grammar does not permit`);
    }
  }
  assert.deepEqual(operatorsFor('BOOL'), ['EQ', 'NEQ', 'IS_NULL']);
  assert.deepEqual(operatorsFor('MULTI_SELECT'), ['CONTAINS', 'IS_NULL']);
  // A kind from a newer server is filterable rather than invisible, on what is true of any value.
  assert.deepEqual(operatorsFor('GEO'), ['EQ', 'NEQ', 'IS_NULL']);
});

test('a definition becomes a filter field under the grammar’s prefix', () => {
  const fields = queryFieldsFor([
    definition({ key: 'effort', kind: 'NUMBER' }),
    definition({ key: 'stage', kind: 'SELECT', options: ['draft', 'final'] }),
  ]);

  assert.deepEqual(fields.map((f) => f.field), ['custom_fields.effort', 'custom_fields.stage']);
  assert.equal(FILTER_PREFIX, 'custom_fields.');
  // `number` is the grammar's own name for a value a definition shapes; a column never has it.
  assert.equal(fields[0]?.kind, 'number');
  assert.deepEqual(fields[1]?.values, ['draft', 'final']);
  // Filterable and neither sortable nor groupable, which is what the server says too: an order
  // over a jsonb value is the same mistake `LT` would be.
  assert.equal(fields[0]?.sortable, false);
  assert.equal(fields[0]?.groupable, false);
  assert.equal(fields[0]?.nullable, true);
});
