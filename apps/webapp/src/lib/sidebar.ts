// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The sidebar's two levels (the navigation concept, user-experience.md §2, UX-1): a fixed top
 * (*For you*), a middle that is the workspace level or one hub's level, and a fixed foot. Every
 * function here is pure; `WorkspaceNav` reads the data and draws what these answer. The hub tools
 * are `navigation.ts`'s, which the palette and the view bars read too. Apart from that file because
 * only the frame, loaded after sign-in, needs it.
 */

import { KEEPING, PRODUCTS, primary, products, workspaceBand, type Destination, type KeepingRow, type ProductRow } from './navigation.ts';

export { HUB_TOOLS, hubTools, switchPath, toolOf, toolPath, type HubTool } from './navigation.ts';

/** Which middle the sidebar draws: the workspace's products and hubs, or one hub's three parts. */
export type NavLevel = { readonly kind: 'workspace' } | { readonly kind: 'hub'; readonly hubId: string };

/** A route as the level is decided from it: its name, its parameters and its query. */
export interface LevelRoute {
  readonly name: string | null;
  readonly params: Readonly<Record<string, string | undefined>>;
  readonly query: Readonly<Record<string, string | undefined>>;
}

/** Routes whose `:id` is a hub. */
const HUB_ROUTES = new Set([
  'hub', 'hub-pages', 'hub-contents', 'hub-desk', 'hub-plan', 'hub-plan-releases', 'hub-plan-sprints', 'hub-inventory', 'hub-inventory-section',
  'hub-time', 'hub-time-workload', 'hub-time-budget', 'hub-goals', 'hub-goals-dashboards', 'hub-settings', 'hub-settings-section',
]);
/** Routes whose `:id` is a collection. */
const COLLECTION_ROUTES = new Set(['collection', 'collection-timeline', 'collection-sprint', 'collection-backlog', 'collection-time', 'collection-workload', 'collection-settings', 'collection-settings-section']);
/** Routes whose place the screen names once it has read it (`page.locate`): an entry, a page, a board, a CI. */
const LOCATED_ROUTES = new Set(['item', 'page', 'whiteboard', 'ci']);

/**
 * The level a route shows (§2.4, §6.1): anything of a hub - the hub, a tool, a collection, a page,
 * a whiteboard, an entry - shows its hub's level; everything else the workspace level. `hubOf`
 * answers a container's hub (a hub's is itself); `located` is the container the screen named.
 */
export function levelOf(route: LevelRoute, hubOf: (containerId: string) => string | undefined, located?: string): NavLevel {
  const name = route.name ?? '';
  let hubId: string | undefined;
  if (HUB_ROUTES.has(name)) hubId = route.params['id'];
  else if (COLLECTION_ROUTES.has(name)) hubId = route.params['id'] ? hubOf(route.params['id']) : undefined;
  else if (LOCATED_ROUTES.has(name)) hubId = located ? hubOf(located) : undefined;
  return hubId ? { kind: 'hub', hubId } : { kind: 'workspace' };
}

/**
 * The fixed top (§2.2): `places` without Search, which is the palette's field, and without Capture.
 * `besideBar` on `compact`, where the bottom bar carries *Today* and *Inbox*: only what it does not.
 */
export function forYou(options: { readonly hasJumble: boolean; readonly modulesOn?: readonly string[]; readonly besideBar?: boolean }): readonly Destination[] {
  return primary({ hasJumble: options.hasJumble, modulesOn: options.modulesOn })
    .filter((destination) => destination.id !== 'search' && (!options.besideBar || destination.isOffBar === true));
}

/** The fixed foot (§2.5): *Administration* where the reader reaches it, then *Archive* and *Trash*. */
export function foot(options: { readonly isAdministrationReachable: boolean }): readonly KeepingRow[] {
  return [...workspaceBand(options), ...KEEPING];
}

/**
 * The *Products* section (§2.3): the products switched on, in the person's order (`nav_products`,
 * the default order after it), the hidden ones (`nav_products_hidden`) behind *More*.
 */
