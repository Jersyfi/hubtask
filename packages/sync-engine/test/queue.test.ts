// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The queue and the push (F6-05), headless: a scripted transport whose `/sync:push` applies
// what it is sent the way the server does - once per op_id - and a memory store. The points of
// offline-sync.md §9 the engine owes as named tests: 1 (a created entry's identity is the
// client's and final), 2 (the same push twice applies once), 5 (the server's answer overwrites
// the prediction; a rejection is shown, not swallowed).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine } from '../src/SyncEngine.ts';
import { TransportError } from '../src/errors.ts';
import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import { QUEUE, REJECTED } from '../src/queue.ts';
import type { ChangeRecord, QueuedWrite, StoredRecord, SyncMutation, SyncMutationResult } from '../src/schema.ts';
import type { RequestOptions, Response } from '../src/ports.ts';
import { FakeTransport, FixedClock } from './fakes.ts';

const COLLECTION = '0192f000-0000-7000-8000-0000000000c1';
const ITEM = '0192f000-0000-7000-8000-000000000001';
const LABEL = '0192f000-0000-7000-8000-0000000000e1';

const WORKSPACE: readonly ChangeRecord[] = [
  { op: 'UPSERT', entity: 'container', entity_id: COLLECTION, container_id: null, payload: { id: COLLECTION, type: 'COLLECTION', name: 'Inbox', parent_id: null, version: 1 } } as ChangeRecord,
  { op: 'UPSERT', entity: 'item', entity_id: ITEM, container_id: COLLECTION, payload: { id: ITEM, title: 'Buy milk', collection_id: COLLECTION, version: 3, order_key: 'a0' } } as ChangeRecord,
];

const pathsFor = (record: ChangeRecord) => (record.entity === 'item' ? [`/items/${record.entity_id}`, '/items'] : []);

/** The application's `mutationFor`, as small as the test needs: a patch, a create, a set, a delete. */
const mutationFor = async (method: string, path: string, body: unknown, helpers: { mintId: () => string }): Promise<QueuedWrite | undefined> => {
  if (method === 'POST' && path === '/items') {
    return { kind: 'ITEM_CREATE', itemId: helpers.mintId(), payload: body as Record<string, unknown> };
  }
  let match = /^\/items\/([^/]+)$/.exec(path);
  if (match?.[1] && method === 'PATCH') return { kind: 'ITEM_PATCH', itemId: match[1], fields: body as Record<string, unknown> };
  if (match?.[1] && method === 'DELETE') return { kind: 'ITEM_DELETE', itemId: match[1] };
  match = /^\/items\/([^/]+)\/labels\/([^/]+)$/.exec(path);
  if (match?.[1] && match[2]) return { kind: method === 'PUT' ? 'SET_ADD' : 'SET_REMOVE', itemId: match[1], set: 'labels', element: match[2] };
  return undefined;
};

/**
 * A transport whose `/sync:push` is the server's: it applies each mutation once per op_id and
 * answers a result per mutation, and it can be told to go away, or to drop the answer to the
 * next push after applying it - the network failing on the way back.
 */
class PushingTransport extends FakeTransport {
  readonly applied = new Map<string, SyncMutation>();
  readonly pushes: unknown[] = [];
  down = false;
  dropNextAnswer = false;
  results: (mutation: SyncMutation) => SyncMutationResult = (mutation) => ({
    op_id: mutation.op_id, result: 'APPLIED', entity_id: mutation.item_id,
    server_state: { id: mutation.item_id, title: 'from the server', version: 4 },
  });

  override async send<T>(method: 'POST' | 'PATCH' | 'PUT' | 'DELETE', path: string, body: unknown, options: RequestOptions): Promise<Response<T>> {
    if (this.down) throw new TransportError('offline');
    if (path !== '/sync:push') return super.send<T>(method, path, body, options);
    this.pushes.push(body);
    await Promise.resolve();
    const request = body as { mutations: SyncMutation[] };
    const results = request.mutations.map((mutation) => {
      const seen = this.applied.get(String(mutation.op_id));
      if (!seen) this.applied.set(String(mutation.op_id), mutation);
      return this.results(mutation);
    });
    if (this.dropNextAnswer) {
      this.dropNextAnswer = false;
      throw new TransportError('offline');
    }
    return { status: 200, body: { results, cursor: 'c-after-push' } as T };
  }
}

async function settle(turns = 12): Promise<void> {
  for (let i = 0; i < turns; i += 1) await new Promise((resolve) => setTimeout(resolve, 0));
}

