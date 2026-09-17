// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the replica answers, path by path (F6-04): every path it answers, and the four it refuses
// by design. Over the memory store the package ships, filled the way a snapshot fills it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MemoryStorage, type StoredRecord } from '@hubtask/sync-engine';

import { storeFor } from './replica.ts';

const HUB = 'h-1';
const COLLECTION = 'c-1';
const OTHER = 'c-2';

function record(document: Record<string, unknown>, sets?: Record<string, readonly string[]>): StoredRecord<Record<string, unknown>> {
  return { id: String(document.id), document, version: 1, seenAt: 0, ...(sets ? { sets } : {}) };
}

async function workspace(): Promise<MemoryStorage> {
  const storage = new MemoryStorage();
  await storage.put('containers', HUB, record({ id: HUB, type: 'HUB', name: 'Engines', parent_id: null, order_key: 'a0' }));
  await storage.put('containers', COLLECTION, record({ id: COLLECTION, type: 'COLLECTION', name: 'Inbox', parent_id: HUB, order_key: 'a0' }));
  await storage.put('containers', OTHER, record({ id: OTHER, type: 'COLLECTION', name: 'Later', parent_id: HUB, order_key: 'a1' }));
  await storage.put('containers', 'c-gone', record({ id: 'c-gone', type: 'COLLECTION', name: 'Gone', parent_id: HUB, order_key: 'a2', deleted_at: '2026-01-01T00:00:00Z' }));
  await storage.put('items', 'i-2', record({ id: 'i-2', title: 'second', collection_id: COLLECTION, parent_id: null, order_key: 'a1', bucket_id: 'b-1' }));
  await storage.put('items', 'i-1', record({ id: 'i-1', title: 'first', collection_id: COLLECTION, parent_id: null, order_key: 'a0', bucket_id: null }, { labels: ['l-1'], members: ['acc-1'] }));
  await storage.put('items', 'i-1-a', record({ id: 'i-1-a', title: 'a child', collection_id: COLLECTION, parent_id: 'i-1', order_key: 'a0' }));
  await storage.put('items', 'i-9', record({ id: 'i-9', title: 'elsewhere', collection_id: OTHER, parent_id: null, order_key: 'a0' }));
  await storage.put('buckets', 'b-1', record({ id: 'b-1', collection_id: COLLECTION, name: 'Doing', order_key: 'a0' }));
  await storage.put('labels', 'l-1', record({ id: 'l-1', collection_id: COLLECTION, name: 'urgent', order_key: 'a0' }));
  await storage.put('comments', 'k-2', record({ id: 'k-2', item_id: 'i-1', body: 'later', created_at: '2026-09-17T11:00:00Z' }));
  await storage.put('comments', 'k-1', record({ id: 'k-1', item_id: 'i-1', body: 'first', created_at: '2026-09-17T10:00:00Z' }));
  await storage.put('reminder', 'r-1', record({ id: 'r-1', item_id: 'i-1', offset_spec: 'REL:-PT30M' }));
  await storage.put('recurrence_rule', 'rr-1', record({ id: 'rr-1', item_id: 'i-2', rrule: 'FREQ=DAILY' }));
  await storage.put('template', 't-hub', record({ id: 't-hub', name: 'Hub-wide', scope_type: 'HUB', scope_id: HUB }));
  await storage.put('template', 't-ws', record({ id: 't-ws', name: 'Workspace', scope_type: 'TENANT', scope_id: null }));
  await storage.put('template', 't-other', record({ id: 't-other', name: 'Elsewhere', scope_type: 'COLLECTION', scope_id: OTHER }));
  return storage;
}

const ids = (rows: unknown) => ((rows as { id: string }[]) ?? []).map((row) => row.id);

test('the tree: the hubs, a hub\'s collections, and one node', async () => {
  const storage = await workspace();
  const hubs = (await storeFor({ path: '/containers?type=HUB&page_size=200' }, storage)) as { data: unknown[]; page: unknown };
  assert.deepEqual(ids(hubs.data), [HUB]);
  assert.deepEqual(hubs.page, { next_cursor: null, has_more: false });
  const collections = (await storeFor({ path: `/containers?parent_id=${HUB}&page_size=200` }, storage)) as { data: unknown[] };
  assert.deepEqual(ids(collections.data), [COLLECTION, OTHER], 'in the manual order, the deleted one left out');
  const one = (await storeFor({ path: `/containers/${COLLECTION}` }, storage)) as { name: string };
  assert.equal(one.name, 'Inbox');
  assert.equal(await storeFor({ path: '/containers/nope' }, storage), undefined);
});

