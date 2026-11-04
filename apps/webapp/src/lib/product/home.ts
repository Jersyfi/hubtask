// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a product's workspace-wide home holds (user-experience.md §2.2, §5.1): its views, the hub
 * screen each hub row opens. Kept out
 * of the view so the one anatomy is the same for every product and a test reads it whole.
 */

import { PRODUCTS, type ProductRow } from '../navigation.ts';

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
 * A product home's create act (UX-2, user-experience.md §5.1): its words, and the words of the
 * menu choosing another hub. It leads to the hub's screen of the product, where the act itself
 * stands, in the hub's own words; the desk words its own (`DeskHome`), time and goals create on
 * their own pages. The plans have no act of making at a hub to lead to, so none here.
 */
export function createOf(module: string): { readonly code: string; readonly inCode: string } | undefined {
  switch (module) {
    case 'KNOWLEDGE':
      return { code: 'app.pages.new', inCode: 'app.product_home.new_in' };
    case 'WHITEBOARD':
      return { code: 'app.whiteboards.new', inCode: 'app.product_home.new_in' };
    case 'ASSETS':
      return { code: 'app.assets.new', inCode: 'app.product_home.new_in' };
    default:
      return undefined;
  }
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
