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

import type { ResourceRequest, Storage, StoredRecord } from '@hubtask/sync-engine';

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

/** The plain level question, or nothing for one the copy would have to understand. */
function levelScope(body: unknown): { container_id?: string; item_id?: string; group?: boolean } | undefined {
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
  if (!scope || scope.include_descendants) return undefined;
  const containerId = typeof scope.container_id === 'string' ? scope.container_id : undefined;
  const itemId = typeof scope.item_id === 'string' ? scope.item_id : undefined;
  if (!containerId && !itemId) return undefined;
  return { container_id: containerId, item_id: itemId, group: group !== undefined };
}

/** The entries of one level: a collection's own, or one entry's children, in the manual order. */
async function level(storage: Storage, scope: { container_id?: string; item_id?: string }): Promise<Document[]> {
  const items = await documents(storage, 'items');
  const own = items.filter((item) => {
    if (item.deleted_at) return false;
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
