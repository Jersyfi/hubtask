// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// F1-09's acceptance criterion that no fake can satisfy: "a call through the engine reaches a
// running server and returns typed data".
//
// The other suites hand `FetchTransport` a `fetch` of their own, which proves what the transport
// *does* with an answer and nothing about whether it can obtain one. This one starts a real HTTP
// server, lets the real global `fetch` reach it, and asserts on what actually arrived over the
// wire - the header the server received, not the header the test set.
//
// A server of eighty lines rather than the product's: what is under test is this package, and
// booting a Go binary and a PostgreSQL to prove that a bearer reaches a socket would test
// everything except the thing that could be wrong. The product's own end-to-end path is
// `scripts/hubctl-e2e.sh` and the Compose gate.

import { test, after, before } from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import type { AddressInfo } from 'node:net';

import { SyncEngine } from '../src/SyncEngine.ts';
import { FetchTransport } from '../src/FetchTransport.ts';
import { TransportError } from '../src/errors.ts';
import { FixedClock } from './fakes.ts';

/** What the server saw. Asserted on instead of what the client meant to send. */
interface Received {
  method: string;
  url: string;
  authorization: string | undefined;
  idempotencyKey: string | undefined;
  body: string;
}

const received: Received[] = [];
let server: http.Server;
let baseUrl: string;

before(async () => {
  server = http.createServer((request, response) => {
    let body = '';
    request.on('data', (chunk) => (body += chunk));
    request.on('end', () => {
      received.push({
        method: request.method ?? '',
        url: request.url ?? '',
        authorization: request.headers.authorization,
        idempotencyKey: request.headers['idempotency-key'] as string | undefined,
        body,
      });

      if (request.url === '/api/v1/accounts/me') {
        response.writeHead(200, { 'Content-Type': 'application/json', ETag: '"3"' });
        response.end(JSON.stringify({
          id: '0192f000-0000-7000-8000-0000000000e1',
          kind: 'USER',
          display_name: 'Anna Beispiel',
          status: 'ACTIVE',
          locale: 'de',
          time_zone: 'Europe/Berlin',
        }));
        return;
      }

      if (request.url === '/api/v1/forbidden') {
        // A real problem document, byte for byte the shape the server sends (RFC 9457).
        response.writeHead(403, { 'Content-Type': 'application/problem+json' });
        response.end(JSON.stringify({
          type: 'about:blank',
          title: 'Forbidden',
          status: 403,
          code: 'errors.forbidden',
          request_id: 'req-42',
        }));
        return;
      }

      response.writeHead(404, { 'Content-Type': 'application/json' });
      response.end('{}');
    });
  });

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  baseUrl = `http://127.0.0.1:${(server.address() as AddressInfo).port}/api/v1`;
});

after(async () => {
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())));
});

function engineAgainstTheServer(token?: string): SyncEngine {
  return new SyncEngine({
    // The real transport with the real global fetch. Nothing is stubbed below this line.
    transport: new FetchTransport({ baseUrl }),
    clock: new FixedClock(),
    token: () => token,
  });
}

test('a read reaches the server and comes back as typed data', async () => {
  received.length = 0;
  const engine = engineAgainstTheServer('a-real-bearer');

  interface Account {
    id: string;
    locale?: string;
    time_zone?: string;
  }
  const state = await engine.refresh<Account>({ path: '/accounts/me' });

  assert.equal(state.status, 'ready');
  const data = state.status === 'ready' ? state.data : undefined;
  // The two fields F1-10 and F1-11 exist to consume, arriving over a socket rather than from a map.
  assert.equal(data?.locale, 'de');
  assert.equal(data?.time_zone, 'Europe/Berlin');

  // What the server received, not what the client believes it sent.
  assert.equal(received[0]?.method, 'GET');
  assert.equal(received[0]?.url, '/api/v1/accounts/me');
  assert.equal(received[0]?.authorization, 'Bearer a-real-bearer');
});

test('a write carries its idempotency key over the wire', async () => {
  received.length = 0;
  const engine = engineAgainstTheServer('a-real-bearer');

  await engine.mutate('POST', '/accounts/me', { locale: 'nl' }, { idempotencyKey: 'k-77' });

  assert.equal(received[0]?.method, 'POST');
  assert.equal(received[0]?.idempotencyKey, 'k-77');
  assert.equal(received[0]?.body, JSON.stringify({ locale: 'nl' }));
});

test('an anonymous call sends no Authorization header at all', async () => {
  received.length = 0;
  await engineAgainstTheServer(undefined).refresh({ path: '/accounts/me' });

  // Absent, not `Bearer undefined`: an empty credential is a credential, and the server would have
  // to decide what to do with it.
  assert.equal(received[0]?.authorization, undefined);
});

test("a real problem document becomes the error the renderer can name", async () => {
  const engine = engineAgainstTheServer('a-real-bearer');

  const state = await engine.refresh({ path: '/forbidden' });

  assert.equal(state.status, 'failed');
  const error = state.status === 'failed' ? state.error : undefined;
  assert.ok(error instanceof TransportError);
  assert.equal(error?.status, 403);
  assert.equal(error?.code, 'errors.forbidden');
  assert.equal(error?.requestId, 'req-42');
  // A 403 is the server saying "not ever", not "not now".
  assert.equal(error?.isRetryable, false);
});

test('a deadline that passes is a timeout rather than a hang', async () => {
  // A path the server never answers, and one millisecond to do it in.
  const slow = http.createServer(() => {
    /* deliberately silent */
  });
  await new Promise<void>((resolve) => slow.listen(0, '127.0.0.1', resolve));
  const port = (slow.address() as AddressInfo).port;

  const transport = new FetchTransport({ baseUrl: `http://127.0.0.1:${port}` });
  await assert.rejects(
    () => transport.get('/never', { timeoutMs: 25 }),
    (error: unknown) => {
      if (!(error instanceof TransportError)) throw error;
      assert.equal(error.kind, 'timeout');
      assert.equal(error.isRetryable, true);
      return true;
    },
  );

  await new Promise<void>((resolve) => slow.close(() => resolve()));
});
