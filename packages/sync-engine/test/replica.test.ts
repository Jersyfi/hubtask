// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The replica and the initial synchronisation (F6-03), headless: a scripted Transport, a memory
// store, and a clock that does not move. What is asserted is what the store holds afterwards,
// because that is what a screen reads while offline - and the points of offline-sync.md §9 the
// engine owes as named tests: 3 (revocation empties the subtree), 4 (a cursor too old resyncs),
// 6 (sign-out deletes the store; the browser has no encryption, and says so), 7 (a field the
// engine has never seen is kept unchanged).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine } from '../src/SyncEngine.ts';
import { TransportError } from '../src/errors.ts';
import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import { collectionOf, META } from '../src/replica.ts';
import type { ChangeRecord, StoredRecord } from '../src/schema.ts';
import type { StreamEvent } from '../src/ports.ts';
import { FakeTransport, FixedClock } from './fakes.ts';

const HUB = '0192f000-0000-7000-8000-0000000000a1';
const COLLECTION = '0192f000-0000-7000-8000-0000000000c1';
const OTHER_COLLECTION = '0192f000-0000-7000-8000-0000000000c2';
const ITEM = '0192f000-0000-7000-8000-000000000001';
const OTHER_ITEM = '0192f000-0000-7000-8000-000000000002';
const LABEL = '0192f000-0000-7000-8000-0000000000e1';
const COMMENT = '0192f000-0000-7000-8000-0000000000f1';

/** A whole object, as the initial synchronisation and a creation carry one. */
const whole = (entity: string, id: string, payload: Record<string, unknown>, container?: string): ChangeRecord =>
  ({ op: 'UPSERT', entity, entity_id: id, container_id: container ?? null, payload: { id, ...payload } }) as ChangeRecord;

/** The workspace the fakes hand out: a hub, two collections, two entries, a label, a comment. */
const WORKSPACE: readonly ChangeRecord[] = [
  whole('container', HUB, { type: 'HUB', name: 'Engines', parent_id: null, version: 1 }),
  whole('container', COLLECTION, { type: 'COLLECTION', name: 'Inbox', parent_id: HUB, version: 1 }, HUB),
  whole('container', OTHER_COLLECTION, { type: 'COLLECTION', name: 'Elsewhere', parent_id: null, version: 1 }),
  whole('label', LABEL, { name: 'urgent', collection_id: COLLECTION }, COLLECTION),
  whole('item', ITEM, { title: 'Buy milk', collection_id: COLLECTION, version: 3, a_field_of_2029: { nested: true } }, COLLECTION),
  whole('item', OTHER_ITEM, { title: 'Elsewhere', collection_id: OTHER_COLLECTION, version: 1 }, OTHER_COLLECTION),
  { op: 'UPSERT', entity: 'item', entity_id: ITEM, container_id: COLLECTION, payload: { set: 'labels', element_id: LABEL, op: 'add' } } as ChangeRecord,
  whole('comment', COMMENT, { item_id: ITEM, body: 'soon' }, COLLECTION),
];

const pathsFor = (record: ChangeRecord) => (record.entity === 'item' ? [`/items/${record.entity_id}`] : []);

function frame(id: string, record: ChangeRecord): StreamEvent {
  return { id, event: record.entity, data: JSON.stringify(record) };
}

async function settle(turns = 12): Promise<void> {
  for (let i = 0; i < turns; i += 1) await new Promise((resolve) => setTimeout(resolve, 0));
}

/** An engine with a store attached and a device minted, over the fakes. */
async function attached(transport: FakeTransport, storage = new MemoryStorage()) {
  const clock = new FixedClock();
  const engine = new SyncEngine({ transport, clock });
  const device = await engine.attach(storage, { platform: 'web', displayName: 'Firefox on Linux' });
  return { engine, storage, device, clock };
}

const held = (storage: MemoryStorage, entity: string, id: string) =>
  storage.get<StoredRecord<Record<string, unknown>>>(collectionOf(entity), id);

