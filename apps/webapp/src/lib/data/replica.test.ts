// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the replica answers, path by path: every path it answers, and the four it refuses
// by design. Over the memory store the package ships, filled the way a snapshot fills it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { MemoryStorage, type StoredRecord } from '@hubtask/sync-engine';

import { belowOf, fetchStructureAfresh, hiddenOf, storeFor, useBound, type BoundType, type EntryHidden, type HeldEntry, type StructureShape } from './replica.ts';

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

  // The entry page's one read: everything under the entry, the entry itself left out.
  const subtree = (await storeFor({
    path: '/items:query',
    body: { scope: { item_id: 'i-1', include_descendants: true }, include_archived: true, sort: [{ field: 'order_key', dir: 'ASC' }], expand: ['labels'], page: { size: 200 } },
  }, storage)) as { data: unknown[] };
  assert.deepEqual(ids(subtree.data), ['i-1-a']);
  // A whole collection at once is not a question the copy answers.
  assert.equal(await storeFor({
    path: '/items:query',
    body: { scope: { container_id: COLLECTION, include_descendants: true }, include_archived: true, sort: [{ field: 'order_key', dir: 'ASC' }], expand: ['labels'], page: { size: 200 } },
  }, storage), undefined);
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

// ---------------------------------------------------------------------------------------------
// What a write becomes when it is queued: every kind, and what is refused.
// ---------------------------------------------------------------------------------------------

import { mutationFor } from './replica.ts';

const helpers = (storage: MemoryStorage) => ({ storage, mintId: () => '0192f000-0000-7000-8000-00000000f00d' });

test('ITEM_CREATE: a created entry takes an identifier the client mints', async () => {
  const storage = await workspace();
  const write = await mutationFor('POST', '/items', { type: 'TASK', collection_id: COLLECTION, title: 'new' }, helpers(storage));
  assert.deepEqual(write, { kind: 'ITEM_CREATE', itemId: '0192f000-0000-7000-8000-00000000f00d', payload: { type: 'TASK', collection_id: COLLECTION, title: 'new' } });
});

test('ITEM_PATCH: an edit, a completion, an assignment, a due date, a custom field', async () => {
  const storage = await workspace();
  const h = helpers(storage);
  assert.deepEqual(await mutationFor('PATCH', '/items/i-1', { title: 'renamed', notes: null }, h), { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { title: 'renamed', notes: null } });
  assert.deepEqual(await mutationFor('POST', '/items/i-1:complete', undefined, h), { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { completion: { is_completed: true } } });
  assert.deepEqual(await mutationFor('POST', '/items/i-1:reopen', undefined, h), { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { completion: { is_completed: false } } });
  // The body is the contract's `Assignment` - `account_id` - and the mutation sets the entry's `assignee_id`.
  assert.deepEqual(await mutationFor('POST', '/items/i-1:assign', { account_id: 'acc-2' }, h), { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { assignee_id: 'acc-2' } });
  assert.deepEqual(await mutationFor('POST', '/items/i-1:unassign', undefined, h), { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { assignee_id: null } });
  assert.deepEqual(
    await mutationFor('PUT', '/items/i-1/due', { due_at: '2026-12-24T00:00:00Z', due_date_only: true, due_time_zone: 'Europe/Berlin' }, h),
    { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { due_at: '2026-12-24T00:00:00Z', due_date_only: true, due_time_zone: 'Europe/Berlin' } },
  );
  assert.deepEqual(await mutationFor('DELETE', '/items/i-1/due', undefined, h), { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { due_at: null } });
  assert.deepEqual(await mutationFor('PUT', '/items/i-1/custom-fields/urgency', { value: 'high' }, h), { kind: 'ITEM_PATCH', itemId: 'i-1', fields: { 'custom_fields.urgency': 'high' } });
});

