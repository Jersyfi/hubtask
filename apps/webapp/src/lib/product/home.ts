// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a product's workspace-wide home holds (user-experience.md §2.2, §5.1): its views, the hub
 * screen each hub row opens. Kept out
 * of the view so the one anatomy is the same for every product and a test reads it whole.
 */

import { PRODUCTS, type ProductRow } from '../navigation.ts';
import { modulesHere } from '../data/modules.ts';

export interface HomeTab {
  readonly id: string;
  readonly code: string;
  readonly path: string;
}

/** A home's views where it has several; one view draws no strip. */
export function homeTabs(module: string): readonly HomeTab[] {
  if (module === 'SERVICE_DESK') {
    // Settings last (§5.1): the desk is set up at its home (§7.4).
    return [
      { id: 'queues', code: 'app.desk_queues.label', path: '/desk' },
      { id: 'desks', code: 'app.desk_queues.desks', path: '/desk/desks' },
      { id: 'settings', code: 'app.product_home.tab_settings', path: '/desk/settings' },
    ];
  }
  if (module === 'AGENTS') {
    // The agents' one screen (§7.8): the cards, their held acts, their runs, then their settings.
    return [
      { id: 'agents', code: 'app.agents.tab_agents', path: '/agents' },
      { id: 'requests', code: 'app.agents.tab_requests', path: '/agents?tab=requests' },
      { id: 'runs', code: 'app.agents.tab_runs', path: '/agents?tab=runs' },
      { id: 'settings', code: 'app.product_home.tab_settings', path: '/agents?tab=settings' },
    ];
  }
  if (module === 'ASSETS') {
    // Settings last (§5.1, §8.2): the classes, relations and sources are set up at the home.
    return [
      { id: 'inventory', code: 'app.product_home.tab_inventory', path: '/assets' },
      { id: 'settings', code: 'app.product_home.tab_settings', path: '/assets/settings' },
    ];
  }
  if (module === 'KNOWLEDGE') {
    return [
      { id: 'spaces', code: 'app.product_home.tab_spaces', path: '/pages' },
      { id: 'notes', code: 'app.nav.notes', path: '/notes' },
    ];
  }
  return [];
}

/**
 * The hub's own screen of a product, which the home's hub row opens; nothing for a product with no
 * screen per hub.
 */
export function hubScreenOf(module: string, hubId: string): string | undefined {
  const at = `/hubs/${encodeURIComponent(hubId)}`;
  switch (module) {
    case 'PLANNING':
      return `${at}/plan`;
    case 'KNOWLEDGE':
      return `${at}/pages`;
    case 'WHITEBOARD':
      return `${at}/whiteboards`;
    case 'ASSETS':
      return `${at}/assets`;
    case 'SERVICE_DESK':
      return `${at}/desk`;
    default:
      return undefined;
  }
}

/**
 * A product home's create act (UX-2, user-experience.md §5.1): its words, the words of the menu
 * choosing another place, and the level of that place. It leads to the place's screen of the
 * product, where the act itself stands, in the place's own words; the desk words its own
 * (`DeskHome`), time and goals create on their own pages, the agents' home is their one screen
 * with *Add an agent* in its head. The plans lead to a collection's sprints, whose head is *Plan a
 * sprint* (§7.5) - a hub's plan holds its entries' dates and has no act of making.
 */
export function createOf(module: string): { readonly code: string; readonly inCode: string; readonly level: 'HUB' | 'COLLECTION' } | undefined {
  switch (module) {
    case 'PLANNING':
      return { code: 'app.sprints.plan', inCode: 'app.product_home.plan_in', level: 'COLLECTION' };
    case 'KNOWLEDGE':
      return { code: 'app.pages.new', inCode: 'app.product_home.new_in', level: 'HUB' };
    case 'WHITEBOARD':
      return { code: 'app.whiteboards.new', inCode: 'app.product_home.new_in', level: 'HUB' };
    case 'ASSETS':
      return { code: 'app.assets.new', inCode: 'app.product_home.new_in', level: 'HUB' };
    default:
      return undefined;
  }
}

/** The query a place's screen opens its create dialog on, once, as the home's create act asks. */
export const CREATE_ASKED = 'new';

