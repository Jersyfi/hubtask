// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The copy of a very large workspace (offline-sync.md §1): past the bound the copy holds the
// containers the application names, halves them while it is still too large, never narrows a copy
// of one container, keeps the stream to what it holds and takes itself afresh when the wanted
// containers change.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SyncEngine, type ReplicaBound } from '../src/SyncEngine.ts';
import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import { collectionOf } from '../src/replica.ts';
import type { ChangeRecord } from '../src/schema.ts';
import { FakeTransport, FixedClock } from './fakes.ts';

const A = '0192f000-0000-7000-8000-0000000000a1';
const B = '0192f000-0000-7000-8000-0000000000a2';
const C = '0192f000-0000-7000-8000-0000000000a3';
const D = '0192f000-0000-7000-8000-0000000000a4';

const entry = (n: number, container: string): ChangeRecord =>
  ({
    op: 'UPSERT', entity: 'item', entity_id: `0192f000-0000-7000-8000-${String(n).padStart(12, '0')}`,
    container_id: container, payload: { id: n, title: `entry ${n}` },
  }) as unknown as ChangeRecord;

const entries = (count: number, container: string, from = 1) =>
  Array.from({ length: count }, (_, i) => entry(from + i, container));

const pathsFor = () => [];

async function settle(turns = 30): Promise<void> {
  for (let i = 0; i < turns; i += 1) await new Promise((resolve) => setTimeout(resolve, 0));
}

function bound(containers: () => readonly string[], records = 2): ReplicaBound {
  return { records, counts: (record) => record.entity === 'item', containers: async () => containers() };
}

async function engineOver(transport: FakeTransport, limits: ReplicaBound, storage = new MemoryStorage()) {
  const engine = new SyncEngine({ transport, clock: new FixedClock(), bound: limits });
  await engine.attach(storage, { platform: 'web', displayName: 'test' });
  transport.answer('/sync:pull', { changes: [], cursor: 'c-2', has_more: false });
  return { engine, storage };
}

const scopesOf = (body: unknown) => (body as { scopes?: { container_id: string }[] }).scopes?.map((s) => s.container_id);
const items = async (storage: MemoryStorage) => (await storage.all(collectionOf('item'))).length;

test('a copy past the bound holds the containers the application names', async () => {
  const transport = new FakeTransport()
    .snapshotSessions({ records: entries(3, A), cursor: 'c-1' }, { records: entries(2, A), cursor: 'c-1' })
    .streamSessions({ open: true });
  const { engine, storage } = await engineOver(transport, bound(() => [A]));

  const stop = engine.listen({ pathsFor });
  await settle();
  stop();

  assert.equal(transport.snapshots.length, 2);
  assert.equal(scopesOf(transport.snapshots[0]?.body), undefined, 'the first walk asks for everything');
  assert.deepEqual(scopesOf(transport.snapshots[1]?.body), [A]);
  assert.deepEqual(engine.heldContainers, [A]);
  assert.equal(await items(storage), 2);
  const pull = transport.calls.find((call) => call.path === '/sync:pull');
  assert.deepEqual(scopesOf(pull?.body), [A], 'the delta asks for the same containers');
});

test('a narrowed copy still past the bound holds the first half, and one container is never narrowed', async () => {
  const transport = new FakeTransport()
    .snapshotSessions(
      { records: entries(3, A), cursor: 'c-1' },
      { records: entries(3, A), cursor: 'c-1' },
      { records: entries(3, A), cursor: 'c-1' },
      { records: entries(3, A), cursor: 'c-1' },
    )
    .streamSessions({ open: true });
  const { engine, storage } = await engineOver(transport, bound(() => [A, B, C, D]));

  const stop = engine.listen({ pathsFor });
  await settle(60);
  stop();

  assert.deepEqual(transport.snapshots.map((s) => scopesOf(s.body)), [undefined, [A, B, C, D], [A, B], [A]]);
  assert.deepEqual(engine.heldContainers, [A]);
  assert.equal(await items(storage), 3, 'a copy of one container is held whole');
});

test('the stream keeps a narrowed copy to what it holds', async () => {
  const outside = entry(90, B);
  const inside = entry(91, A);
  const transport = new FakeTransport()
    .snapshotSessions({ records: entries(3, A), cursor: 'c-1' }, { records: entries(1, A), cursor: 'c-1' })
    .streamSessions({
      events: [
        { id: 'c-3', event: 'item', data: JSON.stringify(outside) },
        { id: 'c-4', event: 'item', data: JSON.stringify(inside) },
      ],
      open: true,
    });
  const { engine, storage } = await engineOver(transport, bound(() => [A]));
  const seen: string[] = [];

  const stop = engine.listen({ pathsFor: (record) => { seen.push(record.entity_id); return []; } });
  await settle(60);
  stop();

  assert.equal(await storage.get(collectionOf('item'), outside.entity_id), undefined, 'held outside the copy');
  assert.notEqual(await storage.get(collectionOf('item'), inside.entity_id), undefined);
  assert.ok(seen.includes(outside.entity_id), 'a record left out of the copy still makes its reads stale');
});

test('a narrowed copy is taken afresh when the wanted containers change', async () => {
  let wanted: readonly string[] = [A];
  const transport = new FakeTransport()
    .snapshotSessions({ records: entries(3, A), cursor: 'c-1' }, { records: entries(1, A), cursor: 'c-1' })
    .streamSessions({ open: true });
  const { engine, storage } = await engineOver(transport, bound(() => wanted));
  const stop = engine.listen({ pathsFor });
  await settle();
  stop();
  assert.equal(await engine.renarrow(), false, 'the same containers take nothing afresh');

  wanted = [B];
  transport.snapshotSessions({ records: entries(1, B, 50), cursor: 'c-5' });
  assert.equal(await engine.renarrow(), true);
  assert.equal(await items(storage), 0, 'the copy of the old containers is gone');
  const again = engine.listen({ pathsFor });
  await settle();
  again();

  assert.deepEqual(scopesOf(transport.snapshots.at(-1)?.body), [B]);
  assert.equal(await items(storage), 1);
});

test('a copy within the bound holds everything, and an engine without one never narrows', async () => {
  const transport = new FakeTransport().snapshotSessions({ records: entries(2, A), cursor: 'c-1' }).streamSessions({ open: true });
  const { engine } = await engineOver(transport, bound(() => [A]));
  const stop = engine.listen({ pathsFor });
  await settle();
  stop();
  assert.equal(engine.heldContainers, undefined);
  assert.equal(transport.snapshots.length, 1);
});