test('MOVE: a reorder mints its rank between the neighbours the copy knows, a move names its destination', async () => {
  const storage = await workspace();
  const h = helpers(storage);
  // i-2 (a1) moved before i-1 (a0): a key below a0, at the top of the level. The collection
  // travels with a reorder: the server refuses a MOVE that names no destination, and a
  // rank-only move is a move to the same place.
  assert.deepEqual(await mutationFor('POST', '/items/i-2:reorder', { before_item_id: 'i-1' }, h), { kind: 'MOVE', itemId: 'i-2', payload: { parent_id: null, collection_id: COLLECTION, order_key: 'Zz' } });
  // i-1 moved to the end: after i-2's a1.
  assert.deepEqual(await mutationFor('POST', '/items/i-1:reorder', { before_item_id: null }, h), { kind: 'MOVE', itemId: 'i-1', payload: { parent_id: null, collection_id: COLLECTION, order_key: 'a2' } });
  // A reorder under a parent names the parent and the collection the parent is in.
  assert.deepEqual(await mutationFor('POST', '/items/i-1-a:reorder', { before_item_id: null }, h), { kind: 'MOVE', itemId: 'i-1-a', payload: { parent_id: 'i-1', collection_id: COLLECTION, order_key: 'a0' } });
  // i-9 moved under i-1, into its collection, as the first child - before i-1-a (a0).
  assert.deepEqual(
    await mutationFor('POST', '/items/i-9:move', { target_parent_id: 'i-1', target_collection_id: COLLECTION, before_item_id: 'i-1-a' }, h),
    { kind: 'MOVE', itemId: 'i-9', payload: { parent_id: 'i-1', collection_id: COLLECTION, order_key: 'Zz' } },
  );
  // A destination the copy does not hold cannot name a rank: the write goes directly.
  assert.equal(await mutationFor('POST', '/items/i-1:reorder', { before_item_id: 'nope' }, h), undefined);
  assert.equal(await mutationFor('POST', '/items/nope:reorder', { before_item_id: null }, h), undefined);
});

test('SET_ADD, SET_REMOVE, ITEM_DELETE, COMMENT_ADD', async () => {
  const storage = await workspace();
  const h = helpers(storage);
  assert.deepEqual(await mutationFor('PUT', '/items/i-1/labels/l-1', undefined, h), { kind: 'SET_ADD', itemId: 'i-1', set: 'labels', element: 'l-1' });
  assert.deepEqual(await mutationFor('DELETE', '/items/i-1/members/acc-1', undefined, h), { kind: 'SET_REMOVE', itemId: 'i-1', set: 'members', element: 'acc-1' });
  assert.deepEqual(await mutationFor('PUT', '/items/i-1/attachments/m-1', undefined, h), { kind: 'SET_ADD', itemId: 'i-1', set: 'attachments', element: 'm-1' });
  assert.deepEqual(await mutationFor('DELETE', '/items/i-1', undefined, h), { kind: 'ITEM_DELETE', itemId: 'i-1' });
  assert.deepEqual(
    await mutationFor('POST', '/items/i-1/comments', { body: 'soon', parent_comment_id: null }, h),
    { kind: 'COMMENT_ADD', itemId: 'i-1', payload: { body: 'soon', parent_comment_id: null, id: '0192f000-0000-7000-8000-00000000f00d' } },
  );
});

