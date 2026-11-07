// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One view bar per object (the navigation concept §3): a hub tool's own tabs and a collection's
 * views, each only where its module is on at the hub (UC-INS-21). A hub itself has no bar - its
 * tools are rows of the sidebar's hub level (`navigation.ts`), never tabs - and there is never a
 * second strip: what was a tab's second strip is a view of the same bar.
 */

import type { IconName } from '@hubtask/design-system/components';

import { HUB_TOOLS, hubTools, toolPath } from '../navigation.ts';

export interface PlaceView {
  readonly id: string;
  readonly code: string;
  readonly icon?: IconName;
  readonly href: string;
}

/** A hub tool's own tabs: *Plan* holds the schedule and the releases, *Time* the actuals and the workload. */
export function toolViews(hubId: string, tool: string): readonly PlaceView[] {
  const base = `/hubs/${hubId}`;
  if (tool === 'plan') {
    return [
      { id: 'plan', code: 'app.planning.title', href: `${base}/plan` },
      { id: 'releases', code: 'app.releases.title', href: `${base}/releases` },
    ];
  }
  if (tool === 'time') {
    return [
      { id: 'actuals', code: 'app.actuals.title', href: `${base}/actuals` },
      { id: 'workload', code: 'app.workload.title', href: `${base}/workload` },
    ];
  }
  return [];
}

export interface CollectionInput {
  readonly id: string;
  /** The modules on at the collection: the workspace's, narrowed by its hub. */
  readonly modulesOn: readonly string[];
  /** Whether it acts as a desk (UC-SVC-05): only then is its portal a view. */
  readonly isDesk?: boolean;
  /** Whether it runs sprints (its sprints' read says so): only then are they a view. */
  readonly runsSprints?: boolean;
}

/** A collection's views, *Entries* first and always there; time's two screens are two views of the one bar. */
export function collectionViews(input: CollectionInput): readonly PlaceView[] {
  const on = (module: string) => input.modulesOn.includes(module);
  const base = `/collections/${input.id}`;
  return [
    { id: 'entries', code: 'app.place_tabs.entries', icon: 'task', href: base },
    ...(on('PLANNING') ? [{ id: 'plan', code: 'app.planning.title', icon: 'calendar' as const, href: `${base}/plan` }] : []),
    ...(on('PLANNING') && input.runsSprints ? [{ id: 'sprints', code: 'app.sprints.title', icon: 'repeat' as const, href: `${base}/sprints` }] : []),
    ...(on('SERVICE_DESK') && input.isDesk ? [{ id: 'portal', code: 'app.place_tabs.portal', icon: 'inbox' as const, href: `/portal/${input.id}` }] : []),
    ...(on('TIME')
      ? [
          { id: 'actuals', code: 'app.actuals.title', icon: 'clock' as const, href: `${base}/actuals` },
          { id: 'workload', code: 'app.workload.title', icon: 'clock' as const, href: `${base}/workload` },
        ]
      : []),
  ];
}

/**
 * Where a hub's products are, for the palette's *Go to <hub> › <place>* (user-experience.md §5.3):
 * its tools - without *Queues*, whose desk the palette does not read - and its pages and boards.
 */
export function hubPlaces(hubId: string, modulesOn: readonly string[]): readonly PlaceView[] {
  return [
    ...hubTools({ modulesOn, hasDesk: false }).map((tool) => ({ id: tool.id, code: tool.code, icon: tool.icon, href: toolPath(hubId, tool.id) })),
    ...(modulesOn.includes('KNOWLEDGE') ? [{ id: 'pages', code: 'app.pages.title', icon: 'book-open' as const, href: `/hubs/${hubId}/pages` }] : []),
    ...(modulesOn.includes('WHITEBOARD') ? [{ id: 'whiteboards', code: 'app.whiteboards.title', icon: 'layout-template' as const, href: `/hubs/${hubId}/whiteboards` }] : []),
  ];
}

/** Where a module is found in a hub once switched on, or nothing for one the hub does not show. */
export function hubPlaceOf(hubId: string, module: string): PlaceView | undefined {
  const tool = HUB_TOOLS.find((each) => each.module === module);
  if (tool) return { id: tool.id, code: tool.code, icon: tool.icon, href: toolPath(hubId, tool.id) };
  return hubPlaces(hubId, [module]).find((each) => each.id === 'pages' || each.id === 'whiteboards');
}
