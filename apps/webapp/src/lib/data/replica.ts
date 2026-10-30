// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the replica answers while the server cannot be reached, in this application's own
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

import { orderKeyBetween, type QueuedWrite, type ResourceRequest, type Storage, type StoredRecord, type VocabularyEntity } from '@hubtask/sync-engine';

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
  const rows = held.map((record) => withSets(record));
  return collection === 'items' ? marked(storage, rows) : rows;
}

// ---------------------------------------------------------------------------------------------
// What a collection hides of an entry (domain-model.md §2, "Narrowing hides, it never deletes").
// ---------------------------------------------------------------------------------------------

/** One type as the installation offers it: `/meta/capabilities`' `item_types`. */
export interface BoundType {
  readonly type?: string;
  readonly capabilities?: readonly string[];
}

/** A container's structure in force, as far as the mark reads it: its levels and fields. */
export interface StructureShape {
  readonly levels?: readonly { readonly type?: string }[];
  readonly fields?: Readonly<Record<string, { readonly capabilities?: readonly string[] } | undefined>>;
}

/** `WorkItem.hidden`, as the server answers it on every read the replica stands in for. */
export interface EntryHidden {
  readonly level: boolean;
  readonly capabilities: string[];
  /** How many live entries under a shown entry the collection hides; set by `marked`. */
  readonly below?: number;
}

/** An entry as `belowOf` reads it: its type, its parent and whether it is live. */
export interface HeldEntry {
  readonly id: string;
  readonly type: string;
  readonly parent_id?: string | null;
  readonly deleted_at?: string | null;
  readonly archived_at?: string | null;
}

/**
 * How many live entries under a shown entry the collection hides (UC-WRK-24 check 4): every one
 * under it whose own level, or a level between, is not offered - the server's count
 * (`EntryGate.countBelow`), held to the same table (`below` in `api/fixtures/hidden-entries.json`).
 * Zero for an entry hidden itself; its whole subtree is hidden with it.
 */
export function belowOf(
  entry: HeldEntry,
  ancestors: readonly string[],
  children: ReadonlyMap<string, readonly HeldEntry[]>,
  structure: StructureShape,
  types: readonly BoundType[],
): number {
  if (hiddenOf(entry.type, ancestors, structure, types).level) return 0;
  const held = new Set(types.map((row) => row.type));
  const levels = new Set((structure.levels ?? []).map((level) => level.type));
  const hides = (type: string) => held.has(type) && !levels.has(type);
  let count = 0;
  const walk = (id: string, hidden: boolean, depth: number): void => {
    if (depth > 16) return;
    for (const child of children.get(id) ?? []) {
      if (child.deleted_at || child.archived_at) continue;
      const under = hidden || hides(child.type);
      if (under) count += 1;
      walk(child.id, under, depth + 1);
    }
  };
  walk(entry.id, false, 0);
  return count;
}

/**
 * The installation's types, handed over by `capabilities.svelte.ts` when the manifest arrives.
 * Until then no mark is answered: before the manifest nothing is shown as available, and a mark
 * derived from a guess would say something the client cannot know.
 */
let bound: readonly BoundType[] | undefined;

export function useBound(types: readonly BoundType[] | undefined): void {
  bound = types;
}

/**
 * What the collection hides of an entry of this type under these ancestors (nearest first): the
 * server's own rule (`StructureEntries.go`), held to the same table (`api/fixtures/hidden-entries.json`).
 * Sync records carry no mark - a change of a structure would otherwise rewrite every entry under it -
 * so the replica derives it from the container's structure, and the two answers must not drift.
 */
export function hiddenOf(
  type: string,
  ancestors: readonly string[],
  structure: StructureShape,
  types: readonly BoundType[],
): EntryHidden {
  const held = new Map(types.filter((row) => typeof row.type === 'string').map((row) => [row.type as string, row]));
  const levels = new Set((structure.levels ?? []).map((level) => level.type));
  const level = [type, ...ancestors].some((each) => held.has(each) && !levels.has(each));
  const offered = structure.fields?.[type]?.capabilities;
  const capabilities = offered === undefined
    ? []
    : (held.get(type)?.capabilities ?? []).filter((capability) => capability !== 'COMPLETION' && !offered.includes(capability));
  return { level, capabilities };
}

/** The structure in force a container's record carries, or `undefined` for one from before it. */
function structureOf(container: Document | undefined): StructureShape | undefined {
  const structure = container?.structure as { settings?: StructureShape } | undefined;
  return structure?.settings?.levels ? structure.settings : undefined;
}

