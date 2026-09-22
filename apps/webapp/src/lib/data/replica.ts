// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the replica answers while the server cannot be reached (F6-04), in this application's own
 * paths — the shape `live.ts` follows for the other direction.
 *
 * **The engine does not learn which path is a list of what.** It hands the request and the store
 * here, and this answers the document the path would have answered: the tree and one node, a
 * level's entries in the server's order, one entry, the collections the snapshot delivered. What
 * comes back is exactly what the server would have sent for that read, so that no component learns
 * where its data came from — the state carries `source: 'replica'`, and that is the whole of the
 * difference a screen sees.
 *
 * **`undefined` is a real answer and the common one.** A path the replica cannot answer is `failed`
 * as it would have been, named `sync.needs_connection`. Four kinds refuse *by design*, and the test
 * lists them: `/items:query` with a filter, a sort other than the manual order or a grouping other
 * than the board's, because the query language is the server's (ADR-0026) and answering a filter
 * from the copy would be a second implementation of it; `/search`, for the same reason twice over;
 * `/activity`, because the history is not synchronised (offline-sync.md §3.1 walks no history);
 * and the administration's data — accounts, groups, memberships, tokens, audit, backups,
 * providers, quotas — because §1's right column *is* administration.
 *
 * **The plain question a level asks is answered.** This client reads a level and a board through
 * `POST /items:query` (`items.svelte.ts`), not through `GET /items`; the question it asks there is
 * its own — a scope, the manual order, archived included, labels expanded — and a copy can answer
 * that without knowing the language. A board grouped by bucket is the same question in columns.
 */

import { orderKeyBetween, type QueuedWrite, type ResourceRequest, type Storage, type StoredRecord } from '@hubtask/sync-engine';

type Document = Record<string, unknown>;

/** A page, as the contract's envelope spells one: everything, on one page, no successor. */
const page = <T>(data: readonly T[]) => ({ data, page: { next_cursor: null, has_more: false } });

/** `order_key ASC` — the manual order, and the order the server lists in. */
const byOrderKey = (a: Document, b: Document): number => {
  const x = String(a.order_key ?? '');
  const y = String(b.order_key ?? '');
  return x < y ? -1 : x > y ? 1 : 0;
};

/** The documents of one collection, the sets an entry carries folded into the fields the API spells. */
async function documents(storage: Storage, collection: string): Promise<Document[]> {
  const held = await storage.all<StoredRecord<Document>>(collection);
  return held.map((record) => withSets(record));
}

/**
 * An entry's document as the API would send it with `expand: ['labels']`: the sets the replica
 * keeps beside the document become `label_ids` and `member_ids`. Held beside rather than inside,
 * because the server's document carries no sets and a whole object arriving later must not take
 * them with it (`replica.ts` in the package).
 */
function withSets(record: StoredRecord<Document>): Document {
  const sets = record.sets ?? {};
  return {
    ...record.document,
    ...(sets.labels ? { label_ids: [...sets.labels] } : {}),
    ...(sets.members ? { member_ids: [...sets.members] } : {}),
  };
}

async function one(storage: Storage, collection: string, id: string): Promise<Document | undefined> {
  const record = await storage.get<StoredRecord<Document>>(collection, id);
  return record ? withSets(record) : undefined;
}

/**
 * The plain level question - or an entry's whole subtree, which the entry page asks for in one
 * read (issue 877) - or nothing for one the copy would have to understand.
 */
function levelScope(body: unknown): { container_id?: string; item_id?: string; group?: boolean; descendants?: boolean } | undefined {
  if (body === null || typeof body !== 'object') return undefined;
  const query = body as Record<string, unknown>;
  if (query.filter !== undefined) return undefined;
  const sort = query.sort;
  const manual = sort === undefined
    || (Array.isArray(sort) && sort.length === 1 && (sort[0] as Document)?.field === 'order_key' && (sort[0] as Document)?.dir === 'ASC');
  if (!manual) return undefined;
  const group = query.group_by as Document | undefined;
  if (group !== undefined && group.field !== 'bucket_id') return undefined;
  const scope = query.scope as Document | undefined;
  if (!scope) return undefined;
  const containerId = typeof scope.container_id === 'string' ? scope.container_id : undefined;
  const itemId = typeof scope.item_id === 'string' ? scope.item_id : undefined;
  if (!containerId && !itemId) return undefined;
  // A whole collection or hub at once is a question no screen asks of the copy; an entry's
  // subtree is bounded by the levels below it and is what its page shows.
  if (scope.include_descendants && (!itemId || group !== undefined)) return undefined;
  return { container_id: containerId, item_id: itemId, group: group !== undefined, descendants: scope.include_descendants === true };
}

