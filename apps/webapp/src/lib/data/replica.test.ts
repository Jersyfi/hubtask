// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the replica answers, path by path: every path it answers, and the four it refuses
// by design. Over the memory store the package ships, filled the way a snapshot fills it.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { MemoryStorage, type StoredRecord } from '@hubtask/sync-engine';

import { belowOf, fetchStructureAfresh, hiddenOf, mondayOf, storeFor, useBound, type BoundType, type EntryHidden, type HeldEntry, type StructureShape } from './replica.ts';

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
// What a write becomes when it is queued: the seven kinds, and what is refused.
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

test('ENTITY_*: the vocabulary written offline, a versioned change against the version the copy holds', async () => {
  const storage = await workspace();
  await storage.put('entry_types', 't-1', { ...record({ id: 't-1', key: 'bug', name: 'Bug', level: 'TASK' }), version: 4 });
  await storage.put('custom_roles', 'r-1', { ...record({ id: 'r-1', key: 'auditor', name: 'Auditor' }), version: 2 });
  const h = helpers(storage);
  const minted = '0192f000-0000-7000-8000-00000000f00d';
  assert.deepEqual(
    await mutationFor('POST', '/entry-types', { key: 'risk', name: 'Risk', level: 'TASK' }, h),
    { kind: 'ENTITY_UPSERT', entity: 'entry_type', entityKey: 'risk', itemId: minted, payload: { key: 'risk', name: 'Risk', level: 'TASK' } },
  );
  assert.deepEqual(
    await mutationFor('PATCH', '/entry-types/bug', { name: 'Defect' }, h),
    { kind: 'ENTITY_PATCH', entity: 'entry_type', entityKey: 'bug', itemId: 't-1', baseVersion: 4, payload: { name: 'Defect' } },
  );
  assert.deepEqual(await mutationFor('DELETE', '/entry-types/bug', undefined, h), { kind: 'ENTITY_DELETE', entity: 'entry_type', entityKey: 'bug', itemId: 't-1' });
  assert.deepEqual(
    await mutationFor('PATCH', '/roles/auditor', { name: 'Auditors', permissions: ['READ'] }, h),
    { kind: 'ENTITY_PATCH', entity: 'custom_role', entityKey: 'auditor', itemId: 'r-1', baseVersion: 2, payload: { name: 'Auditors', permissions: ['READ'] } },
  );
  assert.equal((await mutationFor('POST', '/workflows', { key: 'triage', name: 'Triage', shape: {} }, h))?.entity, 'workflow');
  assert.equal((await mutationFor('POST', '/link-types', { key: 'relates', name: 'Relates' }, h))?.entity, 'link_type');
  assert.deepEqual(
    await mutationFor('POST', '/items/i-1/links', { target_id: 'i-2', type: 'blocks' }, h),
    { kind: 'ENTITY_UPSERT', entity: 'entry_link', entityKey: minted, itemId: minted, payload: { target_id: 'i-2', type: 'blocks', item_id: 'i-1' } },
  );
  assert.deepEqual(await mutationFor('DELETE', '/links/k-1', undefined, h), { kind: 'ENTITY_DELETE', entity: 'entry_link', entityKey: 'k-1', itemId: 'k-1' });
  // A change of an entity the copy does not hold names no version: it goes directly.
  assert.equal(await mutationFor('PATCH', '/entry-types/unknown', { name: 'x' }, h), undefined);
});