test('a fresh sign-in takes the snapshot and holds every record of it; the cursor is the store\'s', async () => {
  const transport = new FakeTransport().snapshotSessions({ records: WORKSPACE, cursor: 'c-100' }).streamSessions({ open: true });
  transport.answer('/sync:pull', { changes: [], cursor: 'c-100', has_more: false, tombstone_window_days: 90, server_time: '2026-09-17T10:00:00Z' });
  const { engine, storage, device } = await attached(transport);

  const stop = engine.listen({ pathsFor });
  await settle();
  stop();

  assert.equal(transport.snapshots.length, 1, 'one snapshot');
  assert.deepEqual(transport.snapshots[0]?.body, { device_id: device.id, platform: 'web', display_name: 'Firefox on Linux' });
  assert.ok(/^[0-9a-f-]{36}$/.test(device.id) && device.id[14] === '7', 'the device is a UUIDv7');

  const item = await held(storage, 'item', ITEM);
  assert.equal(item?.document.title, 'Buy milk');
  assert.equal(item?.version, 3);
  assert.deepEqual(item?.sets, { labels: [LABEL] }, 'the set element landed beside the entry');
  assert.equal((await held(storage, 'container', HUB))?.document.name, 'Engines');
  assert.equal((await held(storage, 'comment', COMMENT))?.document.body, 'soon');
  assert.equal((await storage.get<{ cursor: string; tombstoneWindowDays: number }>(META, 'position'))?.cursor, 'c-100');
  assert.equal((await storage.get<{ tombstoneWindowDays: number }>(META, 'position'))?.tombstoneWindowDays, 90, 'the window the pull named is kept beside the cursor');
  assert.equal(transport.streams[0]?.lastEventId, 'c-100', 'the stream resumed from the snapshot\'s cursor');
});

test('§9.7: a field the engine has never seen is stored and read back unchanged', async () => {
  const transport = new FakeTransport().snapshotSessions({ records: WORKSPACE, cursor: 'c-1' }).streamSessions({ open: true });
  transport.answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false });
  const { engine, storage } = await attached(transport);
  const stop = engine.listen({ pathsFor });
  await settle();
  stop();
  assert.deepEqual((await held(storage, 'item', ITEM))?.document.a_field_of_2029, { nested: true });
  // And an entity this build has never heard of is held under its own name, whole.
  const transport2 = new FakeTransport()
    .snapshotSessions({ records: [whole('something_newer', OTHER_ITEM, { shape: [1, 2, 3] })], cursor: 'c-2' })
    .streamSessions({ open: true });
  transport2.answer('/sync:pull', { changes: [], cursor: 'c-2', has_more: false });
  const second = await attached(transport2);
  const stop2 = second.engine.listen({ pathsFor });
  await settle();
  stop2();
  assert.deepEqual((await second.storage.get<StoredRecord<unknown>>('something_newer', OTHER_ITEM))?.document, { id: OTHER_ITEM, shape: [1, 2, 3] });
});

test('a second start takes the delta from the held cursor and no snapshot', async () => {
  const storage = new MemoryStorage();
  const first = new FakeTransport().snapshotSessions({ records: WORKSPACE, cursor: 'c-100' }).streamSessions({ open: true });
  first.answer('/sync:pull', { changes: [], cursor: 'c-100', has_more: false });
  const a = await attached(first, storage);
  const stopA = a.engine.listen({ pathsFor });
  await settle();
  stopA();

  // The tab reloads: a new engine over the same store.
  const second = new FakeTransport().streamSessions({ open: true }).answerEach('/sync:pull', [
    { changes: [{ op: 'UPSERT', entity: 'item', entity_id: ITEM, container_id: COLLECTION, payload: { title: 'Buy oat milk' } }], cursor: 'c-101', has_more: true },
    { changes: [{ op: 'DELETE', entity: 'item', entity_id: OTHER_ITEM, container_id: OTHER_COLLECTION }], cursor: 'c-102', has_more: false },
  ]);
  const b = await attached(second, storage);
  assert.equal(b.device.id, a.device.id, 'the device survives a reload');
  const stopB = b.engine.listen({ pathsFor });
  await settle();
  stopB();

  assert.equal(second.snapshots.length, 0, 'no snapshot on a second start');
  const pulls = second.calls.filter((c) => c.path === '/sync:pull');
  assert.equal(pulls.length, 2, 'the delta was walked until has_more was false');
  assert.equal((pulls[0]?.body as { cursor: string }).cursor, 'c-100');
  assert.equal((pulls[1]?.body as { cursor: string }).cursor, 'c-101');
  const item = await held(storage, 'item', ITEM);
  assert.equal(item?.document.title, 'Buy oat milk', 'a field record updated that field');
  assert.deepEqual(item?.sets, { labels: [LABEL] }, 'and left the sets alone');
  assert.equal(item?.document.collection_id, COLLECTION, 'and the rest of the document');
  assert.equal(await held(storage, 'item', OTHER_ITEM), undefined, 'a DELETE removed the entry');
  assert.equal(second.streams[0]?.lastEventId, 'c-102', 'the stream resumes from where the delta ended');
});

