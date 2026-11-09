// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The phone's frame (the navigation concept §9, `compact`): the bottom bar's five tabs, which tab a
 * route belongs to, each tab's own stack, the way one level up and the level a screen stands on.
 * Pure; `AppFrame` draws what these answer, and the Browse stack's steps are routes of their own
 * (`/browse…`), so the system's back goes one level up like the bar's ‹.
 */

import type { IconName } from '@hubtask/design-system/components';
import type { LevelRoute } from './sidebar.ts';

/** The bottom bar's tabs in their order: *Today · Browse · ＋ · Inbox · Search*. */
export type PhoneTab = 'today' | 'browse' | 'capture' | 'inbox' | 'search';

export interface PhoneTabRow {
  readonly id: PhoneTab;
  readonly icon: IconName;
  readonly code: string;
  /** A tab's root; the ＋ has none, it is *Capture*. */
  readonly path?: string;
}

const TABS: readonly PhoneTabRow[] = [
  { id: 'today', icon: 'sun', code: 'app.nav.today', path: '/' },
  { id: 'browse', icon: 'hub', code: 'app.nav.browse', path: '/browse' },
  { id: 'capture', icon: 'plus', code: 'app.nav.capture' },
  { id: 'inbox', icon: 'jumble', code: 'app.nav.jumble', path: '/jumble' },
  { id: 'search', icon: 'search', code: 'app.nav.search', path: '/search' },
];

/** The tabs this reader has: the inbox only with the jumble, the ＋ only where they may capture. */
export function phoneTabs(options: { readonly hasJumble: boolean; readonly hasCapture: boolean }): readonly PhoneTabRow[] {
  return TABS.filter((tab) => (tab.id !== 'inbox' || options.hasJumble) && (tab.id !== 'capture' || options.hasCapture));
}

/** The routes that are a tab's root: the bar shows the avatar and the bell there, no ‹. */
const ROOTS: Readonly<Record<string, PhoneTab>> = { home: 'today', jumble: 'inbox', search: 'search', browse: 'browse' };

/** A step of the Browse stack, as its route names it. */
export type PhoneStep =
  | { readonly kind: 'workspace' }
  | { readonly kind: 'hub' | 'queues'; readonly hubId: string }
  | { readonly kind: 'contents'; readonly hubId: string; readonly under?: string };

/** The step a Browse route draws. */
export function stepOf(route: LevelRoute): PhoneStep | undefined {
  const id = route.params['id'];
  if (route.name === 'browse') return { kind: 'workspace' };
  if (!id) return undefined;
  if (route.name === 'browse-hub') return { kind: 'hub', hubId: id };
  if (route.name === 'browse-queues') return { kind: 'queues', hubId: id };
  if (route.name === 'browse-contents') return { kind: 'contents', hubId: id, under: route.query['under'] };
  return undefined;
}

/** An area with a column of its own: on a phone its groups are a Browse step (§9). */
export type AreaStep = 'administration' | 'profile' | 'instance';

/** The Browse step that lists an area's groups, by route name. */
const AREA_STEPS: Readonly<Record<string, AreaStep>> = {
  'browse-administration': 'administration',
  'browse-profile': 'profile',
  'browse-instance': 'instance',
};
/** Each area's step, by the area. */
export const AREA_STEP_PATHS: Readonly<Record<AreaStep, string>> = {
  administration: '/administration/browse',
  profile: '/profile/browse',
  instance: '/instance/browse',
};

/** The area whose groups this route lists, where it is such a step. */
export const areaStepOf = (name: string | null): AreaStep | undefined => (name ? AREA_STEPS[name] : undefined);

/** The Browse stack's own steps. */
export const BROWSE_ROUTES: ReadonlySet<string> = new Set(['browse', 'browse-hub', 'browse-queues', 'browse-contents', ...Object.keys(AREA_STEPS)]);

/** Which tab a route belongs to: a root's own, every other place - the areas' screens too - Browse's. */
export function tabOf(route: { readonly name: string | null; readonly area?: string }): PhoneTab | undefined {
  if (!route.name) return undefined;
  return ROOTS[route.name] ?? 'browse';
}

const isArea = (area: string | undefined): area is AreaStep => area === 'administration' || area === 'profile' || area === 'instance';

/** Whether the route is a tab's root. */
export const isTabRoot = (name: string | null): boolean => name !== null && name in ROOTS;

/**
 * The way one level up (§9, *‹ the level above*): the step above in the Browse stack, a hub's step
 * for anything of a hub, Browse for a product home or any other place. `hubId` is the place's hub
 * where it has one (`levelOf`); the caller names it. Nothing on a root.
 */
export type Ascent =
  | { readonly kind: 'browse'; readonly path: string }
  | { readonly kind: 'hubs'; readonly path: string }
  | { readonly kind: 'hub'; readonly hubId: string; readonly path: string }
  | { readonly kind: 'area'; readonly area: AreaStep; readonly path: string }
  | { readonly kind: 'contents'; readonly hubId: string; readonly pageId?: string; readonly path: string };

