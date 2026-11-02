// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's and a collection's views as one strip in a fixed order (user-experience.md §2.4), each
 * only where its module is on at that place (UC-INS-21 check 3). The strip, the tree's mirrored
 * rows and the palette read this one list, so a product is never a head button chosen by position.
 */

import type { IconName } from '@hubtask/design-system/components';

export interface PlaceTabInput {
  readonly type: 'HUB' | 'COLLECTION';
  readonly id: string;
  /** The modules on at this place: the workspace's, narrowed by the hub. */
  readonly modulesOn: readonly string[];
  /** Whether the collection acts as a desk (UC-SVC-05): only then is its portal a tab. */
  readonly isDesk?: boolean;
}

export interface PlaceTabEntry {
  readonly id: string;
  readonly code: string;
  readonly icon: IconName;
  readonly href: string;
  /** The views under the tab, where it has more than one (the plan's releases, time's workload). */
  readonly views?: readonly { readonly id: string; readonly code: string; readonly href: string }[];
}

/** The strip's tabs, *Overview* (a hub) or *Entries* (a collection) first and always there. */
export function placeTabs(input: PlaceTabInput): PlaceTabEntry[] {
  const on = (module: string) => input.modulesOn.includes(module);
  if (input.type === 'HUB') {
    const base = `/hubs/${input.id}`;
    return [
      { id: 'overview', code: 'app.place_tabs.overview', icon: 'hub', href: base },
      ...(on('PLANNING')
        ? [{
            id: 'plan', code: 'app.planning.title', icon: 'calendar' as const, href: `${base}/plan`,
            views: [
              { id: 'plan', code: 'app.planning.title', href: `${base}/plan` },
              { id: 'releases', code: 'app.releases.title', href: `${base}/releases` },
            ],
          }]
        : []),
      ...(on('KNOWLEDGE') ? [{ id: 'pages', code: 'app.pages.title', icon: 'book-open' as const, href: `${base}/pages` }] : []),
      ...(on('WHITEBOARD') ? [{ id: 'whiteboards', code: 'app.whiteboards.title', icon: 'layout-template' as const, href: `${base}/whiteboards` }] : []),
      ...(on('SERVICE_DESK') ? [{ id: 'desk', code: 'app.place_tabs.desk', icon: 'inbox' as const, href: `${base}/desk` }] : []),
      ...(on('ASSETS') ? [{ id: 'assets', code: 'app.assets.title', icon: 'shield' as const, href: `${base}/assets` }] : []),
      ...(on('TIME') ? [timeTab(base)] : []),
      ...(on('GOALS')
        ? [{ id: 'goals', code: 'app.goals.title', icon: 'flag' as const, href: `/goals?hub=${input.id}` }]
        : []),
    ];
  }
  const base = `/collections/${input.id}`;
  return [
    { id: 'entries', code: 'app.place_tabs.entries', icon: 'task', href: base },
    ...(on('PLANNING')
      ? [
          { id: 'plan', code: 'app.planning.title', icon: 'calendar' as const, href: `${base}/plan` },
          { id: 'sprints', code: 'app.sprints.title', icon: 'repeat' as const, href: `${base}/sprints` },
        ]
      : []),
    ...(on('SERVICE_DESK') && input.isDesk ? [{ id: 'portal', code: 'app.place_tabs.portal', icon: 'inbox' as const, href: `/portal/${input.id}` }] : []),
    ...(on('TIME') ? [timeTab(base)] : []),
  ];
}

function timeTab(base: string): PlaceTabEntry {
  return {
    id: 'time', code: 'app.time.title', icon: 'clock', href: `${base}/actuals`,
    views: [
      { id: 'actuals', code: 'app.actuals.title', href: `${base}/actuals` },
      { id: 'workload', code: 'app.workload.title', href: `${base}/workload` },
    ],
  };
}

/** The tab and the view a route is on, by its name (`routes.ts`). */
export function placeTabOf(route: string, query: Readonly<Record<string, string | undefined>> = {}): { tab: string; view?: string } | undefined {
  switch (route) {
    case 'hub': return { tab: 'overview' };
    case 'collection': return { tab: 'entries' };
    case 'hub-plan': return { tab: 'plan', view: 'plan' };
    case 'collection-plan': return { tab: 'plan' };
    case 'hub-releases': return { tab: 'plan', view: 'releases' };
    case 'collection-sprints': return { tab: 'sprints' };
    case 'hub-pages': return { tab: 'pages' };
    case 'hub-whiteboards': return { tab: 'whiteboards' };
    case 'hub-desk': return { tab: 'desk' };
    case 'hub-assets':
    case 'hub-assets-section': return { tab: 'assets' };
    case 'hub-actuals':
    case 'collection-actuals': return { tab: 'time', view: 'actuals' };
    case 'hub-workload':
    case 'collection-workload': return { tab: 'time', view: 'workload' };
    // The goals screen draws its own goals, portfolios and dashboards as tabs.
    case 'goals': return query['hub'] ? { tab: 'goals' } : undefined;
    default: return undefined;
  }
}

/** The tree's id for a hub's view mirrored under it: never a container's id, which is a UUID. */
export const tabNodeId = (placeId: string, tab: string): string => `${placeId}/tab/${tab}`;

/** The place and the tab a tree id names, or nothing for any other row. */
export function tabOfNode(id: string): { placeId: string; tab: string } | undefined {
  const match = /^(.+)\/tab\/([a-z]+)$/u.exec(id);
  return match ? { placeId: match[1]!, tab: match[2]! } : undefined;
}
