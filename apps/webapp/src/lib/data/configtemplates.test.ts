// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The *Templates* gallery: four origins in order, what each template offers, and an upgrade that
// never overwrites what the workspace changed itself without the person choosing it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { byOrigin, offerOf, takenFirst } from './configtemplates.ts';

test('the gallery draws the four origins in order and leaves an empty one out', () => {
  const groups = byOrigin([
    { key: 'agency', origin: 'WORKSPACE' as const },
    { key: 'list', origin: 'HUBTASK' as const },
    { key: 'mine', origin: 'OWN' as const },
  ]);
  assert.deepEqual(groups.map((group) => group.origin), ['HUBTASK', 'WORKSPACE', 'OWN']);
});

test('a template not taken is offered, a taken one only when a newer version exists', () => {
  assert.equal(offerOf({ origin: 'HUBTASK', taken_version: null, update_available: false }), 'TAKE');
  assert.equal(offerOf({ origin: 'WORKSPACE', taken_version: 2, update_available: true }), 'UPDATE');
  assert.equal(offerOf({ origin: 'WORKSPACE', taken_version: 3, update_available: false }), 'IN_USE');
});

test('an upgrade takes what only the template changed and leaves every conflict', () => {
  assert.deepEqual(
    takenFirst([
      { path: 'types.incident', state: 'TAKE' },
      { path: 'roles.lead', state: 'CONFLICT' },
      { path: 'structure.views', state: 'TAKE' },
    ]),
    ['types.incident', 'structure.views'],
  );
});
