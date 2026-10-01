// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { ordered, readLastMethod, readLastProvider, rememberPassword, rememberProvider } from './lastMethod.ts';

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

// A password sign-in is recorded too: a provider used once a month ago does not stay on top of the
// card for somebody who has signed in with their password every day since.
test('a password sign-in is remembered, and a provider no longer comes first after it', () => {
  const held = new Map<string, string>();
  const storage = {
    getItem: (key: string) => held.get(key) ?? null,
    setItem: (key: string, value: string) => void held.set(key, value),
    removeItem: (key: string) => void held.delete(key),
  };
  Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true });
  try {
    rememberProvider('c');
    assert.equal(readLastProvider(), 'c');
    rememberPassword();
    assert.equal(readLastProvider(), undefined, 'the provider still leads after a password sign-in');
    assert.equal(readLastMethod(), 'PASSWORD');
    assert.deepEqual(ordered(providers, readLastProvider()), providers);
  } finally {
    delete (globalThis as { localStorage?: unknown }).localStorage;
  }
});
