// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { REFRESH_KEY, TOKEN_KEY, tokenStore, type TokenStorage } from './tokenStore.ts';

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

const pair = { access: 'access-1', refresh: 'refresh-1' };

test('a pair written is a pair read back - which is what surviving a reload means', () => {
  const storage = fakeStorage();
  tokenStore(storage).write(pair);
  // A reload is a new store over the same storage, which is exactly this.
  const reloaded = tokenStore(storage);
  assert.equal(reloaded.read(), 'access-1');
  assert.equal(reloaded.readRefresh(), 'refresh-1');
});

test('the exchange replaces both halves', () => {
  // The exchange retires the token it was given, so a client keeping the old one beside a new
  // access token would be holding a credential the server has already treated as spent.
  const storage = fakeStorage();
  const store = tokenStore(storage);
  store.write(pair);
  store.write({ access: 'access-2', refresh: 'refresh-2' });
  assert.equal(store.read(), 'access-2');
  assert.equal(store.readRefresh(), 'refresh-2');
  assert.equal(tokenStore(storage).readRefresh(), 'refresh-2', 'and in the storage, not only in memory');
});

test('clearing leaves nothing behind, in the store or in the storage', () => {
  const storage = fakeStorage();
  const store = tokenStore(storage);
  store.write(pair);
  store.clear();
  assert.equal(store.read(), undefined);
  assert.equal(store.readRefresh(), undefined);
  assert.deepEqual(storage.values, {}, 'both keys are gone, not blanked');
  assert.equal(tokenStore(storage).read(), undefined);
});

test('an empty string is no credential rather than an empty one', () => {
  const store = tokenStore(fakeStorage({ [TOKEN_KEY]: '', [REFRESH_KEY]: '' }));
  assert.equal(store.read(), undefined);
  assert.equal(store.readRefresh(), undefined);
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
  store.write(pair);
  assert.equal(store.read(), 'access-1', 'held in memory when the storage will not');
  assert.equal(store.readRefresh(), 'refresh-1');
  store.clear();
  assert.equal(store.read(), undefined);
});

test('no storage at all behaves the same way', () => {
  const store = tokenStore(undefined);
  store.write(pair);
  assert.equal(store.read(), 'access-1');
  assert.equal(store.readRefresh(), 'refresh-1');
  store.clear();
  assert.equal(store.read(), undefined);
});

test('the elevation survives a reload, and a new session is not the old one\'s hour', () => {
  // The walk of SI-17 found this: each screen of `/instance` is its own component, and a full page
  // load threw the elevation away. The door then asked for a second proof while the first hour
  // still stood at the server — and passing it started a *new* hour, which is exactly what "the
  // elevation does not slide" forbids (ADR-0070 §4).
  const storage = fakeStorage();
  const store = tokenStore(storage);
  store.write({ access: 'access-1', refresh: 'refresh-1' });
  store.rememberElevatedUntil('2026-09-28T10:00:00Z');

  assert.equal(
    tokenStore(storage).readElevatedUntil(),
    '2026-09-28T10:00:00Z',
    'a reload forgot when the hour ends',
  );

  // A second sign-in in the same tab is somebody else as far as this is concerned.
  tokenStore(storage).write({ access: 'access-2', refresh: 'refresh-2' });
  assert.equal(
    tokenStore(storage).readElevatedUntil(),
    undefined,
    'a new session inherited the previous one\'s elevation',
  );

  // And signing out takes it with the pair.
  const signedIn = tokenStore(storage);
  signedIn.rememberElevatedUntil('2026-09-28T11:00:00Z');
  signedIn.clear();
  assert.equal(tokenStore(storage).readElevatedUntil(), undefined);
});
