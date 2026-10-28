// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The kinds beyond the entry's (offline-sync.md §3.2, §4.2) and a file captured offline (§1,
// the installed apps only), headless against the memory store: each kind is queued with its
// prediction and pushed in the order it was made; a captured file waits in the store, is
// uploaded and attached when the server answers, before what was made after it; a refused upload
// shows on the change; and a copy that holds no files refuses the capture.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine } from '../src/SyncEngine.ts';
import { TransportError } from '../src/errors.ts';
import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import { FILES, QUEUE } from '../src/queue.ts';
import type { ChangeRecord, QueuedWrite, StoredRecord, SyncMutation, SyncMutationResult } from '../src/schema.ts';
import type { RequestOptions, Response } from '../src/ports.ts';
import { FakeTransport, FixedClock } from './fakes.ts';

const COLLECTION = '0192f000-0000-7000-8000-0000000000c1';
const ITEM = '0192f000-0000-7000-8000-000000000001';
const REMINDER = '0192f000-0000-7000-8000-0000000000a1';
const LABEL = '0192f000-0000-7000-8000-0000000000e1';
const MEDIA = '0192f000-0000-7000-8000-0000000000d1';

const WORKSPACE: readonly ChangeRecord[] = [
  { op: 'UPSERT', entity: 'container', entity_id: COLLECTION, container_id: null, payload: { id: COLLECTION, type: 'COLLECTION', name: 'Inbox', parent_id: null, version: 1 } } as ChangeRecord,
  { op: 'UPSERT', entity: 'item', entity_id: ITEM, container_id: COLLECTION, payload: { id: ITEM, title: 'Buy milk', collection_id: COLLECTION, version: 3, order_key: 'a0' } } as ChangeRecord,
  { op: 'UPSERT', entity: 'label', entity_id: LABEL, container_id: COLLECTION, payload: { id: LABEL, collection_id: COLLECTION, name: 'Errand', version: 1 } } as ChangeRecord,
];

/** The test's `mutationFor`: the write is handed over as the body of `POST /queue`. */
const mutationFor = async (method: string, path: string, body: unknown): Promise<QueuedWrite | undefined> =>
  method === 'POST' && path === '/queue' ? (body as QueuedWrite) : undefined;

/** `/sync:push` applies each mutation once and answers its object as the server holds it. */
class Server extends FakeTransport {
  readonly pushes: { mutations: SyncMutation[] }[] = [];
  readonly sent: string[] = [];
  down = false;
  refuseMedia: TransportError | undefined;

  override async send<T>(method: 'POST' | 'PATCH' | 'PUT' | 'DELETE', path: string, body: unknown, options: RequestOptions): Promise<Response<T>> {
    if (this.down) throw new TransportError('offline');
    this.sent.push(`${method} ${path}`);
    if (path === '/media') {
      if (this.refuseMedia) throw this.refuseMedia;
      return { status: 201, body: { id: MEDIA, upload: { url: 'https://store.example/put' } } as T };
    }
    if (path === `/media/${MEDIA}:confirm`) return { status: 200, body: { id: MEDIA, status: 'READY' } as T };
    if (path !== '/sync:push') return super.send<T>(method, path, body, options);
    const request = body as { mutations: SyncMutation[] };
    this.pushes.push(request);
    const results: SyncMutationResult[] = request.mutations.map((m) => {
      const id = (m.entity_id ?? m.item_id ?? undefined) as string | undefined;
      const deletion = String(m.kind).endsWith('_DELETE');
      return {
        op_id: m.op_id, result: 'APPLIED', entity_id: id ?? null,
        server_state: deletion || m.kind === 'TEMPLATE_APPLY' ? (m.kind === 'TEMPLATE_APPLY' ? { root_item_id: ITEM, created: 2 } : null)
          : { id, from: 'server', version: 2 },
      } as SyncMutationResult;
    });
    return { status: 200, body: { results, cursor: 'c-after-push' } as T };
  }
}

async function settle(turns = 12): Promise<void> {
  for (let i = 0; i < turns; i += 1) await new Promise((resolve) => setTimeout(resolve, 0));
}

async function synced(holdsFiles = true) {
  const storage = new MemoryStorage();
  const transport = new Server().snapshotSessions({ records: WORKSPACE, cursor: 'c-1' }).streamSessions({ open: true });
  transport.answer('/sync:pull', { changes: [], cursor: 'c-1', has_more: false });
  const engine = new SyncEngine({ transport, clock: new FixedClock(1_700_000_000_000), mutationFor, holdsFiles });
  await engine.attach(storage, { platform: 'linux', displayName: 'test' });
  const stop = engine.listen({ pathsFor: () => [] });
  await settle();
  stop();
  transport.down = true;
  return { transport, engine, storage };
}

const queue = (engine: SyncEngine, write: QueuedWrite) => engine.mutate('POST', '/queue', write);

