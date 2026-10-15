// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The note that a connection left for the provider: what makes the callback finish a connection
// rather than a sign-in, and only for the flow's ten minutes.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { forgetConnect, isConnecting, rememberConnect, type Store } from './identities.ts';

function memory(): Store {
  const held = new Map<string, string>();
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => void held.set(key, value),
    removeItem: (key) => void held.delete(key),
  };
}

test('the note is believed for the flow’s ten minutes and no longer', () => {
  const store = memory();
  rememberConnect(store, 1_000);
  assert.equal(isConnecting(store, 1_000 + 9 * 60_000), true);
  assert.equal(isConnecting(store, 1_000 + 10 * 60_000), false);
  // An old note is dropped, so the next sign-in is a sign-in.
  assert.equal(isConnecting(store, 1_000), false);
});

test('no note, or one that cannot be read, is no trip', () => {
  const store = memory();
  assert.equal(isConnecting(store, 0), false);
  store.setItem('hubtask.connect-return', 'not json');
  assert.equal(isConnecting(store, 0), false);
});

test('the note is forgotten once taken', () => {
  const store = memory();
  rememberConnect(store, 5);
  forgetConnect(store);
  assert.equal(isConnecting(store, 5), false);
});