test('a snapshot ending twice without its cursor is followed by the page walk', async () => {
  const transport = new FakeTransport()
    .snapshotSessions({ records: WORKSPACE.slice(0, 2) }, { records: WORKSPACE.slice(0, 3) })
    .streamSessions({ open: true })
    .answerEach('/sync:pull', [
      { changes: WORKSPACE.slice(0, 4), cursor: 'walk-1', has_more: true },
      { changes: WORKSPACE.slice(4), cursor: 'c-200', has_more: false },
      { changes: [], cursor: 'c-200', has_more: false },
    ]);
  const { engine, storage } = await attached(transport);
  const stop = engine.listen({ pathsFor });
  await settle(20);
  stop();

  assert.equal(transport.snapshots.length, 2, 'two attempts, then the pages');
  const pulls = transport.calls.filter((c) => c.path === '/sync:pull').map((c) => (c.body as { cursor: unknown }).cursor);
  assert.deepEqual(pulls.slice(0, 2), [null, 'walk-1'], 'the walk starts from nothing and continues page by page');
  assert.equal((await held(storage, 'item', ITEM))?.document.title, 'Buy milk');
  assert.equal((await storage.get<{ cursor: string }>(META, 'position'))?.cursor, 'c-200', 'the last page\'s cursor');
});

test('the stream applies each record before it invalidates, and the cursor advances in the store', async () => {
  const transport = new FakeTransport()
    .snapshotSessions({ records: WORKSPACE, cursor: 'c-1' })
    .answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false })
    .answer(`/items/${ITEM}`, { id: ITEM, title: 'Buy milk' })
    .streamSessions({
      events: [
        frame('c-2', { op: 'UPSERT', entity: 'item', entity_id: ITEM, container_id: COLLECTION, payload: { title: 'Buy milk, and eggs' } } as ChangeRecord),
        frame('c-3', { op: 'UPSERT', entity: 'item', entity_id: ITEM, container_id: COLLECTION, payload: { set: 'labels', element_id: LABEL, op: 'remove' } } as ChangeRecord),
        frame('c-4', { op: 'DELETE', entity: 'container', entity_id: COLLECTION, container_id: HUB } as ChangeRecord),
      ],
      open: true,
    });
  const { engine, storage } = await attached(transport);
  const seen: string[] = [];
  engine.subscribe({ path: `/items/${ITEM}` }, (state) => seen.push(state.status));
  await settle();

  const stop = engine.listen({ pathsFor });
  await settle(20);
  stop();

  assert.equal(await held(storage, 'item', ITEM), undefined, 'the container deletion took the entry under it');
  assert.equal(await held(storage, 'comment', COMMENT), undefined, 'and the comment on the entry');
  assert.equal(await held(storage, 'label', LABEL), undefined, 'and the label of the collection');
  assert.equal(await held(storage, 'container', COLLECTION), undefined);
  assert.notEqual(await held(storage, 'container', HUB), undefined, 'the hub above stays');
  assert.notEqual(await held(storage, 'item', OTHER_ITEM), undefined, 'and the other collection\'s entry');
  assert.equal((await storage.get<{ cursor: string }>(META, 'position'))?.cursor, 'c-4', 'the cursor advanced in the store on the frame');
  assert.ok(seen.includes('ready'), 'the watched entry was re-read after the record');
});

test('§9.3: an ACCESS_REVOKED for a hub leaves nothing of the hub in the store', async () => {
  const transport = new FakeTransport()
    .snapshotSessions({ records: WORKSPACE, cursor: 'c-1' })
    .answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false })
    .streamSessions({
      events: [frame('c-2', { op: 'ACCESS_REVOKED', entity: 'container', entity_id: HUB, container_id: HUB } as ChangeRecord)],
      open: true,
    });
  const { engine, storage } = await attached(transport);
  const stop = engine.listen({ pathsFor });
  await settle(20);
  stop();

  for (const [entity, id] of [['container', HUB], ['container', COLLECTION], ['item', ITEM], ['label', LABEL], ['comment', COMMENT]] as const) {
    assert.equal(await held(storage, entity, id), undefined, `${entity} ${id} survived the revocation`);
  }
  assert.notEqual(await held(storage, 'container', OTHER_COLLECTION), undefined, 'a collection outside the hub stays');
  assert.notEqual(await held(storage, 'item', OTHER_ITEM), undefined);
});