test('each kind beyond the entry is queued with its prediction and pushed in the order it was made', async () => {
  const { transport, engine, storage } = await synced();
  const writes: QueuedWrite[] = [
    { kind: 'REMINDER_CREATE', itemId: ITEM, entityId: REMINDER, payload: { offset_spec: 'REL:-PT1H' } },
    { kind: 'REMINDER_PATCH', itemId: ITEM, entityId: REMINDER, fields: { offset_spec: 'REL:-PT2H' } },
    { kind: 'RECURRENCE_CREATE', itemId: ITEM, entityId: '0192f000-0000-7000-8000-0000000000b1', payload: { rrule: 'FREQ=WEEKLY' } },
    { kind: 'RECURRENCE_PATCH', itemId: ITEM, entityId: '0192f000-0000-7000-8000-0000000000b1', fields: { time_zone: 'UTC' } },
    { kind: 'TEMPLATE_APPLY', itemId: '', payload: { template_id: 't', collection_id: COLLECTION } },
    { kind: 'CONTAINER_CREATE', itemId: '', entityId: '0192f000-0000-7000-8000-0000000000c9', payload: { type: 'HUB', name: 'Allotment' } },
    { kind: 'CONTAINER_PATCH', itemId: '', entityId: '0192f000-0000-7000-8000-0000000000c9', fields: { name: 'Garden' } },
    { kind: 'LABEL_PATCH', itemId: '', entityId: LABEL, containerId: COLLECTION, fields: { name: 'Errands' } },
    { kind: 'BUCKET_CREATE', itemId: '', entityId: '0192f000-0000-7000-8000-0000000000f1', payload: { collection_id: COLLECTION, name: 'Doing' } },
    { kind: 'BUCKET_PATCH', itemId: '', entityId: '0192f000-0000-7000-8000-0000000000f1', containerId: COLLECTION, fields: { wip_limit: 3 } },
    { kind: 'REMINDER_DELETE', itemId: ITEM, entityId: REMINDER },
    { kind: 'RECURRENCE_DELETE', itemId: ITEM, entityId: '0192f000-0000-7000-8000-0000000000b1' },
    { kind: 'BUCKET_DELETE', itemId: '', entityId: '0192f000-0000-7000-8000-0000000000f1', containerId: COLLECTION },
    { kind: 'LABEL_DELETE', itemId: '', entityId: LABEL, containerId: COLLECTION },
    { kind: 'CONTAINER_DELETE', itemId: '', entityId: '0192f000-0000-7000-8000-0000000000c9' },
  ];

  const reminder = await queue(engine, writes[0]!) as { id: string; offset_spec: string };
  assert.equal(reminder.id, REMINDER, 'the creation answers its prediction under the identifier minted');
  assert.equal(reminder.offset_spec, 'REL:-PT1H');
  for (const write of writes.slice(1, 8)) await queue(engine, write);
  const label = await storage.get<StoredRecord<Record<string, unknown>>>('labels', LABEL);
  assert.equal(label?.document.name, 'Errands', 'a patch predicts its fields');
  assert.equal(label?.pending, true);
  assert.equal((await storage.get<StoredRecord<Record<string, unknown>>>('reminder', REMINDER))?.document.offset_spec, 'REL:-PT2H');
  for (const write of writes.slice(8)) await queue(engine, write);
  assert.equal(await storage.get('reminder', REMINDER), undefined, 'a deletion predicts the object gone');
  assert.equal(engine.queueState.count, writes.length);

  transport.down = false;
  await engine.push();
  assert.equal(transport.pushes.length, 1);
  const pushed = transport.pushes[0]!.mutations;
  assert.deepEqual(pushed.map((m) => m.kind), writes.map((w) => w.kind), 'pushed in the order made');
  const patch = pushed[1]!;
  assert.equal(patch.entity_id, REMINDER);
  assert.equal(patch.item_id, ITEM);
  assert.match(String((patch.fields as Record<string, { hlc: string }>).offset_spec?.hlc), /^\d{13}:\d{5}:/, 'one clock per field');
  assert.equal(pushed[4]!.item_id, undefined, 'a template applied names no entry');
  assert.equal(pushed[7]!.container_id, COLLECTION, "a label's patch names its collection");
  assert.ok(pushed[0]!.hlc && pushed[10]!.hlc, 'a creation and a deletion carry a reading');
  assert.equal(engine.queueState.count, 0);
  assert.equal((await storage.get<StoredRecord<Record<string, unknown>>>('containers', '0192f000-0000-7000-8000-0000000000c9')), undefined,
    'the deletion answered stays a deletion');
});

