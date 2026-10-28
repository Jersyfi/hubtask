// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// View as file (configuration/ui.md A10): a page's values as its place's part of the document.

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { asFile } from './asFile.ts';

test('a value set here is written with its value, one from above is left to the level above', () => {
  const file = asFile(
    [
      { key: 'module.planning', value: true, source: 'WORKSPACE' },
      { key: 'module.time', value: false, source: 'DEFAULT' },
      { key: 'module.agents', value: false, source: 'PLAN', lockedBy: 'PLAN' },
    ],
    'WORKSPACE',
  );
  assert.deepEqual(JSON.parse(file), {
    module: {
      planning: { set: true, value: true, locked: false },
      time: { set: false, locked: false },
      agents: { set: false, locked: false },
    },
  });
});

test('a lock held at this place is written as locked', () => {
  const file = asFile([{ key: 'sign_in.min_length', value: 12, source: 'INSTANCE', lockedBy: 'INSTANCE' }], 'INSTANCE');
  assert.deepEqual(JSON.parse(file), { sign_in: { min_length: { set: true, value: 12, locked: true } } });
});

test('values of several areas are grouped by area', () => {
  const file = asFile(
    [
      { key: 'sign_in.min_length', value: 12, source: 'WORKSPACE' },
      { key: 'legal.terms_url', value: 'https://example.org/terms', source: 'WORKSPACE' },
    ],
    'WORKSPACE',
  );
  assert.deepEqual(Object.keys(JSON.parse(file)), ['sign_in', 'legal']);
});