/**
 * Where a home's create act goes (§5.2): the place's screen of the product with its create dialog
 * open, so the act creates rather than lists; undefined for a product without a create act.
 */
export function createAt(module: string, placeId: string): string | undefined {
  const act = createOf(module);
  if (!act) return undefined;
  const screen = act.level === 'COLLECTION' ? collectionScreenOf(module, placeId) : hubScreenOf(module, placeId);
  return screen ? `${screen}?${CREATE_ASKED}=1` : undefined;
}

/**
 * The places a home's create act offers, the one holding most of the product first (§5.2): the
 * primary creates where the product is used, not in whichever hub sorts first - an empty first hub
 * beside one holding the inventory would open on nothing. Ties, and places not counted yet, keep
 * their order.
 */
export function placesByUse<P extends { readonly id: string }>(places: readonly P[], counts: Readonly<Record<string, number>> | undefined): P[] {
  if (!counts) return [...places];
  return places
    .map((place, at) => ({ place, at, n: counts[place.id] ?? 0 }))
    .sort((a, b) => b.n - a.n || a.at - b.at)
    .map(({ place }) => place);
}

/** The screen a create act at a collection leads to: the product's screen there. */
export function collectionScreenOf(module: string, collectionId: string): string | undefined {
  return module === 'PLANNING' ? `/collections/${encodeURIComponent(collectionId)}/sprints` : undefined;
}

/**
 * Whether nothing of a product exists yet in the hubs where it is on (user-experience.md §4): the
 * first visit, which offers the product's create act and templates. `counts` holds how many of
 * the product's objects each hub answered; `undefined` while a hub's read is out.
 */
export function isUnused(counts: Readonly<Record<string, number>>, hubIds: readonly string[]): boolean | undefined {
  if (!hubIds.every((hubId) => counts[hubId] !== undefined)) return undefined;
  return hubIds.every((hubId) => counts[hubId] === 0);
}

/**
 * *More in Hubtask* (user-experience.md §3.3): the products switched off, each with what it does,
 * in the products band's order. Offered only to a holder of `STRUCTURE` on the workspace, who can
 * switch one on; nobody else is shown a product they cannot have.
 */
export function moreInHubtask(options: { readonly modulesOn: readonly string[] | undefined; readonly mayShape: boolean }): readonly ProductRow[] {
  if (!options.mayShape || options.modulesOn === undefined) return [];
  const on = options.modulesOn;
  return PRODUCTS.filter((row) => !on.includes(row.module));
}

/**
 * The boards of the hubs where whiteboards are on, as one list, the last changed first (§2.2),
 * at most `limit`. Only the hubs named: a hub whose module went off keeps no row here.
 */
export function recentBoards<B extends { readonly hub_id: string; readonly updated_at: string }>(
  lists: Readonly<Record<string, readonly B[]>>,
  hubIds: readonly string[],
  limit: number,
): B[] {
  return hubIds
    .flatMap((hubId) => lists[hubId] ?? [])
    .sort((a, b) => (a.updated_at < b.updated_at ? 1 : a.updated_at > b.updated_at ? -1 : 0))
    .slice(0, limit);
}

/** The agents' recent runs (§7.8), the newest first across the agents listed, at most `limit`. */
export function recentRuns<J extends { readonly started_at: string }>(
  jobsAt: Readonly<Record<string, readonly J[]>>,
  agentIds: readonly string[],
  limit: number,
): J[] {
  return agentIds
    .flatMap((id) => jobsAt[id] ?? [])
    .sort((a, b) => (a.started_at < b.started_at ? 1 : a.started_at > b.started_at ? -1 : 0))
    .slice(0, limit);
}

/** What the pages' home reads of a page: a tree row of `GET /hubs/{id}/pages`. */
export interface PageRow {
  readonly hub_id: string;
  readonly updated_at: string;
  readonly owner_id: string | null;
  readonly template: boolean;
  readonly archived: boolean;
  readonly due_for_review: boolean;
  readonly trashed_at: string | null;
  readonly daily_for: string | null;
}