/**
 * Asks for the fresh snapshot `offline-sync.md` §4.2 promises a copy taken before the server
 * answered `structure`: where the manifest announces `features.structure` and a container is held
 * without one. Without it, a *List* collection read offline would offer what the installation
 * offers, and a write made there would be refused at the push. The engine keeps the "once per
 * device" and lets the request wait while offline; this only says when one is due, and it may say
 * so twice - from the manifest's arrival and from the store's attachment, whichever comes last.
 */
export async function fetchStructureAfresh(
  sync: { readonly storage: Storage | undefined; resynchronise(reason: string): Promise<boolean> },
  features: Readonly<Record<string, unknown>> | undefined,
): Promise<boolean> {
  const storage = sync.storage;
  if (features?.structure !== true || !storage) return false;
  const containers = await storage.all<StoredRecord<Document>>('containers');
  if (!containers.some((held) => held.document?.structure === undefined)) return false;
  return sync.resynchronise('structure');
}

/** The entries with their marks, where the installation and the container's structure are known. */
async function marked(storage: Storage, rows: Document[]): Promise<Document[]> {
  if (!bound || rows.length === 0) return rows;
  const containers = new Map((await storage.all<StoredRecord<Document>>('containers')).map((record) => [record.id, record.document]));
  const items = new Map((await storage.all<StoredRecord<Document>>('items')).map((record) => [record.id, record.document]));
  const types = bound;
  const children = new Map<string, HeldEntry[]>();
  for (const [id, item] of items) {
    if (typeof item.parent_id !== 'string' || typeof item.type !== 'string') continue;
    const siblings = children.get(item.parent_id) ?? [];
    siblings.push({ ...(item as Omit<HeldEntry, 'id'>), id: String(id) });
    children.set(item.parent_id, siblings);
  }
  return rows.map((row) => {
    const structure = structureOf(containers.get(String(row.collection_id)));
    if (!structure || typeof row.type !== 'string') return row;
    const ancestors: string[] = [];
    for (let at = items.get(String(row.parent_id)); at && ancestors.length < 16; at = items.get(String(at.parent_id))) {
      if (typeof at.type === 'string') ancestors.push(at.type);
    }
    const below = belowOf({ id: String(row.id), type: row.type }, ancestors, children, structure, types);
    return { ...row, hidden: { ...hiddenOf(row.type, ancestors, structure, types), below } };
  });
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
  if (!record) return undefined;
  const document = withSets(record);
  return collection === 'items' ? (await marked(storage, [document]))[0] : document;
}

