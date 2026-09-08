// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The change stream, headless: a scripted Transport, and a wait nobody spends.
//
// What is asserted here is not that events arrive - that is the transport's test - but what the
// engine *does* with them, and what it does with each of the four ways the stream refuses. Loads
// are counted rather than described, because "the board reloaded" and "the board reloaded twice"
// look identical in a screenshot and are two different defects.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine, RECONNECT_BASE_MS } from '../src/SyncEngine.ts';
import { TransportError } from '../src/errors.ts';
import type { ChangeRecord } from '../src/schema.ts';
import type { StreamEvent } from '../src/ports.ts';
import { FakeTransport } from './fakes.ts';

const ITEM = '3f1b0b7e-0000-4000-8000-000000000001';
const OTHER = '3f1b0b7e-0000-4000-8000-000000000002';
const CONTAINER = '3f1b0b7e-0000-4000-8000-0000000000c1';

/** One frame as the server sends it: the cursor as `id`, the entity as `event`, the record as data. */
function frame(id: string, record: Partial<ChangeRecord> & { entity: string }): StreamEvent {
  return { id, event: record.entity, data: JSON.stringify({ op: 'UPSERT', ...record }) };
}

/**
 * The application's mapping, as small as a real one is: an entry is a path, a container is a
 * subtree, and an entity this client does not read is nothing at all.
 *
 * It lives in the test rather than in the engine for the same reason it lives in the application
 * in production - the engine does not know what a hub is (ADR-0033 §2).
 */
function pathsFor(record: ChangeRecord): readonly string[] {
  switch (record.entity) {
    case 'work_item':
      return [`/items/${record.entity_id}`];
    case 'container':
      return [`/containers/${record.entity_id}`];
    default:
      return [];
  }
}

/**
 * A wait that costs nothing and stops the loop.
 *
 * The connection loop is deliberately endless, so a test drives it by counting the pauses and
 * ending the listener at the one it wanted to see. Waiting for real would make this suite as slow
 * as the backoff it asserts.
 */
function pauses(stopAfter = Number.POSITIVE_INFINITY) {
  const seen: number[] = [];
  let unsubscribe = () => {};
  const wait = async (ms: number) => {
    seen.push(ms);
    if (seen.length >= stopAfter) unsubscribe();
  };
  return {
    seen,
    wait,
    /** Hands the loop's stop over, so the wait can end a loop that would otherwise not stop. */
    hold: (stop: () => void) => {
      unsubscribe = stop;
    },
    end: () => unsubscribe(),
  };
}

/** Lets every scheduled microtask and timer of the loop run before the assertions read the result. */
async function settle(turns = 6): Promise<void> {
  for (let i = 0; i < turns; i += 1) await new Promise((resolve) => setTimeout(resolve, 0));
}

test('a record re-reads what it names, and leaves what it does not alone', async () => {
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM, title: 'one' })
    .answer(`/items/${OTHER}`, { id: OTHER, title: 'two' })
    .streamSessions({
      events: [frame('7', { entity: 'work_item', entity_id: ITEM })],
      open: true,
    });
  const engine = new SyncEngine({ transport });
  const pause = pauses(1);

  engine.subscribe({ path: `/items/${ITEM}` }, () => {});
  engine.subscribe({ path: `/items/${OTHER}` }, () => {});
  await settle();
  const before = transport.calls.length;

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  const loads = transport.calls.slice(before);
  assert.deepEqual(
    loads.map((call) => call.path),
    [`/items/${ITEM}`],
    'the record named one entry, and the other was re-read anyway',
  );
});

test('a record nobody is watching is forgotten rather than fetched', async () => {
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM })
    .streamSessions({ events: [frame('7', { entity: 'work_item', entity_id: ITEM })], open: true });
  const engine = new SyncEngine({ transport });
  const pause = pauses(1);

  // Read once and let go of it: a cache nobody has open.
  await engine.refresh({ path: `/items/${ITEM}` });
  const before = transport.calls.length;

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  assert.equal(transport.calls.length, before, 'an unwatched entry was re-fetched for nobody');
  assert.equal(engine.peek({ path: `/items/${ITEM}` }).status, 'idle', 'the stale cache was kept');
});

