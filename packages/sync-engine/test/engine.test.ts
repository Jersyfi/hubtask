// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The engine, headless: a fake Transport and a clock that does not move (ADR-0033 §2).
//
// Nothing here renders anything, and that is the acceptance criterion rather than a convenience.
// The engine has to be exercisable without a browser because it is the first-party counterpart to
// `hubctl sync-conformance`, and a suite that needed a DOM could not be one.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine, type ResourceState } from '../src/SyncEngine.ts';
import { TransportError } from '../src/errors.ts';
import { FakeTransport, FixedClock } from './fakes.ts';

const ME = { path: '/accounts/me' };

/** Collects every state a subscriber is told about, in order. */
function record<T>(engine: SyncEngine, path: string) {
  const seen: ResourceState<T>[] = [];
  const stop = engine.subscribe<T>({ path }, (state) => seen.push(state));
  return { seen, stop };
}

test('a subscriber is told idle, then loading, then ready', async () => {
  const transport = new FakeTransport().answer('/accounts/me', { id: 'a1', locale: 'de' });
  const clock = new FixedClock();
  const engine = new SyncEngine({ transport, clock });

  const { seen } = record<{ id: string; locale: string }>(engine, ME.path);

  // Called at once with the current state, so a caller never has to ask what it missed.
  assert.deepEqual(seen[0], { status: 'idle' });
  assert.deepEqual(seen[1], { status: 'loading' });

  await engine.refresh(ME);

  const ready = seen.at(-1);
  assert.equal(ready?.status, 'ready');
  assert.deepEqual(ready?.status === 'ready' ? ready.data : undefined, { id: 'a1', locale: 'de' });
  // Stamped by the injected clock and not by the machine's, which is the whole reason it is a port.
  assert.equal(ready?.status === 'ready' ? ready.at : 0, clock.now());
});

test('a second subscriber is handed what the first already loaded', async () => {
  const transport = new FakeTransport().answer('/accounts/me', { id: 'a1' });
  const engine = new SyncEngine({ transport });

  record(engine, ME.path);
  await engine.refresh(ME);

  const { seen } = record(engine, ME.path);
  assert.equal(seen[0]?.status, 'ready', 'the second subscriber started from idle and loaded again');
  // Two subscribers, and the resource was not fetched twice on subscribe.
  assert.equal(transport.calls.filter((c) => c.path === '/accounts/me').length, 2);
});

test('a failure reaches the subscriber as a TransportError rather than as a throw', async () => {
  const transport = new FakeTransport().fail('/accounts/me',
    new TransportError('problem', { status: 403, code: 'access.insufficient_scope' }));
  const engine = new SyncEngine({ transport });

  const { seen } = record(engine, ME.path);
  await engine.refresh(ME);

  const failed = seen.at(-1);
  assert.equal(failed?.status, 'failed');
  const error = failed?.status === 'failed' ? failed.error : undefined;
  // The code, not a sentence: the renderer resolves it against the catalogue (ADR-0011, F1-07).
  assert.equal(error?.code, 'access.insufficient_scope');
  assert.equal(error?.status, 403);
});

test('anything that is not a TransportError becomes one', async () => {
  // A caller has one shape to render. A failure that escaped as something else would be a failure
  // the UI cannot name.
  const transport = new FakeTransport().fail('/accounts/me', new Error('a wire came loose'));
  const engine = new SyncEngine({ transport });

  const { seen } = record(engine, ME.path);
  await engine.refresh(ME);

  const failed = seen.at(-1);
  assert.equal(failed?.status, 'failed');
  assert.ok(failed?.status === 'failed' && failed.error instanceof TransportError);
});

