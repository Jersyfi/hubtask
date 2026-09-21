// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one list of destinations (ADR-0061 decision 1).
 *
 * Every width draws this list and nothing else: the primary group as a pinned `SideNav` from
 * `expanded`, in the `NavDrawer` on `medium`, and as the `BottomBar` on `compact`; the account
 * group behind the avatar from `medium` up and behind "You" in the bottom bar below it. A
 * destination that is in one drawing and not another is the failure this file exists to
 * prevent, which is why the frame has no list of its own and why the test beside this file
 * reads the route table rather than trusting the strings here.
 *
 * Two things are deliberately **not** in the list. The search field: `/search` is a destination,
 * and a field in the bar would be a second entry to it. And the trash as a destination: it is
 * content of the workspace (arc42 F-09), so it is the last node of the tree, under the hubs
 * where somebody looks when something is missing — `TRASH` below is what the tree appends.
 *
 * `area` is ADR-0032's: the mobile build renders the administration row as an entry that says
 * where the capability lives, and nothing else about the list changes.
 */

import type { IconName } from '@hubtask/design-system/components';

import type { Area } from './router.ts';

export type Group = 'primary' | 'account';

/** What choosing a destination does: goes somewhere, or performs one of the two account verbs. */
export type Target = { readonly kind: 'route'; readonly path: string } | { readonly kind: 'action'; readonly action: 'tour' | 'sign-out' };

export interface Destination {
  /** A stable identifier the drawings key on and the bottom bar announces; never display text. */
  readonly id: string;
  readonly group: Group;
  readonly icon: IconName;
  /** The message code of its word (ADR-0011). */
  readonly code: string;
  readonly target: Target;
  /**
   * The route name the destination is current for, so `aria-current` can be set from the
   * resolved route rather than by comparing paths. An account destination is current for its
   * own route only; the profile also owns `/profile/tokens`.
   */
  readonly routes: readonly string[];
  /** ADR-0032's area, where the row is one the mobile build treats differently. */
  readonly area?: Area;
}

export const DESTINATIONS: readonly Destination[] = [
  // The primary group: the three spaces one moves between. The word for the first is the
  // workspace's own title rather than "Home", because the destination *is* the workspace.
  { id: 'workspace', group: 'primary', icon: 'workspace', code: 'app.workspace.title', target: { kind: 'route', path: '/' }, routes: ['home', 'hub', 'collection', 'item'] },
  { id: 'search', group: 'primary', icon: 'search', code: 'app.nav.search', target: { kind: 'route', path: '/search' }, routes: ['search'] },
  { id: 'jumble', group: 'primary', icon: 'jumble', code: 'app.nav.jumble', target: { kind: 'route', path: '/jumble' }, routes: ['jumble'] },
  // The account group: what the avatar opens. Reachable from every screen, because the profile
  // is where somebody goes when the product is speaking to them in the wrong language — which is
  // exactly the moment a buried link is no use.
  { id: 'profile', group: 'account', icon: 'settings', code: 'app.nav.profile', target: { kind: 'route', path: '/profile' }, routes: ['profile', 'tokens'], area: 'profile' },
  { id: 'installation', group: 'account', icon: 'info', code: 'app.nav.installation', target: { kind: 'route', path: '/installation' }, routes: ['installation'] },
  { id: 'administration', group: 'account', icon: 'capability', code: 'app.nav.administration', target: { kind: 'route', path: '/administration' }, routes: ['administration'], area: 'administration' },
  { id: 'tour', group: 'account', icon: 'info', code: 'app.help.tour_again', target: { kind: 'action', action: 'tour' }, routes: [] },
  { id: 'sign-out', group: 'account', icon: 'log-out', code: 'app.sign_out', target: { kind: 'action', action: 'sign-out' }, routes: [] },
];

/** The tree's last node: the trash, under the hubs, on every width. */
export const TRASH = { id: 'trash', icon: 'trash' as const, code: 'app.nav.trash', path: '/trash', routes: ['trash'] } as const;

/** The word for the account group's head on a phone, where there is no avatar to open. */
export const YOU_CODE = 'app.nav.you';

export function primary(): readonly Destination[] {
  return DESTINATIONS.filter((destination) => destination.group === 'primary');
}

/**
 * The account group, with the one row the server decides. The administration area is offered
 * only where `GET /quotas` is not refused — the frame's `quotas.isReachable`, which is the
 * area's condition exactly and the one place in this client where hiding beats a gate (the
 * reasoning is in `AppFrame`).
 */
export function account(options: { readonly isAdministrationReachable: boolean }): readonly Destination[] {
  return DESTINATIONS.filter(
    (destination) =>
      destination.group === 'account' &&
      (destination.area !== 'administration' || options.isAdministrationReachable),
  );
}

/**
 * Which destination a resolved route belongs to, or nothing for a route outside the list — the
 * sign-in screens, the consent page. The area answers for the whole administration: the table
 * names each of its screens, and the list should not have to.
 */
export function currentDestination(route: { readonly name: string | null; readonly area: Area }): string | undefined {
  if (route.area === 'administration') return 'administration';
  if (route.name === null) return undefined;
  if (route.name === TRASH.routes[0]) return TRASH.id;
  return DESTINATIONS.find((destination) => destination.routes.includes(route.name as string))?.id;
}