test('the server answer overwrites a creation\'s prediction, and a template applied leaves no record of its own', async () => {
  const { transport, engine, storage } = await synced();
  await queue(engine, { kind: 'LABEL_CREATE', itemId: '', entityId: '0192f000-0000-7000-8000-0000000000e2', payload: { collection_id: COLLECTION, name: 'New' } });
  await queue(engine, { kind: 'TEMPLATE_APPLY', itemId: '', payload: { template_id: 't', collection_id: COLLECTION } });
  transport.down = false;
  await engine.push();
  const held = await storage.get<StoredRecord<Record<string, unknown>>>('labels', '0192f000-0000-7000-8000-0000000000e2');
  assert.equal(held?.document.from, 'server');
  assert.equal(held?.pending, undefined);
  assert.deepEqual((await storage.all<{ id: string }>('items')).map((held) => held.id), [ITEM], 'the stamp\'s answer is not a document');
});

test('a file captured offline waits in the copy, is uploaded and attached on reconnect, before what was made after it', async () => {
  const { transport, engine, storage } = await synced(true);
  assert.equal(engine.holdsFiles, true);
  const bytes = new Uint8Array([1, 2, 3, 4]);
  await engine.capture({ itemId: ITEM, fileName: 'receipt.pdf', contentType: 'application/pdf', size: bytes.length, bytes });
  await queue(engine, { kind: 'ITEM_PATCH', itemId: ITEM, fields: { title: 'Paid' } });
  assert.equal(engine.queueState.count, 2);
  const held = await storage.all<{ bytes: Uint8Array }>(FILES);
  assert.deepEqual([...(held[0]?.bytes ?? [])], [1, 2, 3, 4], 'the bytes wait in the store');
  assert.equal((await storage.get<StoredRecord<unknown>>('items', ITEM))?.pending, true);

  transport.down = false;
  await engine.push();
  assert.deepEqual(transport.sent.filter((s) => s !== 'POST /sync:pull'), ['POST /media', `POST /media/${MEDIA}:confirm`, 'POST /sync:push']);
  assert.equal(transport.transfers.length, 1);
  assert.equal(transport.transfers[0]!.url, 'https://store.example/put');
  assert.deepEqual([...(transport.transfers[0]!.body as Uint8Array)], [1, 2, 3, 4]);
  const pushed = transport.pushes[0]!.mutations;
  assert.deepEqual(pushed.map((m) => m.kind), ['SET_ADD', 'ITEM_PATCH'], 'the attachment first, then what was made after');
  assert.equal(pushed[0]!.set, 'attachments');
  assert.equal(pushed[0]!.element, MEDIA);
  assert.ok(pushed[0]!.hlc);
  assert.equal(engine.queueState.count, 0);
  assert.equal((await storage.all(FILES)).length, 0, 'the bytes left the store');
});

test('a capture that cannot reach the server stays queued with its bytes', async () => {
  const { transport, engine, storage } = await synced(true);
  await engine.capture({ itemId: ITEM, fileName: 'a.png', contentType: 'image/png', size: 1, bytes: new Uint8Array([9]) });
  await engine.push();
  assert.equal(transport.sent.filter((s) => s !== 'POST /sync:pull').length, 0);
  assert.equal(engine.queueState.count, 1);
  assert.equal((await storage.all(FILES)).length, 1);
  assert.equal((await storage.all<{ attempts: number }>(QUEUE))[0]?.attempts, 1);
});

test('a refused upload shows on the change with its code, and the rest of the queue goes on', async () => {
  const { transport, engine, storage } = await synced(true);
  await engine.capture({ itemId: ITEM, fileName: 'big.mov', contentType: 'video/quicktime', size: 10, bytes: new Uint8Array(10) });
  await queue(engine, { kind: 'ITEM_PATCH', itemId: ITEM, fields: { title: 'After' } });
  transport.down = false;
  transport.refuseMedia = new TransportError('problem', { status: 422, code: 'quota_exceeded', detailCode: 'media.quota_exceeded' });
  await engine.push();
  const rejected = engine.queueState.rejected;
  assert.equal(rejected.length, 1);
  assert.equal(rejected[0]!.kind, 'ATTACHMENT_CAPTURE');
  assert.equal(rejected[0]!.code, 'quota_exceeded');
  assert.equal(rejected[0]!.messageCode, 'media.quota_exceeded');
  assert.deepEqual(rejected[0]!.local, { file_name: 'big.mov', size: 10 });
  assert.equal((await storage.all(FILES)).length, 0, 'a refused file does not stay in the copy');
  assert.deepEqual(transport.pushes[0]!.mutations.map((m) => m.kind), ['ITEM_PATCH']);
  assert.equal(engine.queueState.count, 0);
});

test('a copy that holds no files refuses a capture: sync.needs_connection', async () => {
  const { engine, storage } = await synced(false);
  assert.equal(engine.holdsFiles, false);
  await assert.rejects(
    engine.capture({ itemId: ITEM, size: 1, bytes: new Uint8Array([1]) }),
    (error: unknown) => error instanceof TransportError && error.detailCode === 'sync.needs_connection',
  );
  assert.equal(engine.queueState.count, 0);
  assert.equal((await storage.all(FILES)).length, 0);
});