/**
 * The plain level question - or an entry's whole subtree, which the entry page asks for in one
 * read - or nothing for one the copy would have to understand.
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

  // A hub's pages and a page read while the server is away (M3): the tree without the bodies, as
  // the server lists it, and a page with its breadcrumb walked up the copy. Nothing is writable
  // offline - a page is written through its live document, which needs the server.
  match = /^\/hubs\/([^/:]+)\/pages$/.exec(pathname);
  if (match?.[1]) {
    const hubId = match[1];
    const withArchived = query.get('include_archived') === 'true';
    const templatesOnly = query.get('templates') === 'true';
    const rows = (await documents(storage, 'pages'))
      .filter((row) => row.hub_id === hubId && !row.trashed_at && (withArchived || !row.archived) && (!templatesOnly || row.template))
      .map(({ body: _body, ...row }) => row)
      .sort((a, b) => {
        // The server's order: the top of the space first, then by position among siblings.
        const x = `${String(a.parent_id ?? '')}\u0000${String(a.position ?? '')}`;
        const y = `${String(b.parent_id ?? '')}\u0000${String(b.position ?? '')}`;
        return x < y ? -1 : x > y ? 1 : 0;
      });
    return { data: rows };
  }

  match = /^\/pages\/([^/:]+)$/.exec(pathname);
  if (match?.[1]) {
    const held = await documents(storage, 'pages');
    const byId = new Map(held.map((row) => [String(row.id), row]));
    const page = byId.get(match[1]);
    if (!page || page.trashed_at) return undefined;
    const ancestors: { id: string; title: string }[] = [];
    for (let above = page.parent_id; typeof above === 'string' && ancestors.length < 16; ) {
      const parent = byId.get(above);
      if (!parent) break;
      ancestors.unshift({ id: String(parent.id), title: String(parent.title) });
      above = parent.parent_id;
    }
    const until = typeof page.verified_until === 'string' ? Date.parse(page.verified_until) : Number.NaN;
    return { ...page, ancestors, writable: false, due_for_review: Number.isFinite(until) && until <= Date.now() };
  }

  // A hub's inventory read while the server is away (M6, offline-sync.md §3): the CIs in the
  // server's order, a CI with its relations and the protection it inherits computed from the copy
  // as the server computes it, the impact walked the same way, and the catalogue. Nothing is
  // writable offline - a CI is written through the reconciliation, which needs the server.
  const inventory = await cmdbFor(pathname, query, storage);
  if (inventory !== NOT_CMDB) return inventory;

  // A collection's sprints read while the server is away (M2), in the server's order - the latest
  // start first. The burndown and the velocity are computed by the server and wait for it.
  match = /^\/collections\/([^/:]+)\/sprints$/.exec(pathname);
  if (match?.[1]) {
    const collectionId = match[1];
    const state = query.get('state');
    const rows = (await documents(storage, 'sprints'))
      .filter((row) => row.collection_id === collectionId && (!state || row.state === state))
      .sort((a, b) => {
        const x = String(a.start_on ?? '');
        const y = String(b.start_on ?? '');
        if (x !== y) return x < y ? 1 : -1;
        return String(a.id) < String(b.id) ? -1 : 1;
      });
    return { data: rows };
  }

  if (pathname === '/entry-types') {
    // The server's order: by level as the levels run, then position, then key.
    const levels = ['TASK', 'WORK_PACKAGE', 'ACTIVITY'];
    const rank = (row: Document) => levels.indexOf(String(row.level));
    const rows = (await documents(storage, 'entry_types')).sort((a, b) =>
      rank(a) - rank(b) || Number(a.position ?? 0) - Number(b.position ?? 0) || String(a.key).localeCompare(String(b.key)));
    return { data: rows };
  }

  if (pathname === '/link-types') {
    const rows = (await documents(storage, 'link_types')).sort((a, b) =>
      Number(a.position ?? 0) - Number(b.position ?? 0) || String(a.key).localeCompare(String(b.key)));
    return { data: rows };
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
// The CMDB from the copy (M6).
// ---------------------------------------------------------------------------------------------

const NOT_CMDB = Symbol('not a CMDB path');
const LEVELS = ['NORMAL', 'HIGH', 'VERY_HIGH'];
/** The walk's bound, the server's (`impactDepth`). */
const IMPACT_DEPTH = 8;

type Protection = { c: string | null; i: string | null; a: string | null };

const higher = (x: unknown, y: unknown): string | null => {
  const a = LEVELS.indexOf(String(x));
  const b = LEVELS.indexOf(String(y));
  const top = Math.max(a, b);
  return top < 0 ? null : (LEVELS[top] ?? null);
};

/** The relation types' impact by key, split the way the server's walk reads it. */
function suffering(catalogue: Document | undefined): { source: Set<string>; target: Set<string> } {
  const source = new Set<string>();
  const target = new Set<string>();
  for (const t of (catalogue?.relation_types ?? []) as Document[]) {
    if (t.impact === 'SOURCE' || t.impact === 'BOTH') source.add(String(t.key));
    if (t.impact === 'TARGET' || t.impact === 'BOTH') target.add(String(t.key));
  }
  return { source, target };
}

/**
 * The CIs an impact reaches, each at its nearest depth, as `WalkCmdbImpact` answers: downstream,
 * what suffers when the CI fails; upstream, what it suffers from. Breadth first, so the first
 * depth a CI is reached at is its nearest.
 */
export function walkImpact(
  from: string, downstream: boolean, relations: readonly Document[], catalogue: Document | undefined, depth = IMPACT_DEPTH,
): { ci: string; depth: number }[] {
  const { source, target } = suffering(catalogue);
  const reached = new Map<string, number>([[from, 0]]);
  let frontier = [from];
  for (let step = 1; step <= depth && frontier.length > 0; step++) {
    const next: string[] = [];
    for (const at of frontier) {
      for (const r of relations) {
        const type = String(r.type);
        const towards = downstream
          ? (r.target_ci_id === at && source.has(type) ? r.source_ci_id : r.source_ci_id === at && target.has(type) ? r.target_ci_id : undefined)
          : (r.source_ci_id === at && source.has(type) ? r.target_ci_id : r.target_ci_id === at && target.has(type) ? r.source_ci_id : undefined);
        if (typeof towards === 'string' && !reached.has(towards)) {
          reached.set(towards, step);
          next.push(towards);
        }
      }
    }
    frontier = next;
  }
  reached.delete(from);
  return [...reached].map(([ci, d]) => ({ ci, depth: d })).sort((a, b) => a.depth - b.depth || (a.ci < b.ci ? -1 : a.ci > b.ci ? 1 : 0));
}

