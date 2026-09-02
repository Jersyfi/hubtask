// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The one file in the client that calls `fetch`, and therefore the one place three promises can be
// checked rather than reviewed: every request carries its bearer, its idempotency key where it has
// one, and a deadline.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { FetchTransport } from '../src/FetchTransport.ts';
import { TransportError } from '../src/errors.ts';

/** A `fetch` that records the request and answers what the test says. */
function recordingFetch(answer: () => globalThis.Response | Promise<globalThis.Response>) {
  const calls: { url: string; init: RequestInit }[] = [];
  const fetch = (async (url: string | URL | Request, init: RequestInit = {}) => {
    calls.push({ url: String(url), init });
    return answer();
  }) as unknown as typeof globalThis.fetch;
  return { fetch, calls };
}

const ok = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), { status: 200, ...init });

test('a request carries the bearer, the key and the content type', async () => {
  const { fetch, calls } = recordingFetch(() => ok({ id: 'i1' }));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });

  await transport.send('POST', '/items', { title: 'x' }, {
    token: 'tok', idempotencyKey: 'k-1', timeoutMs: 1000,
  });

  const headers = new Headers(calls[0]?.init.headers);
  assert.equal(headers.get('Authorization'), 'Bearer tok');
  assert.equal(headers.get('Idempotency-Key'), 'k-1');
  assert.equal(headers.get('Content-Type'), 'application/json');
  assert.equal(calls[0]?.url, '/api/v1/items');
});

test('a read sends no content type and no body', async () => {
  const { fetch, calls } = recordingFetch(() => ok({}));
  await new FetchTransport({ baseUrl: '/api/v1', fetch }).get('/accounts/me', { timeoutMs: 1000 });

  assert.equal(calls[0]?.init.body, undefined);
  assert.equal(new Headers(calls[0]?.init.headers).get('Content-Type'), null);
});

test('a call without a deadline is refused before it is made', async () => {
  const { fetch, calls } = recordingFetch(() => ok({}));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });

  // Not a default of "forever": a call with no deadline is a connection nobody is waiting for any
  // more, which is the same defect on this side of the wire as on the other.
  for (const timeoutMs of [0, -1, Number.NaN, Number.POSITIVE_INFINITY]) {
    await assert.rejects(
      () => transport.get('/accounts/me', { timeoutMs }),
      TypeError,
      `timeoutMs ${timeoutMs} was accepted`,
    );
  }
  assert.equal(calls.length, 0, 'a request without a deadline reached the network');
});

test('a base URL with a trailing slash does not produce a double one', async () => {
  const { fetch, calls } = recordingFetch(() => ok({}));
  await new FetchTransport({ baseUrl: 'https://example.org/api/v1/', fetch })
    .get('/accounts/me', { timeoutMs: 1000 });

  assert.equal(calls[0]?.url, 'https://example.org/api/v1/accounts/me');
});

test('a problem document becomes a TransportError carrying its code', async () => {
  const body = { code: 'errors.forbidden', params: { scope: 'items:write' }, request_id: 'r-9' };
  const { fetch } = recordingFetch(() => new Response(JSON.stringify(body), { status: 403 }));

  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).get('/items', { timeoutMs: 1000 }),
    (error: unknown) => {
      // `assert.ok` narrows at runtime but not for the compiler, so the guard is explicit.
      if (!(error instanceof TransportError)) throw error;
      assert.equal(error.kind, 'problem');
      assert.equal(error.status, 403);
      // The code and its params travel; no sentence is invented here (ADR-0011).
      assert.equal(error.code, 'errors.forbidden');
      assert.equal(error.params?.scope, 'items:write');
      assert.equal(error.requestId, 'r-9');
      return true;
    },
  );
});

test('an empty answer is not an error', async () => {
  // 204 is what a delete answers, and parsing an empty body as JSON would turn a success into a
  // failure.
  const { fetch } = recordingFetch(() => new Response(null, { status: 204 }));
  const answer = await new FetchTransport({ baseUrl: '/api/v1', fetch })
    .send('DELETE', '/items/i1', undefined, { timeoutMs: 1000 });

  assert.equal(answer.status, 204);
  assert.equal(answer.body, undefined);
});

test('an answer that is not JSON is malformed rather than a crash', async () => {
  const { fetch } = recordingFetch(() => new Response('<html>a proxy said no</html>', { status: 200 }));

  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).get('/items', { timeoutMs: 1000 }),
    (error: unknown) => error instanceof TransportError && error.kind === 'malformed',
  );
});

test('a network that does not answer is offline, and a deadline is a timeout', async () => {
  // The two are different to a caller: one is worth retrying at once, the other after a wait.
  const refused = recordingFetch(() => {
    throw new TypeError('failed to fetch');
  });
  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch: refused.fetch }).get('/i', { timeoutMs: 1000 }),
    (error: unknown) => error instanceof TransportError && error.kind === 'offline' && error.isRetryable,
  );

  const slow = recordingFetch(() => {
    const aborted = new Error('the deadline passed');
    aborted.name = 'TimeoutError';
    throw aborted;
  });
  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch: slow.fetch }).get('/i', { timeoutMs: 1 }),
    (error: unknown) => error instanceof TransportError && error.kind === 'timeout' && error.isRetryable,
  );
});

test('the ETag is handed back, because the next write compares against it', async () => {
  const { fetch } = recordingFetch(() => ok({ id: 'i1' }, { headers: { ETag: '"7"' } }));
  const answer = await new FetchTransport({ baseUrl: '/api/v1', fetch })
    .get('/items/i1', { timeoutMs: 1000 });

  assert.equal(answer.etag, '"7"');
});

test('a refusal the server cannot explain is still retryable when it is a 5xx', async () => {
  const { fetch } = recordingFetch(() => new Response('', { status: 503 }));
  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).get('/i', { timeoutMs: 1000 }),
    (error: unknown) => error instanceof TransportError && error.isRetryable && error.code === undefined,
  );
});
