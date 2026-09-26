// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ordered } from './lastMethod.ts';

const providers = [{ id: 'a' }, { id: 'b' }, { id: 'c' }];

test('without a remembered method the server’s order stands', () => {
  assert.deepEqual(ordered(providers, undefined), providers);
});

test('the one used last comes first, and the rest keep their order', () => {
  assert.deepEqual(ordered(providers, 'c'), [{ id: 'c' }, { id: 'a' }, { id: 'b' }]);
});

test('a remembered provider that is no longer offered changes nothing', () => {
  // Switched off for a week: the list is what it is, and the memory is not cleared - it is the
  // right answer again the moment the provider comes back.
  assert.deepEqual(ordered(providers, 'gone'), providers);
});

test('the list is copied rather than sorted in place', () => {
  const given = [...providers];
  ordered(given, 'b');
  assert.deepEqual(given, providers);
});
