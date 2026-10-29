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