/**
 * The entries of one level - a collection's own, or one entry's children - in the manual order;
 * or everything under one entry, the anchor left out as the server leaves it out.
 */
async function level(storage: Storage, scope: { container_id?: string; item_id?: string; descendants?: boolean }): Promise<Document[]> {
  const items = await documents(storage, 'items');
  const live = items.filter((item) => !item.deleted_at);
  if (scope.item_id && scope.descendants) {
    const under = (parentId: string): Document[] =>
      live.filter((item) => item.parent_id === parentId).flatMap((child) => [child, ...under(child.id as string)]);
    return under(scope.item_id).sort(byOrderKey);
  }
  const own = live.filter((item) => {
    if (scope.item_id) return item.parent_id === scope.item_id;
    return item.collection_id === scope.container_id && !item.parent_id;
  });
  return own.sort(byOrderKey);
}

/** The board: the same entries by column, the entries with no column last, as the API orders them. */
async function board(storage: Storage, containerId: string): Promise<unknown> {
  const rows = await level(storage, { container_id: containerId });
  const buckets = (await documents(storage, 'buckets'))
    .filter((bucket) => bucket.collection_id === containerId)
    .sort(byOrderKey);
  const groups = buckets.map((bucket) => {
    const data = rows.filter((row) => row.bucket_id === bucket.id);
    return { key: bucket.id as string, count: data.length, data, page: { next_cursor: null, has_more: false } };
  });
  const loose = rows.filter((row) => !row.bucket_id || !buckets.some((bucket) => bucket.id === row.bucket_id));
  groups.push({ key: null as unknown as string, count: loose.length, data: loose, page: { next_cursor: null, has_more: false } });
  return { data: [], groups, page: { next_cursor: null, has_more: false }, total: rows.length };
}

/** What a request's path says, split from its query. */
function parse(path: string): { pathname: string; query: URLSearchParams } {
  const url = new URL(path, 'http://localhost');
  return { pathname: url.pathname, query: url.searchParams };
}

/**
 * The document the replica answers for a request, or `undefined` for one it cannot.
 *
 * Every answered path is listed here, and the test lists every one refused by design.
 */
export async function storeFor(request: ResourceRequest, storage: Storage): Promise<unknown> {
  const { pathname, query } = parse(request.path);

  if (pathname === '/items:query') {
    const scope = levelScope(request.body);
    if (!scope) return undefined;
    if (scope.group) return scope.container_id ? board(storage, scope.container_id) : undefined;
    const rows = await level(storage, scope);
    return { data: rows, groups: [], page: { next_cursor: null, has_more: false }, total: rows.length };
  }

  if (pathname === '/containers') {
    const all = (await documents(storage, 'containers')).filter((c) => !c.deleted_at);
    const type = query.get('type');
    const parent = query.get('parent_id');
    const chosen = all.filter((container) => {
      if (type && container.type !== type) return false;
      if (parent && container.parent_id !== parent) return false;
      if (!parent && !type) return true;
      return true;
    });
    return page(chosen.sort(byOrderKey));
  }

  let match = /^\/containers\/([^/]+)$/.exec(pathname);
  if (match?.[1]) return one(storage, 'containers', match[1]);

  match = /^\/containers\/([^/]+)\/(buckets|labels)$/.exec(pathname);
  if (match?.[1] && match[2]) {
    const collectionId = match[1];
    const rows = (await documents(storage, match[2])).filter((row) => row.collection_id === collectionId);
    return rows.sort(byOrderKey);
  }

  match = /^\/items\/([^/]+)$/.exec(pathname);
  if (match?.[1]) return one(storage, 'items', match[1]);

  match = /^\/items\/([^/]+)\/comments$/.exec(pathname);
  if (match?.[1]) {
    const itemId = match[1];
    const rows = (await documents(storage, 'comments')).filter((row) => row.item_id === itemId);
    rows.sort((a, b) => String(a.created_at ?? '').localeCompare(String(b.created_at ?? '')));
    return page(rows);
  }

  match = /^\/items\/([^/]+)\/reminders$/.exec(pathname);
  if (match?.[1]) {
    const itemId = match[1];
    return (await documents(storage, 'reminder')).filter((row) => row.item_id === itemId);
  }

  match = /^\/items\/([^/]+)\/recurrence$/.exec(pathname);
  if (match?.[1]) {
    const itemId = match[1];
    return (await documents(storage, 'recurrence_rule')).find((row) => row.item_id === itemId);
  }

  if (pathname === '/templates') {
    // The container's own, and the ones above it: its hub's, and the workspace's. The tree is
    // the containers', walked up by parent_id.
    const containerId = query.get('container_id');
    const above = new Set<string | null>([null]);
    if (containerId) {
      const containers = await documents(storage, 'containers');
      let at: string | undefined = containerId;
      while (at) {
        above.add(at);
        const container = containers.find((c) => c.id === at);
        at = typeof container?.parent_id === 'string' ? container.parent_id : undefined;
      }
    }
    const rows = (await documents(storage, 'template'))
      .filter((row) => !row.deleted_at && (!containerId || above.has((row.scope_id ?? null) as string | null)));
    return page(rows);
  }

  return undefined;
}

