// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { keystoreTokens, parseHeld, readWorkspace, writeWorkspace, type SessionKeeper } from './appSession.ts';

/** A web storage that records every key written to it. */
function recordingStorage() {
  const items = new Map<string, string>();
  return {
    items,
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => void items.set(key, value),
    removeItem: (key: string) => void items.delete(key),
    clear: () => items.clear(),
    key: (index: number) => [...items.keys()][index] ?? null,
    get length() {
      return items.size;
    },
  };
}

function keeper(): SessionKeeper & { held: string | undefined } {
  const state = {
    held: undefined as string | undefined,
    keep(value: string) {
      state.held = value;
    },
    forget() {
      state.held = undefined;
    },
  };
  return state;
}

const SERVER = 'https://tasks.example.com';

// UC-SYN-07 check 3: the app's session is kept in the platform's keystore - and no token reaches
// the webview's storage on the way, whatever the session goes through.
test('no token of the app reaches the webview storage', () => {
  const session = recordingStorage();
  const local = recordingStorage();
  const previous = { session: globalThis.sessionStorage, local: globalThis.localStorage };
  Object.defineProperty(globalThis, 'sessionStorage', { value: session, configurable: true });
  Object.defineProperty(globalThis, 'localStorage', { value: local, configurable: true });
  try {
    const keystore = keeper();
    const store = keystoreTokens(SERVER, undefined, keystore);
    store.write({ access: 'hbt_sat_first', refresh: 'hbt_srt_first' });
    store.rememberAccount('account-1');
    store.rememberElevatedUntil('2026-10-28T15:00:00Z');
    store.rotate({ access: 'hbt_sat_second', refresh: 'hbt_srt_second' });
    writeWorkspace(local, { server: SERVER, host: 'tasks.example.com', name: 'Acme Ltd' });

    assert.equal(store.read(), 'hbt_sat_second');
    assert.equal(store.readRefresh(), 'hbt_srt_second');
    assert.equal(session.items.size, 0);
    for (const value of [...session.items.values(), ...local.items.values()]) {
      assert.doesNotMatch(value, /hbt_s[ar]t_/);
    }
    assert.match(keystore.held ?? '', /hbt_srt_second/);

    store.clear();
    assert.equal(keystore.held, undefined);
    assert.equal(store.read(), undefined);
  } finally {
    Object.defineProperty(globalThis, 'sessionStorage', { value: previous.session, configurable: true });
    Object.defineProperty(globalThis, 'localStorage', { value: previous.local, configurable: true });
  }
});

// UC-SYN-01 check 7 and UC-SYN-07 check 3: a restart reads the same session back from the keystore,
// the account the copy is opened for with it.
test('a restart finds the session and its account in the keystore', () => {
  const keystore = keeper();
  const before = keystoreTokens(SERVER, undefined, keystore);
  before.write({ access: 'a1', refresh: 'r1' });
  before.rememberAccount('account-1');

  const after = keystoreTokens(SERVER, parseHeld(keystore.held), keeper());
  assert.equal(after.read(), 'a1');
  assert.equal(after.readRefresh(), 'r1');
  assert.equal(after.readAccount(), 'account-1');
});

test('a refresh keeps the elevation and a new sign-in drops it', () => {
  const keystore = keeper();
  const store = keystoreTokens(SERVER, undefined, keystore);
  store.write({ access: 'a1', refresh: 'r1' });
  store.rememberAccount('account-1');
  store.rememberElevatedUntil('2026-10-28T15:00:00Z');
  store.rotate({ access: 'a2', refresh: 'r2' });
  assert.equal(store.readElevatedUntil(), '2026-10-28T15:00:00Z');
  assert.equal(store.readAccount(), 'account-1');
  store.write({ access: 'a3', refresh: 'r3' });
  assert.equal(store.readElevatedUntil(), undefined);
  assert.equal(store.readAccount(), undefined);
});

test('a session of another workspace is not this run\'s', () => {
  const held = parseHeld(JSON.stringify({ server: 'https://other.example.com', access: 'a', refresh: 'r' }));
  assert.equal(keystoreTokens(SERVER, held, keeper()).read(), undefined);
});

test('only a whole session parses', () => {
  for (const raw of [undefined, '', 'nope', '{}', JSON.stringify({ server: SERVER, access: 'a' }), 42]) {
    assert.equal(parseHeld(raw), undefined, String(raw));
  }
  assert.deepEqual(parseHeld(JSON.stringify({ server: SERVER, access: 'a', refresh: 'r', account: 'x' })), {
    server: SERVER, access: 'a', refresh: 'r', account: 'x',
  });
});

test('the workspace is remembered without a credential', () => {
  const local = recordingStorage();
  assert.equal(readWorkspace(local), undefined);
  writeWorkspace(local, { server: SERVER, host: 'tasks.example.com' });
  assert.deepEqual(readWorkspace(local), { server: SERVER, host: 'tasks.example.com' });
  local.setItem('hubtask.app.workspace', 'garbage');
  assert.equal(readWorkspace(local), undefined);
});
