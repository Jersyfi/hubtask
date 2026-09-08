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

test('a PATCH is announced as a merge patch, and nothing else is', async () => {
  for (const [method, mediaType] of [
    ['PATCH', 'application/merge-patch+json'],
    ['POST', 'application/json'],
    ['PUT', 'application/json'],
    ['DELETE', 'application/json'],
  ] as const) {
    const { fetch, calls } = recordingFetch(() => ok({}));
    const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });

    await transport.send(method, '/items/i1', { title: 'x' }, { timeoutMs: 1000 });

    assert.equal(new Headers(calls[0]?.init.headers).get('Content-Type'), mediaType, method);
  }
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

test('the whole problem document travels: detail code and field errors included', async () => {
  // What the frame needs to put a message under the field it belongs to rather than at the top of
  // the form (ADR-0025). None of it is interpreted here - `path` is the server's, and so is the
  // choice between `code` and the more specific `detail_code`.
  const body = {
    code: 'validation_failed',
    detail_code: 'items.title_too_long',
    params: { maximum: '200' },
    field_errors: [
      { path: 'title', code: 'usecase.field_required' },
      { path: 'due_date', code: 'items.due_date_in_past', params: { at: '2026-01-01' } },
    ],
    request_id: 'r-11',
  };
  const { fetch } = recordingFetch(() => new Response(JSON.stringify(body), { status: 422 }));

  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).get('/items', { timeoutMs: 1000 }),
    (error: unknown) => {
      if (!(error instanceof TransportError)) throw error;
      assert.equal(error.code, 'validation_failed');
      assert.equal(error.detailCode, 'items.title_too_long');
      assert.equal(error.fieldErrors.length, 2);
      assert.equal(error.fieldErrors[1]?.path, 'due_date');
      assert.equal(error.fieldErrors[1]?.params?.at, '2026-01-01');
      return true;
    },
  );
});

test('a problem with no field errors carries an empty list rather than nothing', async () => {
  // So that a caller never has to guard twice. `field_errors` is optional in the contract and a
  // `?? []` at every call site is a `??` somebody eventually forgets.
  const { fetch } = recordingFetch(() => new Response(JSON.stringify({ code: 'errors.not_found' }), { status: 404 }));
  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).get('/items/x', { timeoutMs: 1000 }),
    (error: unknown) => {
      if (!(error instanceof TransportError)) throw error;
      assert.deepEqual(error.fieldErrors, []);
      assert.equal(error.detailCode, undefined);
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

// ---- The stream (F3-04): a response that does not end, read from the body and never through
// `EventSource`, which cannot carry a bearer.

/** A body that a test feeds line by line, the way a server's chunks arrive. */
function feed() {
  let push: (chunk: Uint8Array) => void = () => {};
  let close: () => void = () => {};
  const encoder = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      push = (chunk) => controller.enqueue(chunk);
      close = () => controller.close();
    },
  });
  return { body, write: (text: string) => push(encoder.encode(text)), close: () => close() };
}

const STREAM = { connectTimeoutMs: 1000, idleTimeoutMs: 1000 };

test('a stream carries the bearer and the cursor, and reads events as they arrive', async () => {
  const { body, write, close } = feed();
  const { fetch, calls } = recordingFetch(() => new Response(body, {
    status: 200, headers: { 'Content-Type': 'text/event-stream' },
  }));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });

  const connection = await transport.stream('/stream', { ...STREAM, token: 'tok', lastEventId: 'c-41' });
  const headers = new Headers(calls[0]?.init.headers);
  assert.equal(headers.get('Authorization'), 'Bearer tok');
  assert.equal(headers.get('Last-Event-ID'), 'c-41');
  assert.equal(headers.get('Accept'), 'text/event-stream');
  assert.equal(calls[0]?.url, '/api/v1/stream');

  // A heartbeat comment, a retry hint, an event whose data spans two lines and arrives in two
  // chunks, and CRLF line endings: the framing, not the happy path.
  write(': heartbeat\n\nretry: 250\n\nid: c-42\r\nevent: work_item\r\ndata: {"op":"UPSERT",\n');
  write('data: "entity":"work_item"}\n\nid: c-43\nevent: container\ndata: {"op":"DELETE"}\n\n');
  close();

  const events = [];
  for await (const event of connection.events) events.push(event);

  assert.deepEqual(events, [
    { id: 'c-42', event: 'work_item', data: '{"op":"UPSERT",\n"entity":"work_item"}', retryMs: 250 },
    { id: 'c-43', event: 'container', data: '{"op":"DELETE"}', retryMs: 250 },
  ]);
});

test('a refused stream is a TransportError carrying the status, the code and Retry-After', async () => {
  const body = { code: 'unavailable', detail_code: 'sync.stream_unavailable', params: { reason: 'tenant_cap' } };
  const { fetch } = recordingFetch(() => new Response(JSON.stringify(body), {
    status: 503, headers: { 'Retry-After': '7', 'Content-Type': 'application/problem+json' },
  }));

  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).stream('/stream', STREAM),
    (error: unknown) => {
      assert.ok(error instanceof TransportError);
      assert.equal(error.status, 503);
      assert.equal(error.detailCode, 'sync.stream_unavailable');
      assert.equal(error.retryAfterMs, 7000);
      return true;
    },
  );
});

