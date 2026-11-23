// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What a level carries, in four groups, and the closed field's sentence (design-system.md §4).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { format } from '../i18n/format.ts';
import { SOURCE } from '../i18n/source.ts';
import { FIELD_GROUPS, fieldGroupOf, fieldGroups, fieldsSummary } from './fieldGroups.ts';

// The capabilities an item type may carry: the domain's constants (domain-model.md §2), which the
// manifest lists per type. The contract types them as strings, so the domain is the list.
const domain = readFileSync(new URL('../../../../../core/domain/model/work/CapabilityProfile.go', import.meta.url), 'utf8');
const CAPABILITIES = [...domain.matchAll(/^\s*Capability\w+\s+Capability = "([A-Z_]+)"/gmu)].map((match) => match[1] ?? '');
// The capabilities the catalogue names a label for.
const LABELLED = Object.keys(SOURCE)
  .filter((code) => code.startsWith('app.structure.field.'))
  .map((code) => code.slice('app.structure.field.'.length));

test('the domain list is read: fifteen capabilities', () => {
  assert.equal(CAPABILITIES.length, 15, `read ${CAPABILITIES.join(', ')}`);
});

test('every capability of the domain and of the catalogue stands in exactly one group by name', () => {
  for (const capability of new Set([...CAPABILITIES, ...LABELLED])) {
    const holders = Object.entries(FIELD_GROUPS).filter(([, members]) => (members as readonly string[]).includes(capability as string));
    assert.equal(holders.length, 1, `${capability} is named by ${holders.length} groups`);
  }
});

test('the four groups as proposed', () => {
  assert.deepEqual(Object.keys(FIELD_GROUPS), ['STATUS_TIME', 'PEOPLE', 'CONTENT', 'EXTENSIONS']);
  assert.deepEqual(FIELD_GROUPS.STATUS_TIME, ['COMPLETION', 'DUE_DATE', 'REMINDER', 'RECURRENCE']);
  assert.deepEqual(FIELD_GROUPS.PEOPLE, ['ASSIGNMENT', 'MEMBERS']);
  assert.deepEqual(FIELD_GROUPS.CONTENT, ['NOTES', 'LABELS', 'COMMENTS', 'COVER', 'ATTACHMENTS', 'HISTORY']);
  assert.deepEqual(FIELD_GROUPS.EXTENSIONS, ['BUCKET', 'CUSTOM_FIELDS', 'AMOUNT']);
});

test('a capability no group names goes to Extensions', () => {
  assert.equal(fieldGroupOf('SOMETHING_NEW'), 'EXTENSIONS');
  assert.equal(fieldGroupOf('NOTES'), 'CONTENT');
});

test('the options in their groups, in the groups order, a group of none left out', () => {
  const option = (value: string) => ({ value, label: value });
  const grouped = fieldGroups([option('NOTES'), option('COMPLETION'), option('LATER'), option('DUE_DATE')], (group) => group);
  assert.deepEqual(
    grouped.map((group) => [group.label, group.options.map((each) => each.value)]),
    [
      ['STATUS_TIME', ['COMPLETION', 'DUE_DATE']],
      ['CONTENT', ['NOTES']],
      ['EXTENSIONS', ['LATER']],
    ],
  );
});

const LABELS: Record<string, string> = { A: 'Dates', B: 'Notes', C: 'Cover', D: 'Amount', E: 'History' };
const options = Object.entries(LABELS).map(([value, label]) => ({ value, label }));
const en = (code: string, params: Record<string, string | number> = {}) => format(SOURCE[code] ?? code, params, 'en');
const chosen = (...off: string[]) => options.filter((each) => !off.includes(each.value));

test('the closed field: all on says All n', () => {
  assert.equal(fieldsSummary(chosen(), options, en, 'en'), 'All 5 fields');
  assert.equal(fieldsSummary([options[0]!], [options[0]!], en, 'en'), 'The one field');
});

test('the closed field: one or two off say all except them, three off show the chips', () => {
  assert.equal(fieldsSummary(chosen('D'), options, en, 'en'), 'All except Amount');
  assert.equal(fieldsSummary(chosen('B', 'D'), options, en, 'en'), 'All except Notes and Amount');
  assert.equal(fieldsSummary(chosen('B', 'C', 'D'), options, en, 'en'), undefined);
});

test('German: the same sentences in the catalogue words, with the plural', () => {
  const german = JSON.parse(readFileSync(new URL('../../../../../locales/de.json', import.meta.url), 'utf8')) as Record<string, string>;
  const de = (code: string, params: Record<string, string | number> = {}) => format(german[code] ?? code, params, 'de');
  assert.equal(fieldsSummary(chosen(), options, de, 'de'), 'Alle 5 Felder');
  assert.equal(fieldsSummary([options[0]!], [options[0]!], de, 'de'), 'Das eine Feld');
  assert.equal(fieldsSummary(chosen('B', 'D'), options, de, 'de'), 'Alle außer Notes und Amount');
});