test('a reminder, the series, a template applied, a container, a label and a bucket', async () => {
  const storage = await workspace();
  const h = helpers(storage);
  const minted = '0192f000-0000-7000-8000-00000000f00d';
  assert.deepEqual(await mutationFor('POST', '/items/i-1/reminders', { offset_spec: 'REL:-PT1H' }, h),
    { kind: 'REMINDER_CREATE', itemId: 'i-1', entityId: minted, payload: { offset_spec: 'REL:-PT1H' } });
  assert.deepEqual(await mutationFor('PATCH', '/items/i-1/reminders/r-1', { channels: ['EMAIL'] }, h),
    { kind: 'REMINDER_PATCH', itemId: 'i-1', entityId: 'r-1', fields: { channels: ['EMAIL'] } });
  assert.deepEqual(await mutationFor('DELETE', '/items/i-1/reminders/r-1', undefined, h), { kind: 'REMINDER_DELETE', itemId: 'i-1', entityId: 'r-1' });

  assert.deepEqual(await mutationFor('PUT', '/items/i-1/recurrence', { rrule: 'FREQ=WEEKLY' }, h),
    { kind: 'RECURRENCE_CREATE', itemId: 'i-1', entityId: minted, payload: { rrule: 'FREQ=WEEKLY' } }, 'no series held: a creation');
  await storage.put('recurrence_rule', 's-1', { id: 's-1', document: { id: 's-1', item_id: 'i-1', rrule: 'FREQ=DAILY' }, version: 1, seenAt: 0 });
  assert.deepEqual(await mutationFor('PUT', '/items/i-1/recurrence', { rrule: 'FREQ=WEEKLY' }, h),
    { kind: 'RECURRENCE_PATCH', itemId: 'i-1', entityId: 's-1', fields: { rrule: 'FREQ=WEEKLY' } }, 'a series held: changed, not created again');
  assert.deepEqual(await mutationFor('DELETE', '/items/i-1/recurrence', undefined, h), { kind: 'RECURRENCE_DELETE', itemId: 'i-1', entityId: 's-1' });

  assert.deepEqual(await mutationFor('POST', '/templates/t-1:instantiate', { collection_id: COLLECTION, anchor_date: '2026-11-02' }, h),
    { kind: 'TEMPLATE_APPLY', itemId: '', payload: { collection_id: COLLECTION, anchor_date: '2026-11-02', template_id: 't-1' } });

  assert.deepEqual(await mutationFor('POST', '/containers', { type: 'COLLECTION', parent_id: 'h-1', name: 'Garden' }, h),
    { kind: 'CONTAINER_CREATE', itemId: '', entityId: minted, payload: { type: 'COLLECTION', parent_id: 'h-1', name: 'Garden' } });
  assert.deepEqual(await mutationFor('PATCH', '/containers/c-1', { name: 'Allotment' }, h), { kind: 'CONTAINER_PATCH', itemId: '', entityId: 'c-1', fields: { name: 'Allotment' } });
  assert.deepEqual(await mutationFor('DELETE', '/containers/c-1', undefined, h), { kind: 'CONTAINER_DELETE', itemId: '', entityId: 'c-1' });

  assert.deepEqual(await mutationFor('POST', '/containers/c-1/labels', { name: 'Urgent', color_token: 'red' }, h),
    { kind: 'LABEL_CREATE', itemId: '', entityId: minted, payload: { name: 'Urgent', color_token: 'red', collection_id: 'c-1' } });
  assert.deepEqual(await mutationFor('PATCH', '/containers/c-1/labels/l-1', { name: 'Soon' }, h),
    { kind: 'LABEL_PATCH', itemId: '', entityId: 'l-1', containerId: 'c-1', fields: { name: 'Soon' } });
  assert.deepEqual(await mutationFor('DELETE', '/containers/c-1/labels/l-1', undefined, h), { kind: 'LABEL_DELETE', itemId: '', entityId: 'l-1', containerId: 'c-1' });
  assert.deepEqual(await mutationFor('POST', '/containers/c-1/buckets', { name: 'Doing' }, h),
    { kind: 'BUCKET_CREATE', itemId: '', entityId: minted, payload: { name: 'Doing', collection_id: 'c-1' } });
  assert.deepEqual(await mutationFor('PATCH', '/containers/c-1/buckets/b-1', { wip_limit: 3 }, h),
    { kind: 'BUCKET_PATCH', itemId: '', entityId: 'b-1', containerId: 'c-1', fields: { wip_limit: 3 } });
  assert.deepEqual(await mutationFor('DELETE', '/containers/c-1/buckets/b-1', undefined, h), { kind: 'BUCKET_DELETE', itemId: '', entityId: 'b-1', containerId: 'c-1' });
});