test('the vocabulary the copy answers: types by level, position and key, link types by position', async () => {
  const storage = await workspace();
  await storage.put('entry_types', 't-2', record({ id: 't-2', key: 'step', level: 'ACTIVITY', position: 0 }));
  await storage.put('entry_types', 't-1', record({ id: 't-1', key: 'bug', level: 'TASK', position: 1 }));
  await storage.put('entry_types', 't-0', record({ id: 't-0', key: 'task', level: 'TASK', position: 0 }));
  await storage.put('link_types', 'l-2', record({ id: 'l-2', key: 'relates', position: 2 }));
  await storage.put('link_types', 'l-1', record({ id: 'l-1', key: 'blocks', position: 1 }));
  const types = await storeFor({ path: '/entry-types' }, storage) as { data: { key: string }[] };
  assert.deepEqual(types.data.map((row) => row.key), ['task', 'bug', 'step']);
  const links = await storeFor({ path: '/link-types' }, storage) as { data: { key: string }[] };
  assert.deepEqual(links.data.map((row) => row.key), ['blocks', 'relates']);
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
    ['POST', '/containers', 'structure (§1) is not queued'],
    ['PATCH', '/containers/c-1', 'structure (§1) is not queued'],
    ['POST', '/containers/c-1/labels', 'a label\'s definition is structure'],
    ['POST', '/templates', 'structure'],
    ['POST', '/views', 'structure'],
    ['PATCH', '/items/i-1/comments/k-1', 'editing a comment has no mutation kind; only adding one does'],
    ['POST', '/memberships', 'administration (§1\'s right column)'],
    ['POST', '/backups:run', 'administration'],
    ['POST', '/workflows/triage:publish', 'publishing moves entries and is decided online (§4.4)'],
    ['PUT', '/workflow-scheme', 'the scheme is read and written online'],
    ['PUT', '/entry-types/bug/form', 'a type\'s form has no entity mutation'],
    ['POST', '/role-grants', 'a grant is who holds what - a permission (§4.3)'],
    ['POST', '/custom-fields', 'a field\'s definition is not in the replica'],
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

// A page read while the server is away (M3): a hub's tree without the bodies, in the server's
// order and without the trashed; a page with its body and its breadcrumb, never writable.
test('a hub\'s pages and a page are answered from the copy', async () => {
  const storage = new MemoryStorage();
  const page = (id: string, fields: Record<string, unknown>) => storage.put('pages', id, record({ id, hub_id: HUB, ...fields }));
  await page('p-root', { title: 'Handbook', body: 'Travel', parent_id: null, position: 'a0' });
  await page('p-child', { title: 'Trains', body: 'Book early', parent_id: 'p-root', position: 'a0', verified_until: '2020-01-01T00:00:00Z' });
  await page('p-later', { title: 'Later', body: '', parent_id: null, position: 'a1', template: true });
  await page('p-gone', { title: 'Gone', body: '', parent_id: null, position: 'a2', trashed_at: '2026-10-29T00:00:00Z' });

  const tree = await storeFor({ path: `/hubs/${HUB}/pages` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(tree.data.map((row) => row.id), ['p-root', 'p-later', 'p-child']);
  assert.ok(tree.data.every((row) => !('body' in row)), 'the tree carries no bodies');
  const templates = await storeFor({ path: `/hubs/${HUB}/pages?templates=true` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(templates.data.map((row) => row.id), ['p-later']);

  const read = await storeFor({ path: '/pages/p-child' }, storage) as Record<string, unknown>;
  assert.equal(read.body, 'Book early');
  assert.equal(read.writable, false);
  assert.equal(read.due_for_review, true);
  assert.deepEqual(read.ancestors, [{ id: 'p-root', title: 'Handbook' }]);
  assert.equal(await storeFor({ path: '/pages/p-gone' }, storage), undefined);
});

// A hub's inventory read while the server is away (UC-AST-01 check 10): the CIs of the hub in the
// server's order without the merged, narrowed by a class and its descendants; a CI with its
// relations and the protection it inherits; the impact walked by the catalogue's directions.
test('a hub\'s configuration items, a CI and its impact are answered from the copy', async () => {
  const storage = new MemoryStorage();
  await storage.put('cmdb_catalogue', 'w-1', record({
    id: 'w-1',
    classes: [
      { key: 'server', lineage: ['server', 'computer', 'hardware', 'ci'] },
      { key: 'application_service', lineage: ['application_service', 'service', 'ci'] },
      { key: 'hardware', lineage: ['hardware', 'ci'] },
    ],
    relation_types: [{ key: 'runs_on', impact: 'SOURCE' }, { key: 'connects_to', impact: 'NONE' }],
  }));
  const ci = (id: string, fields: Record<string, unknown>) => storage.put('cis', id, record({ id, hub_id: HUB, stage: 'IN_USE', ...fields }));
  await ci('ci-1', { class: 'server', name: 'web1', protection: { c: null, i: null, a: 'NORMAL', fixed: [] } });
  await ci('ci-2', { class: 'application_service', name: 'Shop', protection: { c: 'HIGH', i: null, a: 'VERY_HIGH', fixed: [] } });
  await ci('ci-3', { class: 'server', name: 'old', merged_into: 'ci-1' });
  await ci('ci-4', { class: 'server', name: 'elsewhere', hub_id: 'h-2' });
  await storage.put('ci_relations', 'r-1', record({ id: 'r-1', type: 'runs_on', source_ci_id: 'ci-2', target_ci_id: 'ci-1' }));
  await storage.put('ci_relations', 'r-2', record({ id: 'r-2', type: 'connects_to', source_ci_id: 'ci-1', target_ci_id: 'ci-4' }));

  const all = await storeFor({ path: `/hubs/${HUB}/cis` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(all.data.map((row) => row.id), ['ci-1', 'ci-2']);
  const hardware = await storeFor({ path: `/hubs/${HUB}/cis?class=hardware` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(hardware.data.map((row) => row.id), ['ci-1'], 'a class narrows to itself and its descendants');

  const read = await storeFor({ path: '/cis/ci-1' }, storage) as Record<string, unknown>;
  assert.deepEqual((read.relations as Record<string, unknown>[]).map((r) => r.id), ['r-2', 'r-1']);
  assert.deepEqual(read.effective_protection, { c: 'HIGH', i: null, a: 'VERY_HIGH' }, 'the server inherits what the application on it needs');

  const impact = await storeFor({ path: '/cis/ci-1/impact?direction=DOWNSTREAM' }, storage) as Record<string, unknown>;
  assert.deepEqual(impact.reached, [{ ci_id: 'ci-2', class: 'application_service', name: 'Shop', stage: 'IN_USE', depth: 1 }]);
  assert.deepEqual(impact.services, ['ci-2']);
  const upstream = await storeFor({ path: '/cis/ci-2/impact?direction=UPSTREAM' }, storage) as Record<string, unknown>;
  assert.deepEqual((upstream.reached as Record<string, unknown>[]).map((r) => r.ci_id), ['ci-1']);

  const classes = await storeFor({ path: '/cmdb/classes' }, storage) as { data: unknown[] };
  assert.equal(classes.data.length, 3);
  assert.equal(await storeFor({ path: '/cis/ci-9' }, storage), undefined);
});

// UC-AST-11 check 4 and UC-AST-12 check 4 offline: a hub's groups by key and its asset register
// by CI, narrowed to a state and named by its CIs, and a CI's asset; never another hub's.
test('a hub\'s CI groups, its asset register and a CI\'s asset are answered from the copy', async () => {
  const storage = new MemoryStorage();
  await storage.put('cis', 'ci-1', record({ id: 'ci-1', hub_id: HUB, class: 'server', name: 'web1' }));
  await storage.put('ci_groups', 'g-2', record({ id: 'g-2', hub_id: HUB, key: 'web', name: 'Web' }));
  await storage.put('ci_groups', 'g-1', record({ id: 'g-1', hub_id: HUB, key: 'db', name: 'Databases' }));
  await storage.put('ci_groups', 'g-3', record({ id: 'g-3', hub_id: 'h-2', key: 'all', name: 'Elsewhere' }));
  await storage.put('ci_assets', 'ci-2', record({ ci_id: 'ci-2', hub_id: HUB, state: 'IN_STOCK' }));
  await storage.put('ci_assets', 'ci-1', record({ ci_id: 'ci-1', hub_id: HUB, state: 'DEPLOYED' }));
  await storage.put('ci_assets', 'ci-4', record({ ci_id: 'ci-4', hub_id: 'h-2', state: 'DEPLOYED' }));

  const groups = await storeFor({ path: `/hubs/${HUB}/ci-groups` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(groups.data.map((row) => row.key), ['db', 'web']);
  const register = await storeFor({ path: `/hubs/${HUB}/assets` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(register.data.map((row) => [row.ci_id, row.name]), [['ci-1', 'web1'], ['ci-2', undefined]]);
  const deployed = await storeFor({ path: `/hubs/${HUB}/assets?state=DEPLOYED` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(deployed.data.map((row) => row.ci_id), ['ci-1']);
  const asset = await storeFor({ path: '/cis/ci-2/asset' }, storage) as Record<string, unknown>;
  assert.equal(asset.state, 'IN_STOCK');
  assert.equal(await storeFor({ path: '/cis/ci-9/asset' }, storage), undefined);
});

// UC-PLN-06 offline: a collection's sprints are answered from the copy, the latest start first,
// narrowed to a state when one is asked for, and never another collection's.
test('a collection\'s sprints are answered from the copy', async () => {
  const storage = new MemoryStorage();
  const sprint = (id: string, fields: Record<string, unknown>) => storage.put('sprints', id, record({ id, collection_id: COLLECTION, ...fields }));
  await sprint('s-1', { name: 'One', start_on: '2026-11-02', state: 'CLOSED' });
  await sprint('s-2', { name: 'Two', start_on: '2026-11-16', state: 'ACTIVE' });
  await sprint('s-other', { name: 'Elsewhere', start_on: '2026-11-30', state: 'PLANNED', collection_id: 'c-other' });

  const all = await storeFor({ path: `/collections/${COLLECTION}/sprints` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(all.data.map((row) => row.id), ['s-2', 's-1']);
  const active = await storeFor({ path: `/collections/${COLLECTION}/sprints?state=ACTIVE` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(active.data.map((row) => row.id), ['s-2']);
});

// A form drawn offline: the definitions in force are answered from the copy - the workspace's
// alone without a collection, with a collection's own after them, never another collection's.
test('the custom field definitions in force are answered from the copy', async () => {
  const storage = new MemoryStorage();
  const field = (id: string, fields: Record<string, unknown>) => storage.put('custom_fields', id, record({ id, ...fields }));
  await field('f-wide', { key: 'cost_centre', collection_id: null });
  await field('f-own', { key: 'priority', collection_id: COLLECTION });
  await field('f-other', { key: 'secret', collection_id: 'c-other' });

  const wide = await storeFor({ path: '/custom-fields' }, storage) as Record<string, unknown>[];
  assert.deepEqual(wide.map((row) => row.id), ['f-wide']);
  const inCollection = await storeFor({ path: `/custom-fields?collection_id=${COLLECTION}` }, storage) as Record<string, unknown>[];
  assert.deepEqual(inCollection.map((row) => row.id), ['f-wide', 'f-own']);
});

// UC-TIM-01 offline: the reader's running timer, with its minutes counted on the device, and the
// workspace's kinds of work and policy are answered from the copy; without a timer it is not
// running, and a policy never set is the default.
test('the timer, the kinds of work and the policy are answered from the copy', async () => {
  const empty = new MemoryStorage();
  assert.deepEqual(await storeFor({ path: '/timer' }, empty), { running: false });
  assert.deepEqual(await storeFor({ path: '/time-work-types' }, empty), { data: [] });
  assert.equal((await storeFor({ path: '/time-policy' }, empty) as Record<string, unknown>).visibility, 'FULL');

  const storage = new MemoryStorage();
  const started = new Date(Date.now() - 25 * 60000).toISOString();
  await storage.put('time_timers', 'me', record({ account_id: 'me', running: true, work_type: 'development', started_at: started }));
  await storage.put('time_work_types', 't-1', record({ data: [{ key: 'development', name: 'Development' }] }));
  const timer = await storeFor({ path: '/timer' }, storage) as Record<string, unknown>;
  assert.equal(timer.running, true);
  assert.equal(timer.work_type, 'development');
  assert.ok(Number(timer.elapsed_minutes) >= 24, 'the minutes are counted from the start');
  assert.equal('account_id' in timer, false);
  const kinds = await storeFor({ path: '/time-work-types' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(kinds.data.map((row) => row.key), ['development']);
});

// UC-TIM-02, 06, 07 and 08 offline: the reader's week - its head, its records placed by their
// entries, the locked days -, their capacities, the absences of a range and the cost rates are
// answered from the copy; another person's week waits for the server, and so do the rates where the
// copy holds none.
test('a week, the capacities, the absences and the rates are answered from the copy', async () => {
  assert.equal(mondayOf('2026-11-05'), '2026-11-02');
  assert.equal(mondayOf('2026-11-02'), '2026-11-02');
  assert.equal(mondayOf('2026-11-08'), '2026-11-02');

  const storage = new MemoryStorage();
  await storage.put('items', 'i-1', record({ id: 'i-1', title: 'Checkout form', collection_id: COLLECTION }));
  await storage.put('timesheets', 's-1', record({ account_id: 'me', week_start: '2026-11-02', state: 'SUBMITTED', version: 1 }));
  await storage.put('time_records', 'r-1', record({ id: 'r-1', account_id: 'me', entry_id: 'i-1', day: '2026-11-03', minutes: 90 }));
  await storage.put('time_records', 'r-2', record({ id: 'r-2', account_id: 'me', day: '2026-11-10', minutes: 30 }));
  await storage.put('time_policy', 't-1', record({ locked_through: '2026-11-02', visibility: 'FULL', version: 1 }));
  const week = await storeFor({ path: '/timesheets/2026-11-05' }, storage) as {
    state: string; minutes: number; records: Record<string, unknown>[]; days: { day: string; minutes: number; locked: boolean }[];
  };
  assert.equal(week.state, 'SUBMITTED');
  assert.equal(week.minutes, 90);
  assert.deepEqual(week.records.map((row) => [row.id, row.entry_title, row.collection_id]), [['r-1', 'Checkout form', COLLECTION]]);
  assert.deepEqual(week.days.slice(0, 2), [{ day: '2026-11-02', minutes: 0, locked: true }, { day: '2026-11-03', minutes: 90, locked: false }]);
  const open = await storeFor({ path: '/timesheets/2026-11-09' }, storage) as { state: string; minutes: number };
  assert.deepEqual([open.state, open.minutes], ['OPEN', 30]);
  assert.equal(await storeFor({ path: '/timesheets/2026-11-05?account_id=other' }, storage), undefined, "another person's week");

  await storage.put('time_capacities', 'me', record({ account_id: 'me', data: [{ from: '2026-11-02', week_minutes: [360, 360, 360, 360, 0, 0, 0] }] }));
  const capacities = await storeFor({ path: '/time-capacities' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(capacities.data.map((row) => row.from), ['2026-11-02']);

  await storage.put('time_absences', 'a-1', record({ id: 'a-1', account_id: 'ben', from: '2026-11-09', to: '2026-11-13', kind: 'LEAVE' }));
  await storage.put('time_absences', 'a-2', record({ id: 'a-2', account_id: 'me', from: '2026-12-01', to: '2026-12-01', kind: 'TRAINING' }));
  const absences = await storeFor({ path: '/time-absences?from=2026-11-01&to=2026-11-30' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(absences.data.map((row) => row.id), ['a-1']);

  assert.equal(await storeFor({ path: '/cost-rates' }, storage), undefined, 'no rates in the copy');
  await storage.put('time_cost_rates', 't-1', record({ data: [{ id: 'rate-1', role: 'designer', hour_minor: 6000 }] }));
  const rates = await storeFor({ path: '/cost-rates' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(rates.data.map((row) => row.id), ['rate-1']);
});

// UC-TIM-01 check 9: while the server is away the timer is started, stopped and discarded from the
// queue with the device's moments; a stop waiting reads the timer as stopped, and a stop or a discard
// of a timer the copy does not hold goes online.
test('the timer is started, stopped and discarded from the queue', async () => {
  const empty = new MemoryStorage();
  assert.equal(await mutationFor('POST', '/timer:stop', { zone: 'Europe/Berlin' }, helpers(empty)), undefined, 'no timer held');
  assert.equal(await mutationFor('DELETE', '/timer', undefined, helpers(empty)), undefined, 'no timer held');
  const before = Date.now();
  const start = await mutationFor('POST', '/timer:start', { work_type: 'development' }, helpers(empty));
  assert.equal(start?.kind, 'ENTITY_UPSERT');
  assert.equal(start?.entity, 'time_timer');
  assert.equal(start?.itemId, '0192f000-0000-7000-8000-00000000f00d');
  assert.equal(start?.payload?.work_type, 'development');
  assert.ok(Date.parse(String(start?.payload?.started_at)) >= before - 1000, "the start carries the device's moment");

  const storage = new MemoryStorage();
  await storage.put('time_timers', 'me', record({ id: 'me', running: true, work_type: 'development', started_at: new Date().toISOString() }));
  const stop = await mutationFor('POST', '/timer:stop', { zone: 'Europe/Berlin' }, helpers(storage));
  assert.equal(stop?.kind, 'ENTITY_PATCH');
  assert.equal(stop?.itemId, 'me');
  assert.equal(stop?.payload?.zone, 'Europe/Berlin');
  assert.ok(typeof stop?.payload?.stopped_at === 'string', "the stop carries the device's moment");
  assert.deepEqual(await mutationFor('DELETE', '/timer', undefined, helpers(storage)), {
    kind: 'ENTITY_DELETE', entity: 'time_timer', entityKey: 'own', itemId: 'me',
  });

  await storage.put('time_timers', 'me', record({ id: 'me', zone: 'Europe/Berlin', stopped_at: new Date().toISOString(), key: 'own' }));
  assert.deepEqual(await storeFor({ path: '/timer' }, storage), { running: false }, 'a stop waiting reads as stopped');
});

// UC-GOL-03 offline: the portfolios are answered from the copy by name, each project the copy
// holds named from its container and none counted; a project whose container the copy does not
// hold is left out.
test('the portfolios are answered from the copy', async () => {
  const storage = new MemoryStorage();
  await storage.put('containers', COLLECTION, record({ id: COLLECTION, type: 'COLLECTION', name: 'Checkout', archived_at: null }));
  await storage.put('portfolios', 'p-2', record({ id: 'p-2', name: 'Zeta', items: [] }));
  await storage.put('portfolios', 'p-1', record({
    id: 'p-1', name: 'Programme', status: 'ON_TRACK',
    items: [{ container_id: COLLECTION, start_on: '2026-10-01', end_on: null, status: 'AT_RISK' }, { container_id: 'gone' }],
  }));
  const list = await storeFor({ path: '/portfolios' }, storage) as { data: { id: string; items: Record<string, unknown>[] }[] };
  assert.deepEqual(list.data.map((row) => row.id), ['p-1', 'p-2']);
  assert.deepEqual(list.data[0]!.items.map((row) => [row.container_id, row.kind, row.name, row.status, row.entries]), [[COLLECTION, 'COLLECTION', 'Checkout', 'AT_RISK', null]]);
  const one = await storeFor({ path: '/portfolios/p-1' }, storage) as Record<string, unknown>;
  assert.equal(one.latest_update, null);
  assert.equal(one.progress, null);
  assert.equal(await storeFor({ path: '/portfolios/none' }, storage), undefined);
});

// UC-GOL-01, 02 and 04 offline: the periods, a period's goals and a hub's dashboards are answered
// from the copy as their definitions, nothing computed for the reader; one goal's page waits for the
// server.
test('the periods, the goals and the dashboards are answered from the copy', async () => {
  const storage = new MemoryStorage();
  await storage.put('goal_periods', 'q3', record({ id: 'q3', name: 'Q3', start_on: '2026-07-01' }));
  await storage.put('goal_periods', 'q4', record({ id: 'q4', name: 'Q4', start_on: '2026-10-01' }));
  await storage.put('goals', 'g-1', record({ id: 'g-1', period_id: 'q4', hub_id: null, title: 'Ship', key_results: [{ id: 'k-1', current: 3 }] }));
  await storage.put('goals', 'g-2', record({ id: 'g-2', period_id: 'q4', hub_id: 'h-1', parent_id: 'g-1', title: 'Checkout', key_results: [] }));
  await storage.put('goals', 'g-3', record({ id: 'g-3', period_id: 'q3', hub_id: null, title: 'Old', key_results: [] }));
  await storage.put('dashboards', 'd-1', record({ id: 'd-1', hub_id: 'h-1', name: 'Team' }));
  await storage.put('dashboards', 'd-2', record({ id: 'd-2', hub_id: null, name: 'Mine' }));

  const periods = await storeFor({ path: '/goal-periods' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(periods.data.map((row) => row.id), ['q4', 'q3'], 'the latest first');
  const goals = await storeFor({ path: '/goals?period_id=q4' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(goals.data.map((row) => [row.id, row.child_count, row.progress]), [['g-1', 1, null], ['g-2', 0, null]]);
  assert.equal((goals.data[0]!.key_results as Record<string, unknown>[])[0]!.progress, null);
  const workspace = await storeFor({ path: '/goals?workspace_only=true&period_id=q4' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(workspace.data.map((row) => row.id), ['g-1']);
  assert.equal(await storeFor({ path: '/goals/g-1' }, storage), undefined, "one goal's page");
  const dashboards = await storeFor({ path: '/dashboards?hub_id=h-1' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(dashboards.data.map((row) => row.id), ['d-1']);
  assert.equal((await storeFor({ path: '/dashboards/d-2' }, storage) as Record<string, unknown>).name, 'Mine');
});

// UC-AGT-01 … 04 offline: the agents, the models, the held acts of a status and an agent's cases
// and runs are answered from the copy, in the server's order and without the owner a record names
// only to reach its devices.
test("the agents' registry is answered from the copy", async () => {
  const storage = new MemoryStorage();
  await storage.put('agents', 'a-2', record({ id: 'a-2', name: 'Triage', owner_id: 'me', model_id: 'm-1' }));
  await storage.put('agents', 'a-1', record({ id: 'a-1', name: 'Drafts', owner_id: 'ben', model_id: null }));
  await storage.put('agent_models', 'm-1', record({ id: 'm-1', key: 'local' }));
  await storage.put('agent_requests', 'r-1', record({ id: 'r-1', agent_id: 'a-2', status: 'PENDING', created_at: '2026-11-02T09:00:00Z', owner_id: 'me' }));
  await storage.put('agent_requests', 'r-2', record({ id: 'r-2', agent_id: 'a-2', status: 'PENDING', created_at: '2026-11-02T10:00:00Z', owner_id: 'me' }));
  await storage.put('agent_requests', 'r-3', record({ id: 'r-3', agent_id: 'a-2', status: 'APPROVED', created_at: '2026-11-02T11:00:00Z', owner_id: 'me' }));
  await storage.put('agent_cases', 'c-1', record({ id: 'c-1', agent_id: 'a-2', created_at: '2026-11-01T09:00:00Z' }));
  await storage.put('agent_runs', 'u-1', record({ id: 'u-1', agent_id: 'a-2', at: '2026-11-01T09:00:00Z', owner_id: 'me' }));

  const agents = await storeFor({ path: '/agents' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(agents.data.map((row) => row.id), ['a-1', 'a-2'], 'by name');
  const mine = await storeFor({ path: '/agents?owner_id=me' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(mine.data.map((row) => row.id), ['a-2']);
  const models = await storeFor({ path: '/agent-models' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(models.data.map((row) => row.id), ['m-1']);
  const pending = await storeFor({ path: '/agent-requests?status=PENDING' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(pending.data.map((row) => row.id), ['r-2', 'r-1'], 'the latest first');
  assert.equal('owner_id' in pending.data[0]!, false);
  const cases = await storeFor({ path: '/agents/a-2/cases' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(cases.data.map((row) => row.id), ['c-1']);
  const runs = await storeFor({ path: '/agents/a-2/evaluations' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(runs.data.map((row) => row.id), ['u-1']);
  assert.equal('owner_id' in runs.data[0]!, false);
  assert.equal(await storeFor({ path: '/agents/a-2' }, storage), undefined, "one agent's spending is the server's");
});

// A board read while the server is away, an unopened one included (UC-CNV-01 check 9): a hub's
// boards without their objects, the last changed first and without the trashed; a board with its
// objects, never writable.
test('a hub\'s whiteboards and a whiteboard are answered from the copy', async () => {
  const storage = new MemoryStorage();
  const board = (id: string, fields: Record<string, unknown>) => storage.put('whiteboards', id, record({ id, hub_id: HUB, ...fields }));
  const note = { id: 's1', kind: 'STICKY', x: 0, y: 0, width: 160, height: 160, text: 'Keep it' };
  await board('w-old', { title: 'Retro', elements: [note], element_count: 1, updated_at: '2026-10-28T09:00:00Z', trashed_at: null });
  await board('w-new', { title: 'Plan', elements: [], element_count: 0, updated_at: '2026-10-30T09:00:00Z', trashed_at: null });
  await board('w-gone', { title: 'Gone', elements: [], element_count: 0, updated_at: '2026-10-30T10:00:00Z', trashed_at: '2026-10-30T10:00:00Z' });

  const list = await storeFor({ path: `/hubs/${HUB}/whiteboards` }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(list.data.map((row) => row.id), ['w-new', 'w-old']);
  assert.ok(list.data.every((row) => !('elements' in row)), 'the list carries no objects');
  assert.equal(await storeFor({ path: `/hubs/${HUB}/whiteboards?trashed=true` }, storage), undefined, 'the trash is the server\'s');

  const read = await storeFor({ path: '/whiteboards/w-old' }, storage) as Record<string, unknown>;
  assert.deepEqual(read.elements, [note]);
  assert.equal(read.writable, false);
  assert.equal(await storeFor({ path: '/whiteboards/w-gone' }, storage), undefined);
});

// An entry's SLA goals and approvals read while the server is away (M5): the goals by kind, the
// approvals newest first with their trail, and nobody's leave to decide.
test('an entry\'s goals and approvals are answered from the copy', async () => {
  const storage = new MemoryStorage();
  await storage.put('entry_slas', 's-2', record({ id: 's-2', item_id: 'i-1', goal: 'RESOLUTION', standing: 'RUNNING' }));
  await storage.put('entry_slas', 's-1', record({ id: 's-1', item_id: 'i-1', goal: 'FIRST_RESPONSE', standing: 'MET' }));
  await storage.put('entry_slas', 's-3', record({ id: 's-3', item_id: 'i-2', goal: 'RESOLUTION', standing: 'BREACHED' }));
  await storage.put('approvals', 'a-1', record({ id: 'a-1', item_id: 'i-1', state: 'DECLINED', requested_at: '2026-10-01T08:00:00Z', may_decide: false, decisions: [{ verdict: 'DECLINE' }] }));
  await storage.put('approvals', 'a-2', record({ id: 'a-2', item_id: 'i-1', state: 'PENDING', requested_at: '2026-10-02T08:00:00Z', may_decide: true, decisions: [] }));

  const goals = await storeFor({ path: '/items/i-1/sla' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(goals.data.map((row) => row.goal), ['FIRST_RESPONSE', 'RESOLUTION']);
  assert.ok(goals.data.every((row) => !('item_id' in row)), 'a goal reads as the server answers it');

  const asked = await storeFor({ path: '/items/i-1/approvals' }, storage) as { data: Record<string, unknown>[] };
  assert.deepEqual(asked.data.map((row) => row.id), ['a-2', 'a-1']);
  assert.ok(asked.data.every((row) => row.may_decide === false), 'nobody decides offline');
  assert.equal((asked.data[1]!.decisions as unknown[]).length, 1);
});