// ---------------------------------------------------------------------------------------------
// The other direction (F6-05): what a write becomes when it has to be queued.
// ---------------------------------------------------------------------------------------------

/**
 * `mutationFor` maps a write - its method, path and body, as `mutate` was called - to the
 * mutation `:push` takes, without its clocks (the engine stamps them), or to `undefined` for a
 * write that cannot be made offline and goes straight to the server as before. The seven kinds
 * (`offline-sync.md` §3.2):
 *
 * - `POST /items` is an `ITEM_CREATE` with an identifier the client mints (§9.1: final). The
 *   direct path keeps the server's identifier, because a direct create is not a mutation.
 * - `PATCH /items/{id}`, `:complete`, `:reopen`, `:assign`, `:unassign`, the due date set or
 *   cleared, and a custom field written are an `ITEM_PATCH`, one clock per field.
 * - `:move` and `:reorder` are a `MOVE`: the parent, and the rank as a key minted between the
 *   neighbours the copy knows - the server's own scheme (`ordering.ts`), so a rank chosen offline
 *   is a rank the server would have chosen.
 * - a label, a member or an attachment put or deleted is a `SET_ADD` or `SET_REMOVE`.
 * - `DELETE /items/{id}` is an `ITEM_DELETE`; `POST /items/{id}/comments` a `COMMENT_ADD` with a
 *   comment identifier the client mints.
 *
 * Everything else answers `undefined`, and the test says which and why: archiving and restoring
 * change lifecycle the server owns; the cover, a bulk and a duplicate have no mutation kind;
 * containers, labels' definitions, templates and views are §1's structure the queue does not
 * carry; and the administration's writes are §1's right column.
 */
