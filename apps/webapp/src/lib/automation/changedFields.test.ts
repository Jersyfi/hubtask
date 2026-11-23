// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A rule's changed fields are named in the reader's language, never by their wire names.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { SOURCE } from '../i18n/source.ts';
import { CHANGED_FIELDS } from './changedFields.ts';

const GERMAN = JSON.parse(readFileSync(new URL('../../../../../locales/de.json', import.meta.url), 'utf8')) as Record<string, string>;

test('every changed field has a name in English and German, and none is its wire name', () => {
  for (const [field, code] of Object.entries(CHANGED_FIELDS)) {
    for (const [language, catalogue] of [['en', SOURCE], ['de', GERMAN]] as const) {
      const name = catalogue[code];
      assert.ok(typeof name === 'string' && name.length > 0, `${field}: ${code} has no ${language} name`);
      assert.notEqual(name, field.replace(/_/gu, ' '), `${field} is named by its wire name in ${language}`);
    }
  }
});

test('the eight fields the trigger may carry, in the order the inspector offers them', () => {
  assert.deepEqual(Object.keys(CHANGED_FIELDS), ['title', 'notes', 'due_at', 'start_at', 'assignee_id', 'bucket_id', 'parent_id', 'completed']);
});