test('an entity this client does not read changes nothing', async () => {
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM })
    .streamSessions({
      events: [frame('7', { entity: 'identity_provider', entity_id: OTHER })],
      open: true,
    });
  const engine = new SyncEngine({ transport });
  const pause = pauses(1);

  engine.subscribe({ path: `/items/${ITEM}` }, () => {});
  await settle();
  const before = transport.calls.length;

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  // The empty mapping must not read as "everything": that is the difference between a record this
  // client ignores and a write that declared nothing.
  assert.equal(transport.calls.length, before);
});

test('the reconnect asks for the position the stream last sent', async () => {
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM })
    .streamSessions(
      { events: [frame('7', { entity: 'work_item', entity_id: ITEM }), frame('9', { entity: 'work_item', entity_id: ITEM })] },
      { open: true },
    );
  const engine = new SyncEngine({ transport, token: () => 'tok' });
  const pause = pauses();

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  assert.equal(transport.streams.length, 2, 'the closed stream was not reopened');
  assert.equal(transport.streams[0]?.lastEventId, undefined, 'the first connection invented a cursor');
  assert.equal(transport.streams[1]?.lastEventId, '9', 'the reconnect lost the position');
  assert.equal(transport.streams[1]?.token, 'tok', 'the reconnect went out without a bearer');
});

test('the cursor advances on a frame this client has no mapping for', async () => {
  const transport = new FakeTransport().streamSessions(
    { events: [frame('11', { entity: 'identity_provider', entity_id: OTHER })] },
    { open: true },
  );
  const engine = new SyncEngine({ transport });
  const pause = pauses();

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  // Asking for it again would be asking for a record the server has already handed over.
  assert.equal(transport.streams[1]?.lastEventId, '11');
});

test('a busy server is waited out for exactly as long as it asked', async () => {
  const transport = new FakeTransport().streamSessions(
    { refuse: new TransportError('problem', { status: 503, code: 'sync.stream_unavailable', retryAfterMs: 12_000 }) },
    { open: true },
  );
  const engine = new SyncEngine({ transport });
  const pause = pauses(1);

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  assert.deepEqual(pause.seen, [12_000], 'the server said when, and the engine came back sooner');
});

test('a connection that will not open backs off, doubling', async () => {
  const transport = new FakeTransport().streamSessions({ refuse: new TransportError('offline') });
  const engine = new SyncEngine({ transport });
  const pause = pauses(3);

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle(12);
  pause.end();

  assert.deepEqual(pause.seen, [RECONNECT_BASE_MS, RECONNECT_BASE_MS * 2, RECONNECT_BASE_MS * 4]);
});

test('a cursor older than the tombstone window empties the engine and starts again without one', async () => {
  const tooOld = new TransportError('problem', { status: 410, code: 'conflict', detailCode: 'sync.cursor_too_old' });
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM })
    .answer(`/items/${OTHER}`, { id: OTHER })
    .streamSessions(
      { events: [frame('4', { entity: 'work_item', entity_id: ITEM })] },
      { refuse: tooOld },
      { open: true },
    );
  const engine = new SyncEngine({ transport });
  const pause = pauses();

  engine.subscribe({ path: `/items/${OTHER}` }, () => {});
  await settle();
  const before = transport.calls.length;

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle(10);
  pause.end();

  // A delta across a gap would be silently wrong, so everything held is read again - including
  // the entry no record ever named (offline-sync.md §7).
  assert.ok(
    transport.calls.slice(before).some((call) => call.path === `/items/${OTHER}`),
    'a full resynchronisation left an entry the stream had never mentioned untouched',
  );
  assert.equal(transport.streams.at(-1)?.lastEventId, undefined, 'the refused cursor was sent again');
  // One pause, and it belongs to the stream that closed cleanly. The refusal itself is not waited
  // out: there is nothing to wait for, because what is being dropped is the cursor.
  assert.deepEqual(pause.seen, [RECONNECT_BASE_MS], 'a refused cursor was waited out like a busy server');
});