export function navProducts(options: {
  readonly modulesOn?: readonly string[];
  readonly order?: readonly string[] | null;
  readonly hidden?: readonly string[] | null;
}): { readonly shown: readonly ProductRow[]; readonly more: readonly ProductRow[] } {
  const on = products({ modulesOn: options.modulesOn });
  const order = options.order ?? [];
  const rank = (row: ProductRow) => {
    const at = order.indexOf(row.module);
    return at === -1 ? order.length + PRODUCTS.indexOf(row) : at;
  };
  const sorted = [...on].sort((a, b) => rank(a) - rank(b));
  const hidden = new Set(options.hidden ?? []);
  return { shown: sorted.filter((row) => !hidden.has(row.module)), more: sorted.filter((row) => hidden.has(row.module)) };
}

/**
 * A row's count as *For you* shows it (§2.2): nothing for none, so a row never says "0"; "n+"
 * where the read said it goes on, because its count is then one page's.
 */
export function figureCount(count: number, hasMore = false): string | undefined {
  if (count <= 0) return undefined;
  return hasMore ? `${count}+` : String(count);
}

/** How many hubs the workspace level shows before *n more* (§2.3), and from how many rows a section filters. */
export const HUBS_SHOWN = 12;
export const COLLECTIONS_SHOWN = 15;
export const COLLECTIONS_FILTER_FROM = 20;
export const CONTENTS_SHOWN = 15;
export const CONTENTS_FILTER_FROM = 30;
export const QUEUES_SHOWN = 8;

export type HubsOrder = 'MANUAL' | 'RECENT' | 'NAME';

/**
 * The hubs in the person's order (§2.3): the shared order as given, the recently visited first
 * (the rest after them in the shared order), or A–Z.
 */
export function orderHubs<T extends { readonly id: string; readonly name: string }>(
  hubs: readonly T[],
  mode: HubsOrder | null | undefined,
  visited: readonly string[] = [],
  locale?: string,
): readonly T[] {
  if (mode === 'NAME') return [...hubs].sort((a, b) => a.name.localeCompare(b.name, locale));
  if (mode === 'RECENT') {
    const rank = (hub: T) => {
      const at = visited.indexOf(hub.id);
      return at === -1 ? visited.length : at;
    };
    return hubs.map((hub, index) => ({ hub, index })).sort((a, b) => rank(a.hub) - rank(b.hub) || a.index - b.index).map((each) => each.hub);
  }
  return hubs;
}

/** A queue as the sidebar draws it (§2.4, §7.4). */
export interface QueueRow {
  /** The saved view's id, or `std:<key>` for a standard queue. */
  readonly id: string;
  readonly viewId: string | null;
  readonly key: string;
  /** The view's name, or nothing for a standard queue, which the client names by its key. */
  readonly name: string | null;
  readonly count: number | null;
  /** Whether it holds an entry about to breach or breached: its count on the danger surface. */
  readonly isUrgent: boolean;
  readonly isStarred: boolean;
}

/** The hub's queues: the starred ones first (the person's), then the hub's in their saved order. */
export function queueRows(
  queues: readonly { readonly view_id: string | null; readonly key: string; readonly name: string | null; readonly count: number | null; readonly at_risk: number | null }[],
  starredViews: readonly string[],
): readonly QueueRow[] {
  const rows = queues.map((queue) => ({
    id: queue.view_id ?? `std:${queue.key}`,
    viewId: queue.view_id,
    key: queue.key,
    name: queue.name,
    count: queue.count,
    isUrgent: (queue.at_risk ?? 0) > 0,
    isStarred: queue.view_id !== null && starredViews.includes(queue.view_id),
  }));
  const starred = starredViews.map((id) => rows.find((row) => row.viewId === id)).filter((row): row is QueueRow => row !== undefined);
  return [...starred, ...rows.filter((row) => !row.isStarred)];
}

/** Where a queue opens: the queue page of the hub, the queue named. */
export const queuePath = (hubId: string, row: Pick<QueueRow, 'viewId' | 'key'>): string =>
  `/hubs/${hubId}/desk?queue=${encodeURIComponent(row.viewId ?? row.key)}`;

