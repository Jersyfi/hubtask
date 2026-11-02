// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a product's workspace-wide home holds (user-experience.md §2.2, §5.1): its views, the hub
 * screen each hub row opens, and the doors to the workspace-wide screens it already has. Kept out
 * of the view so the one anatomy is the same for every product and a test reads it whole.
 */

import type { IconName } from '@hubtask/design-system/components';

import { PRODUCTS, type ProductRow } from '../navigation.ts';

export interface HomeTab {
  readonly id: string;
  readonly code: string;
  readonly path: string;
}

export interface Door {
  readonly id: string;
  readonly code: string;
  readonly icon: IconName;
  readonly path: string;
}

/** A home's views where it has several; one view draws no strip. */
export function homeTabs(module: string): readonly HomeTab[] {
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
 * screen per hub. The desk lists its queues in the hub's collections, so a hub row opens the hub.
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
      return at;
    default:
      return undefined;
  }
}

/** The workspace-wide screens a product already has, drawn above its hubs. */
export function productDoors(module: string, options: { readonly isAdministrationReachable: boolean }): readonly Door[] {
  switch (module) {
    case 'SERVICE_DESK':
      return [
        { id: 'approvals', code: 'app.nav.approvals', icon: 'user-check', path: '/approvals' },
        ...(options.isAdministrationReachable
          ? [{ id: 'settings', code: 'app.product_home.desk_settings', icon: 'settings' as const, path: '/administration/service-desk' }]
          : []),
      ];
    case 'ASSETS':
      return options.isAdministrationReachable
        ? [{ id: 'settings', code: 'app.product_home.cmdb_settings', icon: 'settings', path: '/administration/configuration-items' }]
        : [];
    case 'AGENTS':
      return [
        { id: 'mine', code: 'app.product_home.agents_mine', icon: 'user', path: '/profile/agents' },
        ...(options.isAdministrationReachable
          ? [{ id: 'all', code: 'app.product_home.agents_all', icon: 'zap' as const, path: '/administration/agents' }]
          : []),
      ];
    default:
      return [];
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

const AGENT_ORDER: Readonly<Record<string, number>> = { ACTIVE: 0, APPROVED: 1, SUSPENDED: 2, DRAFT: 3 };

/**
 * The agents working here (§2.2), live ones first, each with the screen the reader may act on it
 * from: their own on *Agents acting for you*, anybody's in the administration where the reader
 * reaches it, and none otherwise. A retired agent works nowhere and is left out.
 */
export function agentRows<A extends { readonly status: string; readonly owner_id: string | null }>(
  list: readonly A[],
  viewerId: string | undefined,
  isAdministrationReachable: boolean,
): { readonly agent: A; readonly path: string | undefined }[] {
  return list
    .filter((agent) => agent.status !== 'RETIRED')
    .map((agent, index) => ({ agent, index }))
    .sort((a, b) => (AGENT_ORDER[a.agent.status] ?? 9) - (AGENT_ORDER[b.agent.status] ?? 9) || a.index - b.index)
    .map(({ agent }) => ({
      agent,
      path: viewerId !== undefined && agent.owner_id === viewerId ? '/profile/agents' : isAdministrationReachable ? '/administration/agents' : undefined,
    }));
}
