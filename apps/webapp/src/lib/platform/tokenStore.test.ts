// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { TOKEN_KEY, tokenStore, type TokenStorage } from './tokenStore.ts';

function fakeStorage(initial: Record<string, string> = {}): TokenStorage & { values: Record<string, string> } {
  const values = { ...initial };
  return {
    values,
    getItem: (key) => values[key] ?? null,
    setItem: (key, value) => {
      values[key] = value;
    },
    removeItem: (key) => {
      delete values[key];
    },
  };
}

test('a token written is a token read back - which is what surviving a reload means', () => {
  const storage = fakeStorage();
  tokenStore(storage).write('hbt_pat_x');
  // A reload is a new store over the same storage, which is exactly this.
  assert.equal(tokenStore(storage).read(), 'hbt_pat_x');
});

test('clearing leaves nothing behind, in the store or in the storage', () => {
  const storage = fakeStorage();
  const store = tokenStore(storage);
  store.write('hbt_pat_x');
  store.clear();
  assert.equal(store.read(), undefined);
  assert.deepEqual(storage.values, {}, 'the key itself is gone, not blanked');
  assert.equal(tokenStore(storage).read(), undefined);
});

test('an empty string is no credential rather than an empty one', () => {
  assert.equal(tokenStore(fakeStorage({ [TOKEN_KEY]: '' })).read(), undefined);
});

test('a storage that refuses everything still allows a session', () => {
  // Private mode, a policy, a cleared origin. Signing in has to keep working; it simply does not
  // survive a reload, which is the honest consequence rather than a failure to sign in at all.
  const hostile: TokenStorage = {
    getItem() {
      throw new Error('denied');
    },
    setItem() {
      throw new Error('denied');
    },
    removeItem() {
      throw new Error('denied');
    },
  };
  const store = tokenStore(hostile);
  assert.equal(store.read(), undefined);
  store.write('hbt_pat_x');
  assert.equal(store.read(), 'hbt_pat_x', 'held in memory when the storage will not');
  store.clear();
  assert.equal(store.read(), undefined);
});

test('no storage at all behaves the same way', () => {
  const store = tokenStore(undefined);
  store.write('hbt_pat_x');
  assert.equal(store.read(), 'hbt_pat_x');
  store.clear();
  assert.equal(store.read(), undefined);
});
