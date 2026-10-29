// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { CustomFieldDefinition } from '@hubtask/sync-engine';

import {
  FILTER_PREFIX,
  appliesTo,
  definitionsFor,
  isCalculated,
  isValidCurrency,
  isValidKey,
  isWorkspaceWide,
  operatorsFor,
  sortsBy,
  queryFieldsFor,
  takesCurrency,
  takesOptions,
  valueFor,
  valueText,
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

test('the comparisons offered are a subset of what the grammar permits for the kind', () => {
  // `core/domain/model/view/Field.go` accepts these for every key; `CustomShape.go` holds the
  // ranges to the kinds that have an order. Nothing here may add to that, or the editor would
  // offer a row the server refuses by name.
  const SERVER = new Set(['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL', 'CONTAINS']);
  const RANGES = new Set(['LT', 'LTE', 'GT', 'GTE', 'BETWEEN']);
  const ORDERED = new Set(['TEXT', 'NUMBER', 'DATE', 'SELECT', 'BOOL', 'URL', 'MONEY', 'DURATION']);
  const kinds = [
    'TEXT', 'NUMBER', 'DATE', 'SELECT', 'MULTI_SELECT', 'BOOL', 'USER', 'URL',
    'MONEY', 'DURATION', 'REFERENCE', 'USER_SET', 'RICH_TEXT', 'GEO',
  ];
  for (const kind of kinds) {
    for (const op of operatorsFor(kind)) {
      const permitted = SERVER.has(op) || (RANGES.has(op) && ORDERED.has(kind));
      assert.ok(permitted, `${kind} offers ${op}, which the grammar does not permit`);
    }
  }
  assert.deepEqual(operatorsFor('BOOL'), ['EQ', 'NEQ', 'IS_NULL']);
  assert.deepEqual(operatorsFor('MULTI_SELECT'), ['CONTAINS', 'IS_NULL']);
  assert.deepEqual(operatorsFor('USER_SET'), ['CONTAINS', 'IS_NULL']);
  assert.ok(operatorsFor('MONEY').includes('BETWEEN'));
  assert.ok(!operatorsFor('REFERENCE').includes('LT'));
  // A kind from a newer server is filterable rather than invisible, on what is true of any value.
  assert.deepEqual(operatorsFor('GEO'), ['EQ', 'NEQ', 'IS_NULL']);
});

// UC-WRK-17 check 10: a custom field is a sort and a grouping where its kind has one order, and
// the client says so exactly where the server does (`CustomShape.sorts`, `CustomShape.groups`).
test('a custom field sorts and groups where its kind has one value with an order', () => {
  const fields = queryFieldsFor([
    definition({ key: 'budget', kind: 'MONEY', currency: 'EUR' }),
    definition({ key: 'owner', kind: 'USER' }),
    definition({ key: 'tags', kind: 'MULTI_SELECT', options: ['a'] }),
    definition({ key: 'spec', kind: 'RICH_TEXT' }),
    definition({ key: 'parent_entry', kind: 'REFERENCE' }),
  ]);
  const by = (key: string) => fields.find((field) => field.field === FILTER_PREFIX + key);
  assert.equal(by('budget')?.sortable, true);
  assert.equal(by('budget')?.groupable, true);
  assert.equal(by('owner')?.sortable, false);
  assert.equal(by('owner')?.groupable, true);
  assert.equal(by('tags')?.sortable, false);
  assert.equal(by('tags')?.groupable, false);
  assert.equal(by('spec')?.groupable, false);
  assert.equal(by('parent_entry')?.groupable, true);
  assert.equal(by('parent_entry')?.kind, 'id');
});

test('the new kinds travel in their contract shape', () => {
  assert.equal(valueFor('MONEY', '12.5'), 12.5);
  assert.equal(valueFor('DURATION', 90), 90);
  assert.deepEqual(valueFor('USER_SET', ['a', 'b']), ['a', 'b']);
  assert.equal(valueFor('USER_SET', []), null);
  assert.equal(valueFor('REFERENCE', ' 0192f000-0000-7000-8000-00000000000e '), '0192f000-0000-7000-8000-00000000000e');
  // Formatted text keeps its leading indentation, which Markdown reads.
  assert.equal(valueFor('RICH_TEXT', '    code\n'), '    code\n');
  assert.equal(valueFor('RICH_TEXT', '  '), null);
});

test('only a MONEY field names a currency, as three capital letters', () => {
  assert.equal(takesCurrency('MONEY'), true);
  assert.equal(takesCurrency('NUMBER'), false);
  assert.equal(isValidCurrency('EUR'), true);
  assert.equal(isValidCurrency('eur'), false);
  assert.equal(isValidCurrency('EURO'), false);
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
  // A number has one order, so it is a sort and a grouping as well as a filter.
  assert.equal(fields[0]?.sortable, true);
  assert.equal(fields[0]?.groupable, true);
  assert.equal(fields[0]?.nullable, true);
});

test('a stored value reads the way the reader reads it, people and entries by name', () => {
  const reading = {
    number: (value: number) => String(value).replace('.', ','),
    amount: (value: number, currency: string) => `${String(value).replace('.', ',')} ${currency}`,
    duration: (minutes: number) => `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, '0')}`,
    nameOf: (id: string) => ({ a: 'Amelie', b: 'Jonas' })[id],
    titleOf: (id: string) => (id === 'e-1' ? 'Order chairs' : undefined),
    unknown: 'Not visible',
  };
  assert.equal(valueText({ kind: 'MONEY', currency: 'EUR' }, 12.5, reading), '12,5 EUR');
  assert.equal(valueText({ kind: 'DURATION' }, 90, reading), '1:30');
  assert.equal(valueText({ kind: 'USER_SET' }, ['a', 'b', 'x'], reading), 'Amelie, Jonas, Not visible');
  assert.equal(valueText({ kind: 'REFERENCE' }, 'e-1', reading), 'Order chairs');
  assert.equal(valueText({ kind: 'REFERENCE' }, 'e-2', reading), 'Not visible');
  assert.equal(valueText({ kind: 'RICH_TEXT' }, '\n# Scope\nmore', reading), '# Scope');
  assert.equal(valueText({ kind: 'USER_SET' }, [], reading), undefined);
  assert.equal(valueText({ kind: 'TEXT' }, '', reading), undefined);
});

test('a calculated field reads, sorts and filters as a number, and is nobody to fill in', () => {
  assert.equal(isCalculated('CALCULATED'), true);
  assert.equal(isCalculated('NUMBER'), false);
  assert.equal(sortsBy('CALCULATED'), true);
  assert.ok(operatorsFor('CALCULATED').includes('BETWEEN'));
});