export function ascentOf(route: LevelRoute & { readonly area?: string }, hubId: string | undefined, parentOf?: (pageId: string) => string | null | undefined): Ascent | undefined {
  const name = route.name ?? '';
  if (isTabRoot(name) || !name) return undefined;
  if (areaStepOf(name)) return { kind: 'browse', path: '/browse' };
  if (isArea(route.area)) return { kind: 'area', area: route.area, path: AREA_STEP_PATHS[route.area] };
  const id = route.params['id'];
  if (name === 'browse-hub') return { kind: 'hubs', path: '/browse' };
  if ((name === 'browse-queues' || name === 'browse-contents') && id) {
    const under = name === 'browse-contents' ? route.query['under'] : undefined;
    if (under) {
      const parent = parentOf?.(under) ?? undefined;
      return parent
        ? { kind: 'contents', hubId: id, pageId: parent, path: contentsStep(id, parent) }
        : { kind: 'contents', hubId: id, path: contentsStep(id) };
    }
    return { kind: 'hub', hubId: id, path: hubStep(id) };
  }
  if (name === 'hub-desk' && id && route.query['queue']) return { kind: 'hub', hubId: id, path: hubStep(id) };
  if (hubId) return { kind: 'hub', hubId, path: hubStep(hubId) };
  return { kind: 'browse', path: '/browse' };
}

/** A hub's step in the Browse stack. */
export const hubStep = (hubId: string): string => `/browse/hubs/${hubId}`;
/** A hub's queues as a step of their own. */
export const queuesStep = (hubId: string): string => `/browse/hubs/${hubId}/queues`;
/** A hub's contents as a step, at the top or under a page. */
export const contentsStep = (hubId: string, under?: string): string => `/browse/hubs/${hubId}/contents${under ? `?under=${under}` : ''}`;

/**
 * The level a screen stands on, for the announcement on every stack change ("IT operations, level
 * 2"): a root 1, a hub's step 2, what is under a hub 3, deeper contents one more per page.
 */
export function depthOf(route: LevelRoute & { readonly area?: string }, hubId: string | undefined, ancestors = 0): number {
  const name = route.name ?? '';
  if (!name || isTabRoot(name)) return 1;
  if (areaStepOf(name)) return 2;
  if (isArea(route.area)) return 3;
  if (name === 'browse-hub') return 2;
  if (name === 'browse-queues') return 3;
  if (name === 'browse-contents') return 3 + (route.query['under'] ? 1 + ancestors : 0);
  return hubId ? 3 : 2;
}

/** Where a Browse step lives from `medium` up, where the sidebar draws its level (§9). */
export function browseElsewhere(route: LevelRoute): string | undefined {
  const id = route.params['id'];
  switch (route.name) {
    case 'browse': return '/';
    case 'browse-hub': return id ? `/hubs/${id}` : '/';
    case 'browse-queues': return id ? `/hubs/${id}/desk` : '/';
    case 'browse-contents': return id ? `/hubs/${id}/contents` : '/';
    case 'browse-administration': return '/administration';
    case 'browse-profile': return '/profile';
    case 'browse-instance': return '/instance';
    default: return undefined;
  }
}

/** What the bar's ☆ stars: the place the screen is, where it has a star of its own. */
export type StarKind = 'HUB' | 'COLLECTION' | 'PAGE' | 'WHITEBOARD';
export function starOf(route: LevelRoute): { readonly kind: StarKind; readonly id: string } | undefined {
  const name = route.name ?? '';
  const id = route.params['id'];
  if (!id) return undefined;
  if (name === 'hub' || name === 'browse-hub') return { kind: 'HUB', id };
  if (name === 'collection' || name === 'collection-timeline' || name === 'collection-sprint' || name === 'collection-backlog' || name === 'collection-time') return { kind: 'COLLECTION', id };
  if (name === 'page') return { kind: 'PAGE', id };
  if (name === 'whiteboard') return { kind: 'WHITEBOARD', id };
  return undefined;
}

/**
 * Each tab's own stack (§9): the last address seen in it. Choosing another tab returns to where it
 * was left; choosing the current tab again returns to its root.
 */
export class TabStacks {
  readonly #last = new Map<PhoneTab, string>();

  /** Remembers the address as its tab's top. */
  seen(tab: PhoneTab | undefined, path: string): void {
    if (tab && tab !== 'capture') this.#last.set(tab, path);
  }

  /** Where choosing `tab` goes while `current` is the tab shown. */
  target(tab: PhoneTabRow, current: PhoneTab | undefined): string | undefined {
    if (!tab.path) return undefined;
    if (tab.id === current) return tab.path;
    return this.#last.get(tab.id) ?? tab.path;
  }
}