test('the writes the queue does not carry, and why', async () => {
  const storage = await workspace();
  const h = helpers(storage);
  for (const [method, path, why] of [
    ['POST', '/items/i-1:archive', 'lifecycle the server owns'],
    ['POST', '/items/i-1:restore', 'the trash is the server\'s'],
    ['POST', '/items/i-1:purge', 'irreversible, and the server\'s'],
    ['POST', '/items/i-1:duplicate', 'no mutation kind copies a subtree'],
    ['PUT', '/items/i-1/cover', 'the cover has no mutation kind'],
    ['POST', '/items:bulk', 'a bulk is many operations the server judges together'],
    ['PATCH', '/containers/c-1/structure', 'a structure is resolved by the server, never pushed (§4.2)'],
    ['PUT', '/containers/c-1/policies', 'a container\'s policies have no field in the frame yet'],
    ['PUT', '/containers/c-1/privacy', 'privacy is a permission (§1\'s right column)'],
    ['POST', '/containers/c-1:move', 'a container\'s place has no field in the frame yet'],
    ['POST', '/containers/c-1/buckets/b-1:reorder', 'a column\'s place has no field in the frame yet'],
    ['POST', '/containers/c-1/labels/l-1:restore', 'the trash is the server\'s'],
    ['POST', '/templates', 'a template\'s definition stays online; applying one is queued'],
    ['DELETE', '/items/i-9/recurrence', 'no series held to delete'],
    ['POST', '/views', 'structure'],
    ['PATCH', '/items/i-1/comments/k-1', 'editing a comment has no mutation kind; only adding one does'],
    ['POST', '/memberships', 'administration (§1\'s right column)'],
    ['POST', '/backups:run', 'administration'],
  ] as const) {
    assert.equal(await mutationFor(method, path, {}, h), undefined, `${method} ${path}: ${why}`);
  }
});

// What a collection hides of an entry: the server's rule, read from the same table the Go domain
// reads (`api/fixtures/hidden-entries.json`). A case added on either side turns both red or neither.

interface HiddenCase {
  readonly name: string;
  readonly structure: StructureShape;
  readonly type: string;
  readonly ancestors: readonly string[];
  readonly hidden: EntryHidden;
}

const hiddenFixture = JSON.parse(
  readFileSync(fileURLToPath(new URL('../../../../../api/fixtures/hidden-entries.json', import.meta.url)), 'utf8'),
) as { bound: BoundType[]; cases: HiddenCase[]; below: BelowCase[] };

interface BelowCase {
  readonly name: string;
  readonly levels: readonly string[];
  readonly entries: readonly { id: string; type: string; parent: string | null; trashed?: boolean }[];
  readonly below: Readonly<Record<string, number>>;
}

test('the shared table holds cases', () => {
  assert.ok(hiddenFixture.cases.length > 0);
});

for (const tc of hiddenFixture.cases) {
  test(`hidden: ${tc.name}`, () => {
    assert.deepEqual(hiddenOf(tc.type, tc.ancestors, tc.structure, hiddenFixture.bound), tc.hidden);
  });
}

for (const tc of hiddenFixture.below) {
  test(`hidden below: ${tc.name}`, () => {
    const structure: StructureShape = { levels: tc.levels.map((type) => ({ type })) };
    const held = new Map(tc.entries.map((entry) => [entry.id, entry]));
    const children = new Map<string, HeldEntry[]>();
    for (const entry of tc.entries) {
      if (!entry.parent) continue;
      const row: HeldEntry = { id: entry.id, type: entry.type, parent_id: entry.parent, deleted_at: entry.trashed ? 'gone' : null };
      children.set(entry.parent, [...(children.get(entry.parent) ?? []), row]);
    }
    for (const entry of tc.entries) {
      const ancestors: string[] = [];
      for (let at = held.get(entry.parent ?? ''); at; at = held.get(at.parent ?? '')) ancestors.push(at.type);
      assert.equal(belowOf(entry, ancestors, children, structure, hiddenFixture.bound), tc.below[entry.id] ?? 0, entry.id);
    }
  });
}

