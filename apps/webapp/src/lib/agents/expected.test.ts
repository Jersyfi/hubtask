// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { expectedFields } from './expected.ts';

test('a case\'s expected answer reads as fields, not as JSON', () => {
  assert.deepEqual(expectedFields({ priority: 'P2', labels: ['vpn', 'network'], urgent: true, count: 3 }), [
    { name: 'priority', value: 'P2' },
    { name: 'labels', value: 'vpn, network' },
    { name: 'urgent', value: 'true' },
    { name: 'count', value: '3' },
  ]);
});

test('a nested value is written out by its own fields; an empty one is a dash', () => {
  assert.deepEqual(expectedFields({ route: { group: 'Network', level: 2 }, note: null, tags: [] }), [
    { name: 'route', value: 'group: Network, level: 2' },
    { name: 'note', value: '–' },
    { name: 'tags', value: '–' },
  ]);
});

test('an answer that is no object is one unnamed field', () => {
  assert.deepEqual(expectedFields('yes'), [{ name: '', value: 'yes' }]);
  assert.deepEqual(expectedFields(undefined), []);
});