test('a level: the plain question the list asks, in the manual order, labels and members folded in', async () => {
  const storage = await workspace();
  const level = (await storeFor({
    path: '/items:query',
    body: { scope: { container_id: COLLECTION, include_descendants: false }, include_archived: true, sort: [{ field: 'order_key', dir: 'ASC' }], expand: ['labels'], page: { size: 200 } },
  }, storage)) as { data: Record<string, unknown>[]; groups: unknown[]; total: number };
  assert.deepEqual(ids(level.data), ['i-1', 'i-2'], 'the collection\'s own entries, not the child, not the other collection\'s');
  assert.deepEqual(level.data[0]?.label_ids, ['l-1']);
  assert.deepEqual(level.data[0]?.member_ids, ['acc-1']);
  assert.deepEqual(level.groups, []);
  assert.equal(level.total, 2);

  const children = (await storeFor({
    path: '/items:query',
    body: { scope: { item_id: 'i-1', include_descendants: false }, include_archived: true, sort: [{ field: 'order_key', dir: 'ASC' }], expand: ['labels'], page: { size: 200 } },
  }, storage)) as { data: unknown[] };
  assert.deepEqual(ids(children.data), ['i-1-a']);
});

test('the board: the same entries by column, the loose ones last', async () => {
  const storage = await workspace();
  const board = (await storeFor({
    path: '/items:query',
    body: { scope: { container_id: COLLECTION, include_descendants: false }, include_archived: true, group_by: { field: 'bucket_id', limit_per_group: 50 }, sort: [{ field: 'order_key', dir: 'ASC' }], expand: ['labels'], count: 'exact' },
  }, storage)) as { groups: { key: string | null; count: number; data: unknown[] }[]; total: number };
  assert.deepEqual(board.groups.map((g) => [g.key, ids(g.data)]), [['b-1', ['i-2']], [null, ['i-1']]]);
  assert.equal(board.total, 2);
});

test('one entry, and what hangs on it: comments in order, reminders, the series, the templates above', async () => {
  const storage = await workspace();
  assert.deepEqual(((await storeFor({ path: '/items/i-1' }, storage)) as { label_ids: string[] }).label_ids, ['l-1']);
  assert.equal(await storeFor({ path: '/items/nope' }, storage), undefined);
  assert.deepEqual(ids(((await storeFor({ path: '/items/i-1/comments' }, storage)) as { data: unknown[] }).data), ['k-1', 'k-2']);
  assert.deepEqual(ids(await storeFor({ path: '/items/i-1/reminders' }, storage)), ['r-1']);
  assert.deepEqual(ids(await storeFor({ path: '/items/i-2/reminders' }, storage)), []);
  assert.equal(((await storeFor({ path: '/items/i-2/recurrence' }, storage)) as { rrule: string }).rrule, 'FREQ=DAILY');
  assert.equal(await storeFor({ path: '/items/i-1/recurrence' }, storage), undefined, 'no series is not a document');
  assert.deepEqual(ids(await storeFor({ path: `/containers/${COLLECTION}/buckets` }, storage)), ['b-1']);
  assert.deepEqual(ids(await storeFor({ path: `/containers/${COLLECTION}/labels` }, storage)), ['l-1']);
  assert.deepEqual(ids(((await storeFor({ path: `/templates?container_id=${COLLECTION}` }, storage)) as { data: unknown[] }).data).sort(), ['t-hub', 't-ws']);
  assert.deepEqual(ids(((await storeFor({ path: '/templates' }, storage)) as { data: unknown[] }).data).length, 3);
});

test('the four the replica refuses by design', async () => {
  const storage = await workspace();
  // 1. A query with a filter, a sort of its own or a grouping that is not the board's: the
  //    language is the server's (ADR-0026).
  const plain = { scope: { container_id: COLLECTION, include_descendants: false }, sort: [{ field: 'order_key', dir: 'ASC' }] };
  assert.equal(await storeFor({ path: '/items:query', body: { ...plain, filter: { field: 'title', op: 'contains', value: 'x' } } }, storage), undefined);
  assert.equal(await storeFor({ path: '/items:query', body: { ...plain, sort: [{ field: 'due_at', dir: 'ASC' }] } }, storage), undefined);
  assert.equal(await storeFor({ path: '/items:query', body: { ...plain, group_by: { field: 'assignee_id', limit_per_group: 50 } } }, storage), undefined);
  assert.equal(await storeFor({ path: '/items:query', body: { scope: { container_id: COLLECTION, include_descendants: true } } }, storage), undefined);
  // 2. Search, for the same reason twice over.
  assert.equal(await storeFor({ path: '/search', body: { q: 'milk' } }, storage), undefined);
  // 3. The history is not synchronised.
  assert.equal(await storeFor({ path: '/items/i-1/activity?page_size=25' }, storage), undefined);
  // 4. The administration's data is §1's right column.
  for (const path of ['/accounts/me', '/memberships?scope_type=TENANT', '/groups', '/tokens', '/audit', '/backups', '/ai-provider', '/quotas', '/identity-provider']) {
    assert.equal(await storeFor({ path }, storage), undefined, path);
  }
});
