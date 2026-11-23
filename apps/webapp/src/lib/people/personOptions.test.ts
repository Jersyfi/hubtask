// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { personOptions } from './personOptions.ts';

const NAMES: Record<string, string> = { a: 'Ömer Kaya', b: 'Ada Kern', c: 'Ada Kern', d: 'Zoe Lind', me: 'Ben Ott' };
const ADDRESSES: Record<string, string> = { b: 'ada@north.example', c: 'ada.kern@south.example' };
const words = { nameOf: (id: string) => NAMES[id] ?? 'Somebody', addressOf: (id: string) => ADDRESSES[id], locale: 'de' };

test('people are offered by name under the locale, two of one name told apart by their address', () => {
  const options = personOptions(['d', 'a', 'c', 'b'], words);
  assert.deepEqual(options.map((each) => each.value), ['c', 'b', 'a', 'd'], 'Ö sorts with O under de, and the two Adas by their address');
  assert.deepEqual(options.map((each) => each.hint), ['ada.kern@south.example', 'ada@north.example', undefined, undefined]);
});

test('the locked reader stands first, and what one option carries besides is kept', () => {
  const options = personOptions(['d', 'me', 'b'], words, { first: 'me', extra: (id) => (id === 'me' ? { label: 'You', disabledReason: 'stays' } : {}) });
  assert.deepEqual(options.map((each) => [each.value, each.label, each.disabledReason]), [['me', 'You', 'stays'], ['b', 'Ada Kern', undefined], ['d', 'Zoe Lind', undefined]]);
});

test('an identifier named twice is offered once', () => {
  assert.equal(personOptions(['b', 'b'], words).length, 1);
});
