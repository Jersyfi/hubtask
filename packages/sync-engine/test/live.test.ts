// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { SyncEngine, TransportError } from '../src/index.ts';
import { FakeTransport } from './fakes.ts';

test('a live document socket carries the bearer as its subprotocol, never in the address', () => {
  const opened: { path: string; protocols: string[] }[] = [];
  let token = 'first';
  const engine = new SyncEngine({
    transport: new FakeTransport(),
    token: () => token,
    socket: (path, protocols) => {
      opened.push({ path, protocols });
      return { path };
    },
  });
  engine.liveSocket('/realtime/documents/page/p1');
  token = 'second';
  engine.liveSocket('/realtime/documents/page/p1');
  assert.deepEqual(opened, [
    { path: '/realtime/documents/page/p1', protocols: ['hubtask.v1', 'hubtask.bearer.first'] },
    { path: '/realtime/documents/page/p1', protocols: ['hubtask.v1', 'hubtask.bearer.second'] },
  ]);
});

test('without a way to open one, a live socket is refused as needing a connection', () => {
  const engine = new SyncEngine({ transport: new FakeTransport() });
  assert.throws(() => engine.liveSocket('/realtime/documents/page/p1'), (error: unknown) => error instanceof TransportError && error.code === 'sync.needs_connection');
});

/** A socket the test closes with a code, as the server would. */
function closableSocket() {
  const listeners: ((event: { code: number }) => void)[] = [];
  return {
    addEventListener(type: string, listener: (event: { code: number }) => void) {
      if (type === 'close') listeners.push(listener);
    },
    closeWith(code: number) {
      for (const listener of listeners) listener({ code });
    },
  };
}

test('a live socket the server closes for its credential renews the pair for the reconnect', async () => {
  let renewals = 0;
  let refused = 0;
  const socket = closableSocket();
  const engine = new SyncEngine({
    transport: new FakeTransport(),
    token: () => 'stale',
    onRefresh: async () => { renewals++; return true; },
    onUnauthorized: () => { refused++; },
    socket: () => socket,
  });
  engine.liveSocket('/realtime/documents/page/p1');
  socket.closeWith(1006);
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(renewals, 0, 'a dropped connection is no refused credential');

  socket.closeWith(4401);
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(renewals, 1);
  assert.equal(refused, 0);
});

test('a live socket closed for its credential that cannot be renewed ends the session', async () => {
  let refused = 0;
  const socket = closableSocket();
  const engine = new SyncEngine({
    transport: new FakeTransport(),
    onRefresh: async () => false,
    onUnauthorized: () => { refused++; },
    socket: () => socket,
  });
  engine.liveSocket('/realtime/documents/page/p1');
  socket.closeWith(4401);
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(refused, 1);
});