// A template is a start, an archived or trashed page is put away, and a daily note is somebody's
// own day: none of them is a page somebody reads as the space's content.
const isContent = (page: PageRow) => !page.template && !page.archived && page.trashed_at === null && page.daily_for === null;

/** The pages of the hubs where pages are on, the last changed first, at most `limit` (§2.2). */
export function recentPages<P extends PageRow>(lists: Readonly<Record<string, readonly P[]>>, hubIds: readonly string[], limit: number): P[] {
  const content = Object.fromEntries(hubIds.map((hubId) => [hubId, (lists[hubId] ?? []).filter(isContent)]));
  return recentBoards(content, hubIds, limit);
}

/**
 * How many of a hub's pages changed on or after `from` (§7.9's *3 pages changed this week*), the
 * day each changed read in the reader's zone; the space's content only, as the home lists it.
 */
export function pagesChangedSince(pages: readonly PageRow[], from: string, dayOf: (iso: string) => string | undefined): number {
  return pages.filter((page) => {
    const day = isContent(page) ? dayOf(page.updated_at) : undefined;
    return day !== undefined && day >= from;
  }).length;
}

/**
 * The pages waiting for the reader's review (§2.2): those the reader answers for whose
 * verification has run out (UC-DOC's `due_for_review`), the longest unchanged first.
 */
export function pagesToReview<P extends PageRow>(lists: Readonly<Record<string, readonly P[]>>, hubIds: readonly string[], viewerId: string | undefined): P[] {
  if (viewerId === undefined) return [];
  return hubIds
    .flatMap((hubId) => lists[hubId] ?? [])
    .filter((page) => isContent(page) && page.due_for_review && page.owner_id === viewerId)
    .sort((a, b) => (a.updated_at < b.updated_at ? -1 : a.updated_at > b.updated_at ? 1 : 0));
}

/** The inventory's summary across hubs (§2.2): configuration items in all and those found wanting. */
export interface InventorySummary {
  readonly total: number;
  readonly wanting: number;
  readonly reviewOverdue: number;
  /** Whether every hub named has answered, so a sum is not shown as the whole while one is read. */
  readonly isWhole: boolean;
}

export function inventorySummary(
  healths: Readonly<Record<string, { readonly total: number; readonly review_overdue: number; readonly problems: readonly unknown[] }>>,
  hubIds: readonly string[],
): InventorySummary {
  let total = 0;
  let wanting = 0;
  let reviewOverdue = 0;
  for (const hubId of hubIds) {
    const health = healths[hubId];
    if (!health) continue;
    total += health.total;
    wanting += health.problems.length;
    reviewOverdue += health.review_overdue;
  }
  return { total, wanting, reviewOverdue, isWhole: hubIds.every((hubId) => healths[hubId] !== undefined) };
}

/** One class of the inventory across hubs: how many CIs of it, and in which hubs, most first. */
export interface ClassHeld {
  readonly class: string;
  readonly total: number;
  readonly hubs: readonly { readonly hubId: string; readonly total: number }[];
}

/**
 * The inventory across hubs by class (§2.2): each class some hub holds, its CIs counted over the
 * hubs, the most held first; a class no hub holds a CI of is left out.
 */
export function inventoryByClass(
  healths: Readonly<Record<string, { readonly classes: readonly { readonly class: string; readonly total: number }[] }>>,
  hubIds: readonly string[],
): ClassHeld[] {
  const byClass = new Map<string, { hubId: string; total: number }[]>();
  for (const hubId of hubIds) {
    for (const each of healths[hubId]?.classes ?? []) {
      if (each.total <= 0) continue;
      byClass.set(each.class, [...(byClass.get(each.class) ?? []), { hubId, total: each.total }]);
    }
  }
  return [...byClass.entries()]
    .map(([key, hubs]) => ({ class: key, total: hubs.reduce((sum, each) => sum + each.total, 0), hubs: [...hubs].sort((a, b) => b.total - a.total) }))
    .sort((a, b) => b.total - a.total || (a.class < b.class ? -1 : 1));
}

/**
 * The releases still to go out across the hubs where planning is on (§2.2), the soonest first,
 * at most `limit`. A day is compared as text: `release_on` is a calendar date, not an instant.
 */