export async function mutationFor(
  method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
  path: string,
  body: unknown,
  helpers: { readonly storage: Storage; readonly mintId: () => string },
): Promise<QueuedWrite | undefined> {
  const { pathname } = parse(path);
  const fields = (body ?? {}) as Record<string, unknown>;

  if (method === 'POST' && pathname === '/items') {
    return { kind: 'ITEM_CREATE', itemId: helpers.mintId(), payload: fields };
  }

  let match = /^\/items\/([^/:]+)$/.exec(pathname);
  if (match?.[1]) {
    if (method === 'PATCH') return { kind: 'ITEM_PATCH', itemId: match[1], fields };
    if (method === 'DELETE') return { kind: 'ITEM_DELETE', itemId: match[1] };
    return undefined;
  }

  match = /^\/items\/([^/:]+):(complete|reopen|assign|unassign|move|reorder)$/.exec(pathname);
  if (match?.[1] && match[2]) {
    const itemId = match[1];
    switch (match[2]) {
      case 'complete':
        return { kind: 'ITEM_PATCH', itemId, fields: { completion: { is_completed: true } } };
      case 'reopen':
        return { kind: 'ITEM_PATCH', itemId, fields: { completion: { is_completed: false } } };
      case 'assign':
        // The body names the account (`Assignment.account_id`); the field it sets on the entry
        // is `assignee_id` (issue 876).
        return typeof fields.account_id === 'string'
          ? { kind: 'ITEM_PATCH', itemId, fields: { assignee_id: fields.account_id } }
          : undefined;
      case 'unassign':
        return { kind: 'ITEM_PATCH', itemId, fields: { assignee_id: null } };
      case 'move':
      case 'reorder':
        return moveOf(itemId, match[2], fields, helpers.storage);
      default:
        return undefined;
    }
  }

  match = /^\/items\/([^/]+)\/due$/.exec(pathname);
  if (match?.[1]) {
    if (method === 'PUT') {
      return { kind: 'ITEM_PATCH', itemId: match[1], fields: {
        due_at: fields.due_at ?? null,
        due_date_only: fields.due_date_only ?? false,
        due_time_zone: fields.due_time_zone ?? null,
      } };
    }
    if (method === 'DELETE') return { kind: 'ITEM_PATCH', itemId: match[1], fields: { due_at: null } };
    return undefined;
  }

  match = /^\/items\/([^/]+)\/custom-fields\/([^/]+)$/.exec(pathname);
  if (match?.[1] && match[2] && method === 'PUT') {
    return { kind: 'ITEM_PATCH', itemId: match[1], fields: { [`custom_fields.${decodeURIComponent(match[2])}`]: fields.value ?? null } };
  }

  match = /^\/items\/([^/]+)\/(labels|members|attachments)\/([^/]+)$/.exec(pathname);
  if (match?.[1] && match[2] && match[3] && (method === 'PUT' || method === 'DELETE')) {
    return { kind: method === 'PUT' ? 'SET_ADD' : 'SET_REMOVE', itemId: match[1], set: match[2] as 'labels' | 'members' | 'attachments', element: match[3] };
  }

  match = /^\/items\/([^/]+)\/comments$/.exec(pathname);
  if (match?.[1] && method === 'POST') {
    return { kind: 'COMMENT_ADD', itemId: match[1], payload: { ...fields, id: helpers.mintId() } };
  }

  return undefined;
}

/**
 * A move or a reorder as a `MOVE`: where the entry goes, and the rank it takes there. The rank
 * is minted between the neighbours the copy knows at the destination - the entry to go before,
 * and whatever sits above it - with the server's own scheme, so that two devices inserting into
 * one list keep both insertions (§4.2). A destination the copy does not hold cannot name a rank,
 * and the write then goes directly.
 *
 * The payload always names the collection (issue 777): the server's applier requires a destination -
 * a collection or a parent - and a reorder at a collection's top level is a move to the same
 * place, which only the collection can name. A move names the collection it was asked for; a
 * reorder names the one the entry is in, which its parent's, where it has one, agrees with.
 */
async function moveOf(
  itemId: string, verb: 'move' | 'reorder', body: Record<string, unknown>, storage: Storage,
): Promise<QueuedWrite | undefined> {
  const moving = await one(storage, 'items', itemId);
  if (!moving) return undefined;
  const parentId = verb === 'move'
    ? (typeof body.target_parent_id === 'string' ? body.target_parent_id : null)
    : ((moving.parent_id as string | null | undefined) ?? null);
  const collectionId = verb === 'move' && typeof body.target_collection_id === 'string'
    ? body.target_collection_id
    : (moving.collection_id as string | undefined);
  if (!collectionId) return undefined;

  const siblings = (await level(storage, parentId ? { item_id: parentId } : { container_id: collectionId }))
    .filter((row) => row.id !== itemId);
  const beforeId = typeof body.before_item_id === 'string' ? body.before_item_id : null;
  let previous = '';
  let next = '';
  if (beforeId) {
    const at = siblings.findIndex((row) => row.id === beforeId);
    if (at < 0) return undefined;
    next = String(siblings[at]?.order_key ?? '');
    previous = at > 0 ? String(siblings[at - 1]?.order_key ?? '') : '';
  } else {
    previous = siblings.length > 0 ? String(siblings[siblings.length - 1]?.order_key ?? '') : '';
  }
  let orderKey: string;
  try {
    orderKey = orderKeyBetween(previous, next);
  } catch {
    return undefined;
  }
  return { kind: 'MOVE', itemId, payload: { parent_id: parentId, collection_id: collectionId, order_key: orderKey } };
}