test('a cursor this installation never minted starts the stream again without one', async () => {
  const invalid = new TransportError('problem', { status: 410, code: 'conflict', detailCode: 'sync.cursor_invalid' });
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM })
    .streamSessions(
      { events: [frame('4', { entity: 'work_item', entity_id: ITEM })] },
      { refuse: invalid },
      { open: true },
    );
  const engine = new SyncEngine({ transport });
  const pause = pauses();

  engine.subscribe({ path: `/items/${ITEM}` }, () => {});
  await settle();
  const loadsBefore = transport.calls.length;

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle(10);
  pause.end();

  assert.equal(transport.streams.at(-1)?.lastEventId, undefined, 'the refused cursor was sent again');
  // One reload, from the record that did arrive. Nothing held is known to be wrong, so nothing
  // else is dropped - which is what separates this from a cursor that is too old.
  assert.equal(transport.calls.length, loadsBefore + 1);
});

test('a refused credential ends the session rather than reconnecting', async () => {
  const transport = new FakeTransport().streamSessions({
    refuse: new TransportError('problem', { status: 401, code: 'unauthorized' }),
  });
  let refusals = 0;
  const engine = new SyncEngine({ transport, onUnauthorized: () => { refusals += 1; } });

  const stop = engine.listen({ pathsFor, wait: async () => {} });
  await settle(8);
  stop();

  assert.equal(refusals, 1, 'the one place that sees every 401 did not say so');
  assert.equal(transport.streams.length, 1, 'the engine kept reconnecting with a credential the server refused');
});

test('access revoked on a container reaches the screen that has it open', async () => {
  const revoked = new TransportError('problem', { status: 403, code: 'forbidden' });
  const transport = new FakeTransport()
    .answer(`/containers/${CONTAINER}`, { id: CONTAINER })
    .streamSessions({
      events: [frame('12', { entity: 'container', entity_id: CONTAINER, op: 'ACCESS_REVOKED', container_id: CONTAINER })],
      open: true,
    });
  const engine = new SyncEngine({ transport });
  const pause = pauses(1);

  const seen: string[] = [];
  engine.subscribe({ path: `/containers/${CONTAINER}` }, (state) => seen.push(state.status));
  await settle();
  transport.fail(`/containers/${CONTAINER}`, revoked);

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  // Dropping it quietly would leave a person looking at a container they may no longer read. The
  // re-read is refused, and the refusal is what the screen renders.
  assert.equal(seen.at(-1), 'failed');
});

test('an unreadable frame is skipped rather than ending the stream', async () => {
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM })
    .streamSessions({
      events: [
        { id: '1', event: 'work_item', data: 'not json' },
        { id: '2', event: 'work_item', data: '{"op":"UPSERT"}' },
        frame('3', { entity: 'work_item', entity_id: ITEM }),
      ],
      open: true,
    });
  const engine = new SyncEngine({ transport });
  const pause = pauses(1);

  engine.subscribe({ path: `/items/${ITEM}` }, () => {});
  await settle();
  const before = transport.calls.length;

  pause.hold(engine.listen({ pathsFor, wait: pause.wait }));
  await settle();
  pause.end();

  assert.equal(transport.calls.length, before + 1, 'the readable frame behind the broken one was lost');
  assert.equal(transport.streams.length, 1, 'one unreadable frame tore the connection down');
});

test('every record is offered to the application before it is acted on', async () => {
  const transport = new FakeTransport().streamSessions({
    events: [frame('1', { entity: 'work_item', entity_id: ITEM })],
    open: true,
  });
  const engine = new SyncEngine({ transport });
  const pause = pauses(1);
  const records: ChangeRecord[] = [];

  pause.hold(engine.listen({ pathsFor, wait: pause.wait, onRecord: (record) => records.push(record) }));
  await settle();
  pause.end();

  assert.equal(records.length, 1);
  assert.equal(records[0]?.entity_id, ITEM);
});

test('stopping the listener ends the connection and keeps what was read', async () => {
  const transport = new FakeTransport()
    .answer(`/items/${ITEM}`, { id: ITEM })
    .streamSessions({ open: true });
  const engine = new SyncEngine({ transport });

  await engine.refresh({ path: `/items/${ITEM}` });
  const stop = engine.listen({ pathsFor, wait: async () => {} });
  await settle();
  stop();
  await settle();

  assert.equal(transport.streams.length, 1, 'the loop reopened a connection after it was stopped');
  // Stopping the stream is not signing out: `reset` is what empties the engine.
  assert.equal(engine.peek({ path: `/items/${ITEM}` }).status, 'ready');
});