export function releasesDue<R extends { readonly hub_id: string; readonly state: string; readonly release_on: string }>(
  lists: Readonly<Record<string, readonly R[]>>,
  hubIds: readonly string[],
  limit: number,
): R[] {
  return hubIds
    .flatMap((hubId) => lists[hubId] ?? [])
    .filter((release) => release.state === 'PLANNED')
    .sort((a, b) => (a.release_on < b.release_on ? -1 : a.release_on > b.release_on ? 1 : 0))
    .slice(0, limit);
}

/** A hub's next release still to go out, the soonest, for its row on the plans' home (§2.2). */
export function nextReleaseOf<R extends { readonly hub_id: string; readonly state: string; readonly release_on: string }>(
  lists: Readonly<Record<string, readonly R[]>>,
  hubId: string,
): R | undefined {
  return releasesDue({ [hubId]: lists[hubId] ?? [] }, [hubId], 1)[0];
}

/**
 * The sprints running in the given collections (§2.2, §7.9), the soonest to end first, at most
 * `limit`. A day is compared as text: `end_on` is a calendar date.
 */
export function sprintsRunning<S extends { readonly collection_id: string; readonly state: string; readonly end_on: string }>(
  lists: Readonly<Record<string, readonly S[]>>,
  collectionIds: readonly string[],
  limit: number,
): S[] {
  return collectionIds
    .flatMap((id) => lists[id] ?? [])
    .filter((sprint) => sprint.state === 'ACTIVE')
    .sort((a, b) => (a.end_on < b.end_on ? -1 : a.end_on > b.end_on ? 1 : 0))
    .slice(0, limit);
}

/**
 * The assets' home lists the hubs that hold an inventory (§2.2): a hub whose health answered none
 * is left out once another holds one; one not answered yet stays, so nothing jumps away later.
 */
export function hubsWithAssetsFirst<H extends { readonly id: string }>(hubs: readonly H[], health: Readonly<Record<string, { readonly total: number }>>): H[] {
  const anyHeld = hubs.some((hub) => (health[hub.id]?.total ?? 0) > 0);
  return anyHeld ? hubs.filter((hub) => (health[hub.id]?.total ?? 1) > 0) : [...hubs];
}

/** A place a create dialog's *In:* chip offers: its id and the words it is named by. */
export interface CreatePlace {
  readonly id: string;
  readonly label: string;
}

/**
 * The places a product's create dialog offers in its *In:* chip (user-experience.md §5.2): the
 * hubs where the product is on - never wider than the workspace's set - or, for a product made at a
 * collection, those hubs' collections, each only where the reader may write. A place whose answer
 * is still out is left out rather than offered, so a choice is never refused after it is made.
 */
export function placesToCreate(options: {
  readonly module: string;
  readonly level: 'HUB' | 'COLLECTION';
  readonly hubs: readonly { readonly id: string; readonly name: string }[];
  readonly workspaceOn: readonly string[] | undefined;
  /** Each hub's own modules, once it has answered. */
  readonly onAt: Readonly<Record<string, readonly string[]>>;
  readonly collectionsOf: (hubId: string) => readonly { readonly id: string; readonly name: string }[];
  /** Whether the reader may write at a place; `undefined` while the answer is out. */
  readonly mayWrite: (path: { readonly hubId: string; readonly collectionId?: string }) => boolean | undefined;
}): CreatePlace[] {
  const { module, level, hubs, workspaceOn, onAt, collectionsOf, mayWrite } = options;
  const on = hubs.filter((hub) => onAt[hub.id] !== undefined && (modulesHere(workspaceOn, onAt[hub.id]) as readonly string[]).includes(module));
  if (level === 'HUB') return on.filter((hub) => mayWrite({ hubId: hub.id }) === true).map((hub) => ({ id: hub.id, label: hub.name }));
  return on.flatMap((hub) =>
    collectionsOf(hub.id)
      .filter((collection) => mayWrite({ hubId: hub.id, collectionId: collection.id }) === true)
      .map((collection) => ({ id: collection.id, label: `${hub.name} › ${collection.name}` })),
  );
}