test('the bearer is asked for per call, never held', async () => {
  const transport = new FakeTransport().answer('/accounts/me', {});
  let token: string | undefined = 'first';
  const engine = new SyncEngine({ transport, token: () => token });

  await engine.refresh(ME);
  // A sign-out, then a sign-in as somebody else. A token copied at construction would still be
  // the first one - which is the bug this shape exists to prevent.
  token = 'second';
  await engine.refresh(ME);

  assert.equal(transport.calls[0]?.options.token, 'first');
  assert.equal(transport.calls[1]?.options.token, 'second');
});

test('every call carries a deadline', async () => {
  const transport = new FakeTransport().answer('/accounts/me', {});
  const engine = new SyncEngine({ transport });

  await engine.refresh(ME);
  await engine.mutate('POST', '/items', { title: 'x' });

  for (const call of transport.calls) {
    assert.ok(
      Number.isFinite(call.options.timeoutMs) && call.options.timeoutMs > 0,
      `${call.method} ${call.path} was made without a deadline`,
    );
  }
});

test('an idempotency key is passed through and never minted here', async () => {
  const transport = new FakeTransport().answer('/items', { id: 'i1' });
  const engine = new SyncEngine({ transport });

  await engine.mutate('POST', '/items', { title: 'x' }, { idempotencyKey: 'k-1' });
  await engine.mutate('POST', '/items', { title: 'y' });

  assert.equal(transport.calls[0]?.options.idempotencyKey, 'k-1');
  // Absent where the caller sent none: an intent is the caller's to delimit, and a key minted per
  // attempt would make a retry a second operation.
  assert.equal(transport.calls[1]?.options.idempotencyKey, undefined);
});

test('a write invalidates what was read', async () => {
  const transport = new FakeTransport().answer('/accounts/me', { id: 'a1' }).answer('/items', {});
  const engine = new SyncEngine({ transport });

  await engine.refresh(ME);
  assert.equal(engine.peek(ME).status, 'ready');

  await engine.mutate('POST', '/items', {});
  assert.equal(engine.peek(ME).status, 'idle', 'a stale read survived a write');
});

test('unsubscribing stops the listener and leaves the state', async () => {
  const transport = new FakeTransport().answer('/accounts/me', { id: 'a1' });
  const engine = new SyncEngine({ transport });

  const { seen, stop } = record(engine, ME.path);
  await engine.refresh(ME);
  const before = seen.length;

  stop();
  await engine.refresh(ME);

  assert.equal(seen.length, before, 'a listener was called after it unsubscribed');
  // A component that unmounts and remounts finds what it had.
  assert.equal(engine.peek(ME).status, 'ready');
});

test('a refused credential is reported once, from wherever it was refused', async () => {
  // The engine is the only place that sees every 401. A client that noticed a dead token on one
  // screen and not on another would keep making requests with a credential it already knows about.
  const refused = new TransportError('problem', { status: 401, code: 'unauthenticated' });
  const transport = new FakeTransport().fail('/accounts/me', refused).fail('/items', refused);
  let refusals = 0;
  const engine = new SyncEngine({ transport, onUnauthorized: () => (refusals += 1) });

  await engine.refresh(ME);
  assert.equal(refusals, 1, 'a read that was refused');

  await assert.rejects(() => engine.mutate('POST', '/items', {}));
  assert.equal(refusals, 2, 'and a write, which never reaches a resource state');
});

test('a failure that is not a refusal says nothing about the credential', async () => {
  const transport = new FakeTransport().fail('/accounts/me', new TransportError('problem', { status: 500 }));
  let refusals = 0;
  const engine = new SyncEngine({ transport, onUnauthorized: () => (refusals += 1) });
  await engine.refresh(ME);
  assert.equal(refusals, 0);
});

test('reset forgets everything, which is what sign-out means', async () => {
  const transport = new FakeTransport().answer('/accounts/me', { id: 'a1' });
  const engine = new SyncEngine({ transport });

  const { seen, stop } = record(engine, ME.path);
  await engine.refresh(ME);
  stop();

  engine.reset();
  assert.equal(engine.peek(ME).status, 'idle');
  assert.equal(seen.length > 0, true);
});