const active = (ci: Document | undefined) => ci !== undefined && ci.stage !== 'RETIRED' && ci.stage !== 'DISPOSED';

/** The protection a CI inherits by the maximum principle; a dimension fixed keeps its own. */
export function inheritedProtection(ci: Document, reached: readonly Document[]): Protection {
  const own = (ci.protection ?? {}) as Document;
  const effective: Protection = { c: higher(own.c, null), i: higher(own.i, null), a: higher(own.a, null) };
  for (const other of reached) {
    if (!active(other)) continue;
    const p = (other.protection ?? {}) as Document;
    effective.c = higher(effective.c, p.c);
    effective.i = higher(effective.i, p.i);
    effective.a = higher(effective.a, p.a);
  }
  for (const d of (own.fixed ?? []) as string[]) {
    if (d === 'c' || d === 'i' || d === 'a') effective[d] = higher(own[d], null);
  }
  return effective;
}

async function cmdbFor(pathname: string, query: URLSearchParams, storage: Storage): Promise<unknown> {
  if (pathname === '/cmdb/classes' || pathname === '/cmdb/relation-types') {
    const catalogue = (await documents(storage, 'cmdb_catalogue'))[0];
    if (!catalogue) return undefined;
    return { data: pathname === '/cmdb/classes' ? catalogue.classes : catalogue.relation_types };
  }

  let match = /^\/hubs\/([^/:]+)\/cis$/.exec(pathname);
  if (match?.[1]) {
    const hubId = match[1];
    const wanted = query.get('class');
    const withMerged = query.get('include_merged') === 'true';
    const catalogue = (await documents(storage, 'cmdb_catalogue'))[0];
    const lineage = new Map(((catalogue?.classes ?? []) as Document[]).map((c) => [String(c.key), (c.lineage ?? []) as string[]]));
    if (wanted && catalogue && !lineage.has(wanted)) return undefined;
    const rows = (await documents(storage, 'cis'))
      .filter((ci) => ci.hub_id === hubId && (withMerged || !ci.merged_into))
      .filter((ci) => !wanted || ci.class === wanted || (lineage.get(String(ci.class)) ?? []).includes(wanted))
      .sort((a, b) => (String(a.id) < String(b.id) ? -1 : String(a.id) > String(b.id) ? 1 : 0));
    return { data: rows, next: null };
  }

  match = /^\/cis\/([^/:]+)(\/impact)?$/.exec(pathname);
  if (match?.[1]) {
    const id = match[1];
    const cis = await documents(storage, 'cis');
    const byId = new Map(cis.map((ci) => [String(ci.id), ci]));
    const ci = byId.get(id);
    if (!ci) return undefined;
    const relations = await documents(storage, 'ci_relations');
    const catalogue = (await documents(storage, 'cmdb_catalogue'))[0];
    if (match[2]) {
      const downstream = query.get('direction') !== 'UPSTREAM';
      const asked = Number(query.get('depth'));
      const depth = asked > 0 && asked <= IMPACT_DEPTH ? asked : IMPACT_DEPTH;
      const services = new Set(((catalogue?.classes ?? []) as Document[])
        .filter((c) => ((c.lineage ?? []) as string[]).includes('service')).map((c) => String(c.key)));
      const reached = walkImpact(id, downstream, relations, catalogue, depth)
        .map(({ ci: other, depth: d }) => ({ other: byId.get(other), depth: d }))
        .filter((r): r is { other: Document; depth: number } => r.other !== undefined);
      return {
        ci_id: id, direction: downstream ? 'DOWNSTREAM' : 'UPSTREAM',
        reached: reached.map(({ other, depth: d }) => ({ ci_id: other.id, class: other.class, name: other.name, stage: other.stage, depth: d })),
        services: reached.filter(({ other }) => services.has(String(other.class)) && active(other)).map(({ other }) => other.id),
      };
    }
    const reached = walkImpact(id, true, relations, catalogue).map((r) => byId.get(r.ci)).filter((r): r is Document => r !== undefined);
    return {
      ...ci,
      effective_protection: inheritedProtection(ci, reached),
      relations: relations
        .filter((r) => r.source_ci_id === id || r.target_ci_id === id)
        .sort((a, b) => String(a.type).localeCompare(String(b.type)) || (String(a.id) < String(b.id) ? -1 : 1)),
    };
  }

  return NOT_CMDB;
}