async function synced(storage = new MemoryStorage(), clock = new FixedClock(1_700_000_000_000)) {
  const transport = new PushingTransport().snapshotSessions({ records: WORKSPACE, cursor: 'c-1' }).streamSessions({ open: true });
  transport.answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false });
  transport.answer(`/items/${ITEM}`, { id: ITEM, title: 'Buy milk', version: 3 });
  const engine = new SyncEngine({ transport, clock, mutationFor });
  await engine.attach(storage, { platform: 'web', displayName: 'test' });
  const stop = engine.listen({ pathsFor });
  await settle();
  stop();
  return { transport, engine, storage, clock };
}

const item = (storage: MemoryStorage) => storage.get<StoredRecord<Record<string, unknown>>>('items', ITEM);

test('online with an empty queue, a write goes straight to the server', async () => {
  const { transport, engine, storage } = await synced();
  transport.answer(`/items/${ITEM}`, { id: ITEM, title: 'Renamed', version: 4 });
  const answer = await engine.mutate<{ title: string }>('PATCH', `/items/${ITEM}`, { title: 'Renamed' }, { invalidates: ['/items'] });
  assert.equal(answer.title, 'Renamed');
  assert.equal(transport.pushes.length, 0, 'nothing was pushed');
  assert.equal((await storage.all(QUEUE)).length, 0);
  assert.equal(engine.queueState.count, 0);
});

test('§9.5: a write while the transport is down is queued, shown pending, pushed on reconnect and overwritten by the server', async () => {
  const { transport, engine, storage } = await synced();
  transport.down = true;

  const seen: string[] = [];
  engine.queue((state) => seen.push(`${state.count}:${state.pushing}`));
  const prediction = await engine.mutate<{ title: string }>('PATCH', `/items/${ITEM}`, { title: 'Buy oat milk' }, { invalidates: ['/items'] });
  assert.equal(prediction.title, 'Buy oat milk', 'the write answers its prediction');

  const held = await item(storage);
  assert.equal(held?.document.title, 'Buy oat milk', 'the prediction is in the replica');
  assert.equal(held?.pending, true, 'and marked pending');
  assert.equal(engine.queueState.count, 1);
  assert.equal(engine.queueState.oldestAt, 1_700_000_000_000);
  const queued = await storage.all<{ mutation: SyncMutation }>(QUEUE);
  assert.equal(queued[0]?.mutation.kind, 'ITEM_PATCH');
  assert.match(String((queued[0]?.mutation.fields as Record<string, { hlc: string }>)?.title?.hlc), /^\d{13}:\d{5}:/);

  // The server comes back: the next turn of the loop pushes, and the answer wins.
  transport.down = false;
  const stop = engine.listen({ pathsFor, wait: async () => {} });
  await settle(20);
  stop();
  assert.equal(transport.pushes.length, 1);
  const pushed = transport.pushes[0] as { device_id: string; platform: string; display_name: string; mutations: SyncMutation[] };
  assert.equal(pushed.device_id, engine.device?.id, 'the frame carries the device');
  assert.equal(pushed.platform, 'web');
  assert.equal(pushed.display_name, 'test');
  const after = await item(storage);
  assert.equal(after?.document.title, 'from the server', 'server_state overwrote the prediction');
  assert.equal(after?.pending, undefined);
  assert.equal(engine.queueState.count, 0);
  assert.ok(seen.includes('1:false') && seen.includes('0:false'), `the queue's subscribers saw ${seen.join(' ')}`);
  assert.equal((await storage.get<{ cursor: string }>('meta', 'position'))?.cursor, 'c-after-push', 'the push\'s cursor advanced the store\'s');
});

test('§9.2: the same push sent twice - the transport drops the first answer - applies once', async () => {
  const { transport, engine, storage } = await synced();
  transport.down = true;
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'once' });
  transport.down = false;
  transport.dropNextAnswer = true;

  await engine.push();
  assert.equal(transport.pushes.length, 1);
  assert.equal(engine.queueState.count, 1, 'the answer never arrived: the mutation stays queued');
  assert.equal((await storage.all<{ attempts: number }>(QUEUE))[0]?.attempts, 1, 'one attempt older');

  await engine.push();
  assert.equal(transport.pushes.length, 2);
  const [first, second] = transport.pushes as { mutations: SyncMutation[] }[];
  assert.equal(first?.mutations[0]?.op_id, second?.mutations[0]?.op_id, 'the same op_id both times');
  assert.equal(transport.applied.size, 1, 'the server applied it once');
  assert.equal(engine.queueState.count, 0);
});

