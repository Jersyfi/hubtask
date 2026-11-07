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

/** What a hub tool's tabs depend on beyond the hub: whether the reader manages costs (*Budget*). */
export interface ToolInput {
  readonly mayCost?: boolean;
}

/**
 * A hub tool's own tabs (§2.4): *Plan* the schedule, the releases and the sprints; *Inventory* its
 * sections are its own (`assetStrips`, *Manage* a menu at the bar's end); *Time* the actuals,
 * the workload and, for whoever manages costs, the budget; *Goals* the goals and the dashboards.
 */
export function toolViews(hubId: string, tool: string, input: ToolInput = {}): readonly PlaceView[] {
  const base = `/hubs/${hubId}`;
  switch (tool) {
    case 'plan':
      return [
        { id: 'schedule', code: 'app.nav.view_schedule', icon: 'calendar', href: `${base}/plan` },
        { id: 'releases', code: 'app.releases.title', icon: 'flag', href: `${base}/plan/releases` },
        { id: 'sprints', code: 'app.sprints.title', icon: 'repeat', href: `${base}/plan/sprints` },
      ];
    case 'time':
      return [
        { id: 'actuals', code: 'app.nav.view_actuals', icon: 'clock', href: `${base}/time` },
        { id: 'workload', code: 'app.workload.title', icon: 'users', href: `${base}/time/workload` },
        ...(input.mayCost ? [{ id: 'budget', code: 'app.nav.view_budget', icon: 'gauge' as const, href: `${base}/time/budget` }] : []),
      ];
    case 'goals':
      return [
        { id: 'goals', code: 'app.goals.tab_goals', icon: 'flag', href: `${base}/goals` },
        { id: 'dashboards', code: 'app.goals.tab_dashboards', icon: 'gauge', href: `${base}/goals/dashboards` },
      ];
    default:
      return [];
  }
}

export interface CollectionInput {
  readonly id: string;
  /** The modules on at the collection: the workspace's, narrowed by its hub. */
  readonly modulesOn: readonly string[];
  /** The layouts the collection offers (`layoutsOffered`); the list is one view whichever value. */
  readonly layouts?: readonly string[];
  /** Whether it acts as a desk (UC-SVC-05): its views are the list, the board and the saved ones. */
  readonly isDesk?: boolean;
  /** Whether it runs sprints (its sprints' read says so): the board and the sprint lead. */
  readonly runsSprints?: boolean;
  /** The collection's saved views, in their order. */
  readonly saved?: readonly { readonly id: string; readonly name: string }[];
}

/** A collection's view as the bar names it: a code to translate, or a saved view's own name. */
export interface CollectionView extends Omit<PlaceView, 'code'> {
  readonly code?: string;
  readonly name?: string;
}

/**
 * A collection's one view bar (§2.4 part 3, §3): the list and the board, the timeline - the former
 * *Plan* where the plan is on, else the timeline layout -, the sprint where it runs sprints, the
 * time where time is on, then its saved views. A collection that runs sprints opens with the board
 * and the sprint; a desk keeps to the list, the board and its saved views.
 */
export function collectionViews(input: CollectionInput): readonly CollectionView[] {
  const on = (module: string) => input.modulesOn.includes(module);
  const base = `/collections/${input.id}`;
  const layouts = input.layouts ?? ['LIST_COLLAPSED', 'KANBAN', 'TIMELINE'];
  const has = (layout: string) => layouts.includes(layout);
  const list: CollectionView = { id: 'list', code: 'app.view.LIST_COLLAPSED', icon: 'square-check', href: `${base}?layout=list` };
  const board: CollectionView[] = has('KANBAN') ? [{ id: 'board', code: 'app.view.KANBAN', icon: 'columns-3', href: `${base}?layout=board` }] : [];
  const timeline: CollectionView[] = on('PLANNING')
    ? [{ id: 'timeline', code: 'app.view.TIMELINE', icon: 'calendar', href: `${base}/timeline` }]
    : has('TIMELINE')
      ? [{ id: 'timeline', code: 'app.view.TIMELINE', icon: 'calendar', href: `${base}?layout=timeline` }]
      : [];
  const sprint: CollectionView[] = on('PLANNING') && input.runsSprints ? [{ id: 'sprint', code: 'app.nav.view_sprint', icon: 'repeat', href: `${base}/sprint` }] : [];
  const time: CollectionView[] = on('TIME') ? [{ id: 'time', code: 'app.nav.view_time', icon: 'clock', href: `${base}/time` }] : [];
  const saved: CollectionView[] = (input.saved ?? []).map((view) => ({ id: `view:${view.id}`, name: view.name, icon: 'star', href: `${base}?view=${view.id}` }));
  if (input.isDesk && on('SERVICE_DESK')) return [list, ...board, ...saved];
  if (sprint.length > 0) return [...board, ...sprint, list, ...timeline, ...time, ...saved];
  return [list, ...board, ...timeline, ...time, ...saved];
}

/** Whether a view is a screen of its own, which the palette offers as *Go to*; a layout or a saved view is the collection's own screen. */
export const isOwnPage = (view: { readonly href: string }): boolean => !view.href.includes('?');

/** The view a collection's layout is in its bar: the list's two values are one view. */
export function viewOfLayout(layout: string): string {
  if (layout === 'KANBAN') return 'board';
  if (layout === 'TIMELINE') return 'timeline';
  return 'list';
}

/** The layout a view of the bar asks for (`?layout=`), or nothing for one that is not a layout. */
export function layoutOfView(view: string | undefined, listLayout: string): string | undefined {
  if (view === 'board') return 'KANBAN';
  if (view === 'timeline') return 'TIMELINE';
  if (view === 'list') return listLayout;
  return undefined;
}

/**
 * Where a hub's products are, for the palette's *Go to <hub> › <place>* (user-experience.md §5.3):
 * its tools - without *Queues*, whose desk the palette does not read - and its pages and boards.
 */
export function hubPlaces(hubId: string, modulesOn: readonly string[]): readonly PlaceView[] {
  return [
    ...hubTools({ modulesOn, hasDesk: false }).map((tool) => ({ id: tool.id, code: tool.code, icon: tool.icon, href: toolPath(hubId, tool.id) })),
    ...(modulesOn.includes('KNOWLEDGE') || modulesOn.includes('WHITEBOARD')
      ? [{ id: 'contents', code: 'app.nav.all_contents', icon: 'book-open' as const, href: `/hubs/${hubId}/contents` }]
      : []),
  ];
}

/** Where a module is found in a hub once switched on, or nothing for one the hub does not show. */
export function hubPlaceOf(hubId: string, module: string): PlaceView | undefined {
  const tool = HUB_TOOLS.find((each) => each.module === module);
  if (tool) return { id: tool.id, code: tool.code, icon: tool.icon, href: toolPath(hubId, tool.id) };
  if (module === 'WHITEBOARD') return { id: 'contents', code: 'app.nav.all_contents', icon: 'layout-template', href: `/hubs/${hubId}/contents?kind=whiteboards` };
  return hubPlaces(hubId, [module]).find((each) => each.id === 'contents');
}