// ---------------------------------------------------------------------------------------------
// The other direction: what a write becomes when it has to be queued.
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
 * - an entry type, a link type, a workflow's draft or a custom role created, changed or deleted,
 *   and a link drawn or removed, are `ENTITY_*` mutations (§4.4, `vocabularyOf`).
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
        // is `assignee_id`.
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

  return vocabularyOf(method, pathname, fields, helpers);
}

/** The vocabulary's paths, each the entity its change log names (offline-sync.md §4.4). */
const VOCABULARY: Readonly<Record<string, { entity: VocabularyEntity; collection: string }>> = {
  'entry-types': { entity: 'entry_type', collection: 'entry_types' },
  'link-types': { entity: 'link_type', collection: 'link_types' },
  workflows: { entity: 'workflow', collection: 'workflows' },
  roles: { entity: 'custom_role', collection: 'custom_roles' },
};

/**
 * A write of the workspace's vocabulary as an `ENTITY_*` mutation: a type, a link type, a
 * workflow's draft or a custom role created, changed or deleted, and a link drawn or removed.
 * A versioned entity carries the version the copy holds as `base_version`, so a change made
 * against an older one is refused by the server rather than laid over a newer one. Publishing a
 * workflow, its scheme, a type's form and a role's grants stay online: §4.4 gives them no entity.
 */
async function vocabularyOf(
  method: 'POST' | 'PATCH' | 'PUT' | 'DELETE',
  pathname: string,
  fields: Record<string, unknown>,
  helpers: { readonly storage: Storage; readonly mintId: () => string },
): Promise<QueuedWrite | undefined> {
  let match = /^\/items\/([^/:]+)\/links$/.exec(pathname);
  if (match?.[1] && method === 'POST') {
    const id = helpers.mintId();
    return {
      kind: 'ENTITY_UPSERT', entity: 'entry_link', entityKey: id, itemId: id,
      payload: { ...fields, item_id: match[1] },
    };
  }
  match = /^\/links\/([^/:]+)$/.exec(pathname);
  if (match?.[1] && method === 'DELETE') {
    return { kind: 'ENTITY_DELETE', entity: 'entry_link', entityKey: match[1], itemId: match[1] };
  }

  match = /^\/(entry-types|link-types|workflows|roles)(?:\/([^/:]+))?$/.exec(pathname);
  const vocabulary = match?.[1] ? VOCABULARY[match[1]] : undefined;
  if (!match || !vocabulary) return undefined;
  const key = match[2] === undefined ? (typeof fields.key === 'string' ? fields.key : undefined) : decodeURIComponent(match[2]);
  if (!key) return undefined;
  if (match[2] === undefined && method !== 'POST') return undefined;
  if (match[2] !== undefined && method !== 'PATCH' && method !== 'DELETE') return undefined;

  const held = (await helpers.storage.all<StoredRecord<Document>>(vocabulary.collection))
    .find((record) => record.document.key === key && !record.tombstone);
  if (method === 'DELETE') {
    return held ? { kind: 'ENTITY_DELETE', entity: vocabulary.entity, entityKey: key, itemId: held.id } : undefined;
  }
  if (method === 'POST') {
    return { kind: 'ENTITY_UPSERT', entity: vocabulary.entity, entityKey: key, itemId: held?.id ?? helpers.mintId(), payload: fields };
  }
  // A change of an entity the copy does not hold names no version to be decided against.
  if (!held) return undefined;
  return {
    kind: 'ENTITY_PATCH', entity: vocabulary.entity, entityKey: key, itemId: held.id,
    baseVersion: held.version, payload: fields,
  };
}

/**
 * A move or a reorder as a `MOVE`: where the entry goes, and the rank it takes there. The rank
 * is minted between the neighbours the copy knows at the destination - the entry to go before,
 * and whatever sits above it - with the server's own scheme, so that two devices inserting into
 * one list keep both insertions (§4.2). A destination the copy does not hold cannot name a rank,
 * and the write then goes directly.
 *
 * The payload always names the collection: the server's applier requires a destination -
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