test('§9.1: a created entry keeps the identity the client minted, and the queue survives a reload', async () => {
  const storage = new MemoryStorage();
  const { transport, engine } = await synced(storage);
  transport.down = true;
  const created = await engine.mutate<{ id: string; title: string; version: number }>('POST', '/items', { type: 'TASK', collection_id: COLLECTION, title: 'Made offline' });
  assert.ok(created.id[14] === '7', `a UUIDv7: ${created.id}`);
  assert.equal(created.version, 0);
  assert.equal((await storage.get<StoredRecord<unknown>>('items', created.id))?.pending, true);

  // A second write behind the first, and the tab reloads: a new engine over the same store.
  await engine.mutate('PUT', `/items/${created.id}/labels/${LABEL}`, undefined);
  const again = new PushingTransport();
  again.answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false });
  again.results = (mutation) => ({ op_id: mutation.op_id, result: 'APPLIED', entity_id: mutation.item_id, server_state: { id: mutation.item_id, title: 'Made offline', version: 1 } });
  const reloaded = new SyncEngine({ transport: again, clock: new FixedClock(1_700_000_001_000), mutationFor });
  await reloaded.attach(storage, { platform: 'web', displayName: 'test' });
  assert.equal(reloaded.queueState.count, 2, 'the queue was in the store');

  await reloaded.push();
  const pushed = again.pushes[0] as { mutations: SyncMutation[] };
  assert.deepEqual(pushed.mutations.map((m) => m.kind), ['ITEM_CREATE', 'SET_ADD'], 'two writes queued in order arrive in order');
  assert.equal(pushed.mutations[0]?.item_id, created.id, 'the identity is final');
  assert.equal(pushed.mutations[1]?.set, 'labels');
  assert.equal(reloaded.queueState.count, 0);
  assert.equal((await storage.get<StoredRecord<{ title: string }>>('items', created.id))?.document.title, 'Made offline');
});

test('§9.5: a REJECTED result is kept with its code and removed only by dismissal', async () => {
  const { transport, engine, storage } = await synced();
  transport.results = (mutation) => ({ op_id: mutation.op_id, result: 'REJECTED', entity_id: mutation.item_id, error: { code: 'forbidden', message_code: 'errors.forbidden' } });
  transport.down = true;
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'not allowed' });
  transport.down = false;
  await engine.push();

  assert.equal(engine.queueState.count, 0, 'out of the queue');
  assert.equal(engine.queueState.rejected.length, 1, 'and kept');
  const rejected = engine.queueState.rejected[0];
  assert.equal(rejected?.code, 'forbidden');
  assert.equal(rejected?.messageCode, 'errors.forbidden');
  assert.deepEqual(rejected?.local, { title: 'not allowed' }, 'with what the person wrote');
  assert.equal((await item(storage))?.pending, undefined, 'the prediction is no longer pending');

  await engine.push();
  assert.equal(engine.queueState.rejected.length, 1, 'a push does not clear it');
  await engine.dismissRejected(rejected?.id ?? '');
  assert.equal(engine.queueState.rejected.length, 0);
  assert.equal((await storage.all(REJECTED)).length, 0);
});

test('§9.3, §7: a sync.gone is discarded, the copy goes, and the local text is offered back', async () => {
  const { transport, engine, storage } = await synced();
  transport.results = (mutation) => ({ op_id: mutation.op_id, result: 'REJECTED', entity_id: mutation.item_id, error: { code: 'sync.gone' } });
  transport.down = true;
  await engine.mutate('PATCH', `/items/${ITEM}`, { notes: 'written to a purged entry' });
  transport.down = false;
  await engine.push();

  assert.equal(await item(storage), undefined, 'the copy of a purged entry goes');
  assert.equal(engine.queueState.rejected[0]?.code, 'sync.gone');
  assert.deepEqual(engine.queueState.rejected[0]?.local, { notes: 'written to a purged entry' });
});

test('a CONFLICT writes the server\'s state and keeps both values', async () => {
  const { transport, engine, storage } = await synced();
  transport.results = (mutation) => ({
    op_id: mutation.op_id, result: 'CONFLICT', entity_id: mutation.item_id,
    server_state: { id: mutation.item_id, title: 'theirs', version: 5 },
    conflict: { field: 'title', mine: 'mine', theirs: 'theirs', preserved_comment_id: '0192f000-0000-7000-8000-0000000000f9' },
  });
  transport.down = true;
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'mine' });
  transport.down = false;
  await engine.push();
  assert.equal((await item(storage))?.document.title, 'theirs');
  assert.equal(engine.queueState.conflicts.length, 1);
  assert.deepEqual(engine.queueState.conflicts[0]?.mine, 'mine');
  assert.equal(engine.queueState.conflicts[0]?.preservedCommentId, '0192f000-0000-7000-8000-0000000000f9');
  await engine.dismissConflict(engine.queueState.conflicts[0]?.id ?? '');
  assert.equal(engine.queueState.conflicts.length, 0);
});

test('a write behind a queued one is queued too, and pushed at once while the server answers', async () => {
  const { transport, engine } = await synced();
  transport.down = true;
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'first' });
  transport.down = false;
  // The server answers again, but the queue is not empty: the next write joins the queue and
  // the push runs at once, in order.
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'second' });
  await settle();
  await engine.push();
  const kinds = (transport.pushes as { mutations: SyncMutation[] }[]).flatMap((p) => p.mutations.map((m) => (m.fields as Record<string, { value: string }>)?.title?.value));
  assert.deepEqual(kinds, ['first', 'second']);
  assert.equal(engine.queueState.count, 0);
});

