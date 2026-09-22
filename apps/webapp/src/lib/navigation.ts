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
 * The list has a **shape**, and the frame draws it and no other (ADR-0063 decision 1): three
 * bands in one order — the places that are not the tree, the tree itself, and `KEEPING`, pinned
 * to the foot of the column. They never mix, and nothing is drawn outside one.
 *
 * `area` is ADR-0032's: the mobile build renders the administration row as an entry that says
 * where the capability lives, and nothing else about the list changes.
 */

import type { IconName } from '@hubtask/design-system/components';

import type { Area } from './router.ts';

export type Group = 'primary' | 'account';

/**
 * The three bands the navigation is drawn in, in this order and no other (ADR-0063 decision 1).
 *
 * `places` are the rooms of the product that are not the tree; `tree` is the workspace's own
 * structure; `keeping` is where somebody looks when something is **missing**, and it is pinned to
 * the foot of the column so that it is never mixed in with the hubs above it. Nothing is drawn
 * outside a band, and nothing is in two.
 */
export type Band = 'places' | 'tree' | 'keeping';

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
  // The marks say what each row is about (ADR-0063 decision 6). `user` for what is the reader's
  // own; the **gear moves to the administration**, where "the application's settings" is what it
  // actually means — which is the owner's complaint answered exactly, and one icon fewer than the
  // `sliders` the ADR named; `compass` for a tour, which is a way through rather than a notice.
  // The installation used to carry the same outlined `info` as the tour, so two rows of one menu
  // said the same thing with one mark.
  { id: 'profile', group: 'account', icon: 'user', code: 'app.nav.profile', target: { kind: 'route', path: '/profile' }, routes: ['profile', 'tokens'], area: 'profile' },
  { id: 'administration', group: 'account', icon: 'settings', code: 'app.nav.administration', target: { kind: 'route', path: '/administration' }, routes: ['administration'], area: 'administration' },
  { id: 'tour', group: 'account', icon: 'compass', code: 'app.help.tour_again', target: { kind: 'action', action: 'tour' }, routes: [] },
  { id: 'sign-out', group: 'account', icon: 'log-out', code: 'app.sign_out', target: { kind: 'action', action: 'sign-out' }, routes: [] },
  // The installation, at the foot and in the subtle voice: four facts — the product version, the
  // API version, the tenancy and the languages — that nobody navigates to and everybody quotes
  // when they report a problem. It keeps its address and its page; what it loses is a place in
  // somebody's navigation. Everyone may see it: it names no person and no content, and a reader
  // who cannot read their own product's version cannot file a useful report.
  { id: 'about', group: 'account', icon: 'info', code: 'app.nav.about', target: { kind: 'route', path: '/installation' }, routes: ['installation'] },
];

/**
 * The `keeping` band: where a reader goes when something is missing, at the foot of the column.
 *
 * The archive and the trash. Neither is "the tree's last node" any more — ADR-0063 decision 1
 * supersedes that sentence of ADR-0061, because a row somebody reaches for when something has gone
 * should not sit under the last hub as though it were one.
 */
export interface KeepingRow {
  readonly id: string;
  readonly icon: IconName;
  /** The message code of its word (ADR-0011). */
  readonly code: string;
  readonly path: string;
  readonly routes: readonly string[];
}

const ARCHIVE_ROW: KeepingRow = { id: 'archive', icon: 'archive', code: 'app.nav.archive', path: '/archive', routes: ['archive'] };
const TRASH_ROW: KeepingRow = { id: 'trash', icon: 'trash', code: 'app.nav.trash', path: '/trash', routes: ['trash'] };

// The archive before the trash: what was put aside is the milder of the two, and the one somebody
// reaches for first when something has gone.
export const KEEPING: readonly KeepingRow[] = [ARCHIVE_ROW, TRASH_ROW];

/** Kept as the name the rest of the client uses for the trash's row. */
export const TRASH = TRASH_ROW;

/** The word for the account group's head on a phone, where there is no avatar to open. */
export const YOU_CODE = 'app.nav.you';

/**
 * The primary group, minus the one the bar may be carrying.
 *
 * From `medium` up the bar holds the entry to search (ADR-0063 decision 4), and a row for it in
 * the tree beside it would be the second entry to one destination — the duplication ADR-0061's
 * "no search field in the bar" was protecting against, now kept on the other side. Below that the
 * bar has no room, the field is not there, and Search is a destination in the bottom bar.
 */
export function primary(options: { readonly hasSearchField?: boolean } = {}): readonly Destination[] {
  return DESTINATIONS.filter(
    (destination) =>
      destination.group === 'primary' && !(destination.id === 'search' && options.hasSearchField === true),
  );
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
  const keeping = KEEPING.find((each) => each.routes.includes(route.name as string));
  if (keeping) return keeping.id;
  return DESTINATIONS.find((destination) => destination.routes.includes(route.name as string))?.id;
}