test('§9.4: sync.cursor_too_old empties the store and resynchronises; the device stays', async () => {
  const transport = new FakeTransport()
    .snapshotSessions({ records: WORKSPACE.slice(0, 5), cursor: 'c-old' }, { records: WORKSPACE, cursor: 'c-new' })
    .streamSessions(
      { refuse: new TransportError('problem', { status: 410, code: 'conflict', detailCode: 'sync.cursor_too_old' }) },
      { open: true },
    );
  // The delta after each snapshot answers the cursor it was asked from: nothing moved.
  transport.answerEach('/sync:pull', [
    { changes: [], cursor: 'c-old', has_more: false },
    { changes: [], cursor: 'c-new', has_more: false },
  ]);
  const { engine, storage, device } = await attached(transport);
  const stop = engine.listen({ pathsFor });
  await settle(30);
  stop();

  assert.equal(transport.snapshots.length, 2, 'the refusal led to a second initial synchronisation');
  assert.equal((await storage.get<{ cursor: string }>(META, 'position'))?.cursor, 'c-new');
  assert.equal((await held(storage, 'comment', COMMENT))?.document.body, 'soon', 'the second snapshot filled the store again');
  assert.equal((await storage.get<{ id: string }>(META, 'device'))?.id, device.id, 'the device is the same device');
  assert.deepEqual(transport.streams.map((s) => s.lastEventId), ['c-old', 'c-new']);
});

test('sync.cursor_invalid restarts with no cursor and keeps the replica', async () => {
  const transport = new FakeTransport()
    .snapshotSessions({ records: WORKSPACE, cursor: 'c-1' }, { records: [], cursor: 'c-again' })
    .streamSessions(
      { refuse: new TransportError('problem', { status: 400, code: 'validation', detailCode: 'sync.cursor_invalid' }) },
      { open: true },
    );
  transport.answerEach('/sync:pull', [
    { changes: [], cursor: 'c-1', has_more: false },
    { changes: [], cursor: 'c-again', has_more: false },
  ]);
  const { engine, storage } = await attached(transport);
  const stop = engine.listen({ pathsFor });
  await settle(30);
  stop();

  assert.equal((await held(storage, 'item', ITEM))?.document.title, 'Buy milk', 'nothing held was dropped');
  assert.deepEqual(transport.streams.map((s) => s.lastEventId), ['c-1', 'c-again']);
});

test('§9.6: sign-out deletes the store - the copy, the cursor and the device go together', async () => {
  const transport = new FakeTransport().snapshotSessions({ records: WORKSPACE, cursor: 'c-1' }).streamSessions({ open: true });
  transport.answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false });
  const { engine, storage } = await attached(transport);
  const stop = engine.listen({ pathsFor });
  await settle();
  stop();
  assert.ok((await storage.collections()).length > 0);

  await engine.reset();
  assert.deepEqual(await storage.collections(), []);
  assert.equal(await storage.get(META, 'device'), undefined);
  assert.equal(engine.device, undefined);
  assert.equal(engine.storage, undefined);
  // The browser has no encryption at rest, by decision (ADR-0033 §4): what §9.6 asks of it is
  // exactly this deletion. Reported rather than tested, because there is nothing to test.
});

test('without a store the engine is what it was: online-only, the cursor in memory', async () => {
  const transport = new FakeTransport().streamSessions({ events: [frame('7', whole('item', ITEM, { title: 'x' }))], open: true });
  const engine = new SyncEngine({ transport });
  const stop = engine.listen({ pathsFor });
  await settle();
  stop();
  assert.equal(transport.snapshots.length, 0);
  assert.equal(transport.calls.filter((c) => c.path === '/sync:pull').length, 0);
  assert.equal(engine.storage, undefined);
});

test('a stamp is the device\'s clock, and survives a reload through the store', async () => {
  const storage = new MemoryStorage();
  const transport = new FakeTransport();
  const a = await attached(transport, storage);
  const first = await a.engine.stamp();
  assert.match(first ?? '', /^\d{13}:00000:[0-9a-f-]{36}$/);
  const second = await a.engine.stamp();
  assert.ok((second ?? '') > (first ?? ''), 'the clock moved');
  const b = await attached(new FakeTransport(), storage);
  const third = await b.engine.stamp();
  assert.ok((third ?? '') > (second ?? ''), 'a new engine over the same store continues, never stamps backwards');
});