test('a write the application cannot queue fails in front of the person, as before', async () => {
  const { transport, engine } = await synced();
  transport.down = true;
  await assert.rejects(
    () => engine.mutate('POST', `/items/${ITEM}:archive`, undefined),
    (error: unknown) => error instanceof TransportError && error.kind === 'offline',
  );
  assert.equal(engine.queueState.count, 0);
});

test('the HLC of a queued field never moves backwards under a clock that does', async () => {
  const clock = new FixedClock(1_700_000_000_000);
  const { transport, engine, storage } = await synced(new MemoryStorage(), clock);
  transport.down = true;
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'a' });
  clock.advance(-60_000);
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'b' });
  const [first, second] = [...(await storage.all<{ seq: number; mutation: SyncMutation }>(QUEUE))].sort((x, y) => x.seq - y.seq);
  const hlc = (m?: { mutation: SyncMutation }) => String((m?.mutation.fields as Record<string, { hlc: string }>)?.title?.hlc);
  assert.ok(hlc(second) > hlc(first), `${hlc(second)} is not after ${hlc(first)}`);
});

test('sync.device_revoked on a push clears the store and forgets the device', async () => {
  const { transport, engine, storage } = await synced();
  transport.down = true;
  await engine.mutate('PATCH', `/items/${ITEM}`, { title: 'x' });
  transport.down = false;
  const before = engine.device?.id;
  transport.send = async () => { throw new TransportError('problem', { status: 403, code: 'forbidden', detailCode: 'sync.device_revoked' }); };
  await engine.push();
  assert.equal(await item(storage), undefined, 'the store is emptied');
  assert.equal(await storage.get('meta', 'device'), undefined, 'the device is forgotten');
  assert.equal(engine.device, undefined);
  const again = await engine.attach(storage, { platform: 'web', displayName: 'test' });
  assert.notEqual(again.id, before, 'a new device is minted');
});

test('forgetting another device refuses its next push, and that device starts over (F6-07, N-03)', async () => {
  // One fake server, two devices of one account: A and B, each with a store of its own.
  const forgotten = new Set<string>();
  class Server extends PushingTransport {
    override async send<T>(method: 'POST' | 'PATCH' | 'PUT' | 'DELETE', path: string, body: unknown, options: RequestOptions): Promise<Response<T>> {
      const match = /^\/sync\/devices\/([^/]+)$/.exec(path);
      if (method === 'DELETE' && match?.[1]) {
        forgotten.add(match[1]);
        return { status: 204, body: undefined as T };
      }
      if (path === '/sync:push' && forgotten.has((body as { device_id: string }).device_id)) {
        throw new TransportError('problem', { status: 403, code: 'forbidden', detailCode: 'sync.device_revoked' });
      }
      return super.send<T>(method, path, body, options);
    }
  }
  const server = new Server().snapshotSessions({ records: WORKSPACE, cursor: 'c-1' }).streamSessions({ open: true });
  server.answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false });
  server.answer('/sync/devices', []);

  const a = new SyncEngine({ transport: server, clock: new FixedClock(), mutationFor });
  await a.attach(new MemoryStorage(), { platform: 'web', displayName: 'A' });
  const b = new SyncEngine({ transport: server, clock: new FixedClock(), mutationFor });
  const storageB = new MemoryStorage();
  const deviceB = await b.attach(storageB, { platform: 'web', displayName: 'B' });
  const stopB = b.listen({ pathsFor });
  await settle();
  stopB();

  // B makes a change while away; meanwhile A forgets B from its device list.
  server.down = true;
  await b.mutate('PATCH', `/items/${ITEM}`, { title: 'from B' });
  server.down = false;
  await a.mutate('DELETE', `/sync/devices/${deviceB.id}`, undefined, { invalidates: ['/sync/devices'] });
  assert.ok(forgotten.has(deviceB.id));

  // B's next push is refused: its store is emptied, the device forgotten, and a fresh attach
  // mints a new identity the server has never seen.
  await b.push();
  assert.equal(b.queueState.count, 0);
  assert.equal(b.queueState.rejected[0]?.code, 'sync.device_revoked', 'what B had not sent is kept as refused, not lost');
  assert.deepEqual(b.queueState.rejected[0]?.local, { title: 'from B' });
  assert.equal(await storageB.get('meta', 'device'), undefined);
  const again = await b.attach(storageB, { platform: 'web', displayName: 'B' });
  assert.notEqual(again.id, deviceB.id);
  assert.ok(!forgotten.has(again.id));
});