test('offline, an entry is marked from its collection\'s structure and the levels above it', async () => {
  const storage = new MemoryStorage();
  const list = hiddenFixture.cases.find((tc) => tc.name.startsWith('Tasks offers'))!.structure;
  await storage.put('containers', COLLECTION, record({ id: COLLECTION, type: 'COLLECTION', parent_id: HUB, order_key: 'a0', structure: { settings: list } }));
  await storage.put('items', 'task', record({ id: 'task', type: 'TASK', collection_id: COLLECTION, parent_id: null, order_key: 'a0' }));
  await storage.put('items', 'pack', record({ id: 'pack', type: 'WORK_PACKAGE', collection_id: COLLECTION, parent_id: 'task', order_key: 'a0' }));
  await storage.put('items', 'step', record({ id: 'step', type: 'ACTIVITY', collection_id: COLLECTION, parent_id: 'pack', order_key: 'a0' }));
  await storage.put('items', 'call', record({ id: 'call', type: 'ACTIVITY', collection_id: COLLECTION, parent_id: 'task', order_key: 'a1' }));

  useBound(undefined);
  const before = (await storeFor({ path: '/items/step' }, storage)) as Record<string, unknown>;
  assert.equal(before.hidden, undefined, 'before the manifest arrives nothing is marked');

  useBound(hiddenFixture.bound);
  try {
    const step = (await storeFor({ path: '/items/step' }, storage)) as { hidden: EntryHidden };
    assert.equal(step.hidden.level, true, 'an activity under a hidden work package is hidden');
    const call = (await storeFor({ path: '/items/call' }, storage)) as { hidden: EntryHidden };
    assert.equal(call.hidden.level, false, 'an activity directly under a task is shown');
    const task = (await storeFor({ path: '/items/task' }, storage)) as { hidden: EntryHidden };
    assert.equal(task.hidden.below, 2, 'the task says the work package and its activity are hidden under it');
    const subtree = (await storeFor({
      path: '/items:query',
      body: { scope: { item_id: 'task', include_descendants: true }, include_archived: true, sort: [{ field: 'order_key', dir: 'ASC' }], expand: ['labels'], page: { size: 200 } },
    }, storage)) as { data: { id: string; hidden: EntryHidden }[] };
    const marks = Object.fromEntries(subtree.data.map((row) => [row.id, row.hidden.level]));
    assert.deepEqual(marks, { pack: true, step: true, call: false });
  } finally {
    useBound(undefined);
  }
});

test('a copy held without a container\'s structure is fetched afresh where the manifest announces it', async () => {
  const STRUCTURE = { preset: 'LIST', settings: { levels: [{ type: 'TASK' }] } };
  const asked = async (storage: MemoryStorage | undefined, features: Record<string, unknown> | undefined) => {
    const reasons: string[] = [];
    const sync = { storage, resynchronise: async (reason: string) => (reasons.push(reason), true) };
    await fetchStructureAfresh(sync, features);
    return reasons;
  };
  const held = await workspace();
  assert.deepEqual(await asked(held, { structure: true }), ['structure'], 'a container without its structure, the feature on');
  assert.deepEqual(await asked(held, { structure: false }), [], 'the feature off');
  assert.deepEqual(await asked(held, undefined), [], 'before the manifest arrives');
  assert.deepEqual(await asked(undefined, { structure: true }), [], 'no store, online-only');

  const current = new MemoryStorage();
  await current.put('containers', HUB, record({ id: HUB, type: 'HUB', parent_id: null, structure: STRUCTURE }));
  await current.put('containers', COLLECTION, record({ id: COLLECTION, type: 'COLLECTION', parent_id: HUB, structure: STRUCTURE }));
  assert.deepEqual(await asked(current, { structure: true }), [], 'every container holds its structure');
  assert.deepEqual(await asked(new MemoryStorage(), { structure: true }), [], 'an empty copy waits for its first snapshot');
});