test('an expired access token is exchanged once and the request is retried', async () => {
  // What a person never sees: fifteen minutes pass, the next read is refused, the pair is
  // exchanged behind the screen, and the read succeeds (F4-03).
  const refused = new TransportError('problem', { status: 401, code: 'unauthenticated' });
  const transport = new FakeTransport().failOnce('/accounts/me', refused).answer('/accounts/me', { id: 'a' });
  let exchanges = 0;
  let refusals = 0;
  const engine = new SyncEngine({
    transport,
    onUnauthorized: () => (refusals += 1),
    onRefresh: async () => {
      exchanges += 1;
      return true;
    },
  });

  const state = await engine.refresh<{ id: string }>(ME);

  assert.equal(state.status, 'ready');
  assert.equal(exchanges, 1, 'one exchange');
  assert.equal(refusals, 0, 'and the session did not end');
  assert.equal(transport.calls.length, 2, 'the refused call and its retry');
});

test('ten concurrent refusals share one exchange', async () => {
  // Presenting a retired refresh token is theft as far as the server is concerned and costs the
  // whole family (security.md §5). Ten requests meeting one expired token must not present it ten
  // times.
  const refused = new TransportError('problem', { status: 401, code: 'unauthenticated' });
  const transport = new FakeTransport();
  const paths = Array.from({ length: 10 }, (_, index) => `/items/${index}`);
  for (const path of paths) transport.failOnce(path, refused).answer(path, { id: path });

  let exchanges = 0;
  const engine = new SyncEngine({
    transport,
    onRefresh: async () => {
      exchanges += 1;
      // The exchange is a request of its own, so it does not settle in the same microtask the
      // refusals arrive in - which is the situation the single-flight has to survive.
      await new Promise((resolve) => setTimeout(resolve, 5));
      return true;
    },
  });

  const states = await Promise.all(paths.map((path) => engine.refresh<{ id: string }>({ path })));

  assert.equal(exchanges, 1, 'one exchange for ten refusals');
  for (const state of states) assert.equal(state.status, 'ready');
});

test('a second refusal after a fresh credential ends the session', async () => {
  // Not an expiry: the account, the scope, or the session itself. Arguing past a server that has
  // answered twice is not a client's job.
  const refused = new TransportError('problem', { status: 401, code: 'unauthenticated' });
  const transport = new FakeTransport().fail('/accounts/me', refused);
  let exchanges = 0;
  let refusals = 0;
  const engine = new SyncEngine({
    transport,
    onUnauthorized: () => (refusals += 1),
    onRefresh: async () => {
      exchanges += 1;
      return true;
    },
  });

  await engine.refresh(ME);

  assert.equal(exchanges, 1, 'exchanged once');
  assert.equal(refusals, 1, 'and then gave up');
  assert.equal(transport.calls.length, 2, 'without a third attempt');
});

test('a refused exchange ends the session at once', async () => {
  const refused = new TransportError('problem', { status: 401, code: 'unauthenticated' });
  const transport = new FakeTransport().fail('/accounts/me', refused);
  let refusals = 0;
  const engine = new SyncEngine({
    transport,
    onUnauthorized: () => (refusals += 1),
    onRefresh: async () => false,
  });

  await engine.refresh(ME);

  assert.equal(refusals, 1);
  assert.equal(transport.calls.length, 1, 'nothing was retried with a credential nobody replaced');
});

test('a permission refusal is not a session refusal', async () => {
  // A 403 is an answer, not an expiry. Exchanging on one would be a client asking for a better
  // answer to a question it already got right.
  const transport = new FakeTransport().fail('/accounts/me',
    new TransportError('problem', { status: 403, code: 'forbidden' }));
  let exchanges = 0;
  const engine = new SyncEngine({ transport, onRefresh: async () => (exchanges += 1) > 0 });

  await engine.refresh(ME);

  assert.equal(exchanges, 0);
});