/** A page or a whiteboard in the content tree (§2.4 part 3). */
export interface ContentNode {
  readonly id: string;
  readonly kind: 'page' | 'whiteboard';
  readonly title: string;
  readonly children: readonly ContentNode[];
}

export type ContentsMode = 'TREE' | 'RECENT' | 'NAME';

interface PageInput {
  readonly id: string;
  readonly parent_id: string | null;
  readonly title: string;
  readonly order_key?: string;
  readonly template?: boolean;
  readonly updated_at?: string;
}
interface BoardInput {
  readonly id: string;
  readonly title: string;
  readonly parent_page_id?: string | null;
  readonly order_key?: string | null;
  readonly updated_at?: string;
}

/**
 * The hub's pages and whiteboards as one tree (§2.4 part 3, §7.2). In `TREE` the siblings stand by
 * their shared `order_key`; a board without one (written before the change) stands after the keyed
 * siblings, A–Z, until somebody files it; a board under a page the reader does not see stands at the
 * top. Templates are not in the tree. `RECENT` and `NAME` are flat lists.
 */
export function contentsTree(pages: readonly PageInput[], boards: readonly BoardInput[], mode: ContentsMode | null | undefined = 'TREE', locale?: string): readonly ContentNode[] {
  const shownPages = pages.filter((page) => page.template !== true);
  const byName = (a: { title: string }, b: { title: string }) => a.title.localeCompare(b.title, locale);
  if (mode === 'RECENT' || mode === 'NAME') {
    const all = [
      ...shownPages.map((page) => ({ id: page.id, kind: 'page' as const, title: page.title, at: page.updated_at ?? '' })),
      ...boards.map((board) => ({ id: board.id, kind: 'whiteboard' as const, title: board.title, at: board.updated_at ?? '' })),
    ];
    const sorted = mode === 'NAME' ? all.sort(byName) : all.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));
    return sorted.map(({ id, kind, title }) => ({ id, kind, title, children: [] }));
  }
  const pageIds = new Set(shownPages.map((page) => page.id));
  type Entry = { readonly node: { id: string; kind: 'page' | 'whiteboard'; title: string }; readonly key: string | null };
  const under = new Map<string, Entry[]>();
  const put = (parent: string | null | undefined, entry: Entry) => {
    const at = parent && pageIds.has(parent) ? parent : '';
    under.set(at, [...(under.get(at) ?? []), entry]);
  };
  for (const page of shownPages) put(page.parent_id, { node: { id: page.id, kind: 'page', title: page.title }, key: page.order_key ?? null });
  for (const board of boards) put(board.parent_page_id, { node: { id: board.id, kind: 'whiteboard', title: board.title }, key: board.order_key ?? null });
  const build = (parent: string, seen: Set<string>): ContentNode[] =>
    [...(under.get(parent) ?? [])]
      .sort((a, b) => {
        if (a.key !== null && b.key !== null) return a.key < b.key ? -1 : a.key > b.key ? 1 : 0;
        if (a.key !== null) return -1;
        if (b.key !== null) return 1;
        return byName(a.node, b.node);
      })
      .map(({ node }) => ({
        ...node,
        children: node.kind === 'page' && !seen.has(node.id) ? build(node.id, new Set([...seen, node.id])) : [],
      }));
  return build('', new Set());
}

/** How many pages and boards a tree holds, for the section's count (*Contents · 131*). */
export function contentsCount(nodes: readonly ContentNode[]): number {
  return nodes.reduce((sum, node) => sum + 1 + contentsCount(node.children), 0);
}


/**
 * Where a row lands among flat siblings when it is placed after `afterId` (null: first) - the
 * index `:reorder` and `:move` take for a hub or a collection.
 */
export function placedIndex(ids: readonly string[], id: string, afterId: string | null): number {
  const rest = ids.filter((each) => each !== id);
  return afterId === null ? 0 : rest.indexOf(afterId) + 1;
}