test('a cursor too old is a refusal the engine can tell apart', async () => {
  const body = { code: 'gone', detail_code: 'sync.cursor_too_old', params: { window_days: '90' } };
  const { fetch } = recordingFetch(() => new Response(JSON.stringify(body), { status: 410 }));

  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).stream('/stream', { ...STREAM, lastEventId: 'old' }),
    (error: unknown) => error instanceof TransportError && error.isCursorTooOld && !error.isCursorInvalid,
  );
});

test('a stream without deadlines is refused before it is opened', async () => {
  const { fetch, calls } = recordingFetch(() => ok({}));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });

  await assert.rejects(() => transport.stream('/stream', { connectTimeoutMs: 0, idleTimeoutMs: 1000 }), TypeError);
  await assert.rejects(() => transport.stream('/stream', { connectTimeoutMs: 1000, idleTimeoutMs: Number.NaN }), TypeError);
  assert.equal(calls.length, 0);
});

test('silence for longer than the idle deadline ends the stream rather than hanging it', async () => {
  const { body, write } = feed();
  const { fetch } = recordingFetch(() => new Response(body, { status: 200 }));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });

  const connection = await transport.stream('/stream', { connectTimeoutMs: 1000, idleTimeoutMs: 20 });
  write('id: c-1\ndata: {}\n\n');

  const events = [];
  for await (const event of connection.events) events.push(event);
  // The one event arrived; then nothing did, and the iteration ended on its own. Reconnecting
  // with the cursor is the engine's, and it needs the loop to end to do it.
  assert.equal(events.length, 1);
});

test('the caller ends the stream through its signal', async () => {
  const { body } = feed();
  const { fetch } = recordingFetch(() => new Response(body, { status: 200 }));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });
  const stop = new AbortController();

  const connection = await transport.stream('/stream', { ...STREAM, signal: stop.signal });
  const drained = (async () => {
    const events = [];
    for await (const event of connection.events) events.push(event);
    return events;
  })();
  stop.abort();

  assert.deepEqual(await drained, []);
});

// ---- The bytes (F3-04): the one request that leaves for an address the engine did not compose.

test('a byte transfer sends no bearer, honours its deadline and reports progress', async () => {
  const { fetch, calls } = recordingFetch(() => new Response(null, { status: 204 }));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });
  const bytes = new Uint8Array(200_000);
  const progress: [number, number][] = [];

  await transport.transfer({
    url: 'https://bucket.example.org/objects/o1?X-Amz-Signature=abc',
    method: 'PUT', body: bytes, contentType: 'image/png', timeoutMs: 5000,
    onProgress: (sent, total) => progress.push([sent, total]),
  });

  const call = calls[0];
  assert.equal(call?.url, 'https://bucket.example.org/objects/o1?X-Amz-Signature=abc');
  assert.equal(call?.init.method, 'PUT');
  const headers = new Headers(call?.init.headers);
  assert.equal(headers.get('Authorization'), null, 'a bearer left for a bucket');
  assert.equal(headers.get('Content-Type'), 'image/png');
  assert.equal(call?.init.credentials, 'omit');
  assert.ok(call?.init.signal instanceof AbortSignal, 'no deadline travelled with the bytes');

  assert.ok(progress.length >= 1);
  assert.deepEqual(progress.at(-1), [200_000, 200_000]);
  for (const [sent, total] of progress) assert.ok(sent <= total);
});

test('a byte transfer never carries a bearer even when the caller has one to hand', async () => {
  // The port has no `token` on a transfer at all, which is the stronger guarantee: there is no
  // field through which one could travel. This test pins the absence of the header regardless.
  const { fetch, calls } = recordingFetch(() => new Response(null, { status: 200 }));
  await new FetchTransport({ baseUrl: '/api/v1', fetch }).transfer({
    url: 'http://localhost/api/v1/media/m1:content?token=t', method: 'PUT',
    body: new Blob(['hello']), timeoutMs: 1000,
  });
  assert.equal(new Headers(calls[0]?.init.headers).has('Authorization'), false);
});

test('a byte transfer without a deadline is refused, and an aborted one is a timeout', async () => {
  const { fetch, calls } = recordingFetch(() => new Response(null, { status: 204 }));
  const transport = new FetchTransport({ baseUrl: '/api/v1', fetch });
  await assert.rejects(
    () => transport.transfer({ url: 'https://b/o', method: 'PUT', body: new Uint8Array(1), timeoutMs: 0 }),
    TypeError,
  );
  assert.equal(calls.length, 0);

  const aborting = recordingFetch(() => Promise.reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));
  const stop = new AbortController();
  stop.abort();
  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch: aborting.fetch }).transfer({
      url: 'https://b/o', method: 'PUT', body: new Uint8Array(1), timeoutMs: 1000, signal: stop.signal,
    }),
    (error: unknown) => error instanceof TransportError && error.kind === 'timeout',
  );
});

test('a bucket refusing the bytes is a problem with its status, readable body or not', async () => {
  const { fetch } = recordingFetch(() => new Response('<Error><Code>AccessDenied</Code></Error>', { status: 403 }));
  await assert.rejects(
    () => new FetchTransport({ baseUrl: '/api/v1', fetch }).transfer({
      url: 'https://b/o', method: 'PUT', body: new Uint8Array(3), timeoutMs: 1000,
    }),
    (error: unknown) => error instanceof TransportError && error.kind === 'problem' && error.status === 403,
  );
});
