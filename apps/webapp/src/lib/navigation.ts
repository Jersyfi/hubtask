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
  // `app.nav.overview` and not `app.workspace.title`: since ADR-0063 decision 1 the first
  // destination is the overview - what is on the reader - rather than a list of the hubs the tree
  // below it lists. The workspace keeps its own name for what the workspace is called.
  { id: 'workspace', group: 'primary', icon: 'workspace', code: 'app.nav.overview', target: { kind: 'route', path: '/' }, routes: ['home', 'hub', 'collection', 'item'] },
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

/**
 * The administration's own navigation, in the five groups ADR-0063 decision 7 names.
 *
 * A second list rather than a band of the first, because it is a second *place*: while the route's
 * area is `administration` the column shows this and the workspace's tree is not drawn at all. The
 * reader is inside the section, and a tree of hubs beside sixteen settings screens would say they
 * are somewhere they are not.
 *
 * Who sees it is not decided here and not by this list. The area is offered where `GET /quotas` is
 * not refused, which is the area's condition exactly (ADR-0061 decision 1); a reader without it has
 * no row, no section and no route, and the server refuses the screens regardless.
 */
export interface SectionRow {
  readonly id: string;
  readonly icon: IconName;
  /** The word the row carries in the column. */
  readonly code: string;
  /**
   * The word the screen wears as its heading, where a column has less room than a heading does.
   *
   * "Where you are signed in" is the right sentence over a table and four words too many in a
   * column 240 px wide, where it would be cut to "Where you are sign…". So the row says *Signed
   * in* and the screen says the sentence; absent, they are the same word, which is what every row
   * of the administration is.
   */
  readonly titleCode?: string;
  readonly path: string;
  /** The route names this row is current for. */
  readonly routes: readonly string[];
}

export interface SectionGroup {
  readonly id: string;
  /** The group's caption, or nothing for the head, which is one row and needs none. */
  readonly code?: string;
  readonly rows: readonly SectionRow[];
}

/** The marks come from the set ADR-0041 declares; issue 998 added the three it was missing. */
export const ADMINISTRATION: readonly SectionGroup[] = [
  // The way back, first and alone: a section a reader cannot leave is a trap, and the row that
  // leads out is the one they look for at the top rather than at the foot.
  {
    id: 'back',
    rows: [
      { id: 'back', icon: 'chevron-left', code: 'app.admin.back', path: '/', routes: [] },
    ],
  },
  {
    id: 'workspace',
    code: 'app.admin.group_workspace',
    rows: [
      { id: 'workspace', icon: 'settings', code: 'app.admin.workspace', path: '/administration/workspace', routes: ['workspace-settings'] },
      { id: 'people', icon: 'user', code: 'app.admin.people', path: '/administration/people', routes: ['people'] },
      { id: 'groups', icon: 'users', code: 'app.admin.groups', path: '/administration/groups', routes: ['groups'] },
      { id: 'permissions', icon: 'shield', code: 'app.admin.permissions', path: '/administration/permissions', routes: ['permissions'] },
      { id: 'service-accounts', icon: 'key', code: 'app.admin.service_accounts', path: '/administration/service-accounts', routes: ['service-accounts'] },
      { id: 'apps', icon: 'link', code: 'app.admin.apps', path: '/administration/apps', routes: ['apps'] },
    ],
  },
  {
    id: 'automatic',
    code: 'app.admin.group_automatic',
    rows: [
      // The editor's two routes are the rules' row as well: a reader inside a rule is inside
      // automation, and a column that marked nothing current there would say they are nowhere.
      // `rule-editor` was a name no route has, which is exactly what it said.
      { id: 'rules', icon: 'automation', code: 'app.admin.rules', path: '/administration/rules', routes: ['rules', 'rule', 'rule-new'] },
      { id: 'runs', icon: 'play', code: 'app.admin.runs', path: '/administration/runs', routes: ['runs'] },
      { id: 'webhooks', icon: 'send', code: 'app.admin.webhooks', path: '/administration/webhooks', routes: ['webhooks'] },
    ],
  },
  {
    id: 'holds',
    code: 'app.admin.group_holds',
    rows: [
      { id: 'quotas', icon: 'gauge', code: 'app.admin.quotas', path: '/administration/quotas', routes: ['quotas'] },
      { id: 'backup', icon: 'cloud-upload', code: 'app.admin.backup', path: '/administration/backup', routes: ['backup'] },
      { id: 'retention', icon: 'clock', code: 'app.admin.retention', path: '/administration/retention', routes: ['retention'] },
      { id: 'restore', icon: 'rotate-ccw', code: 'app.admin.restore', path: '/administration/restore', routes: ['restore'] },
    ],
  },
  {
    id: 'record',
    code: 'app.admin.group_record',
    rows: [
      { id: 'audit', icon: 'file-text', code: 'app.admin.audit', path: '/administration/audit', routes: ['audit'] },
      { id: 'privacy', icon: 'file-user', code: 'app.admin.privacy', path: '/administration/privacy', routes: ['privacy'] },
    ],
  },
  {
    id: 'entry',
    code: 'app.admin.group_entry',
    rows: [
      { id: 'identity-provider', icon: 'globe', code: 'app.admin.identity_provider', path: '/administration/identity-provider', routes: ['identity-provider'] },
      { id: 'ai', icon: 'sparkles', code: 'app.admin.ai', path: '/administration/ai', routes: ['ai'] },
    ],
  },
];

/**
 * Your settings, as a section of its own (ADR-0065 decision 3).
 *
 * The same anatomy the administration has, for the same reason: what is the reader's own was one
 * screen of nine sections, read by scrolling, with two lists in it that grow without bound. A
 * section gives each question an address, a heading and a place in a column that says where the
 * reader is.
 *
 * The rows reuse the words the screens already have - "Where you are signed in", "Second factor",
 * "Apps you have allowed" - because a row and the screen it opens saying two different things is
 * how a navigation stops being trustworthy.
 *
 * Who sees it is nobody's decision: every one of these screens is about the reader themselves, and
 * the area is ADR-0032's `profile`, which the mobile shell ships in full.
 */
export const SETTINGS: readonly SectionGroup[] = [
  // The way back, first and alone, exactly as the administration's is.
  {
    id: 'back',
    rows: [
      { id: 'back', icon: 'chevron-left', code: 'app.you.back', path: '/', routes: [] },
    ],
  },
  {
    id: 'you',
    code: 'app.you.group_you',
    rows: [
      { id: 'profile', icon: 'user', code: 'app.you.row_profile', titleCode: 'app.profile.title', path: '/profile', routes: ['profile'] },
      // The theme and reduced motion are the device's (ADR-0043) and the celebrations are the
      // account's; what they have in common is that they are how the product looks and moves at
      // the reader, which is what the screen is called.
      { id: 'appearance', icon: 'sun-moon', code: 'app.profile.device_section', path: '/profile/appearance', routes: ['appearance'] },
    ],
  },
  {
    id: 'told',
    code: 'app.you.group_told',
    rows: [
      { id: 'notifications', icon: 'bell', code: 'app.you.row_notifications', titleCode: 'app.profile.notifications', path: '/profile/notifications', routes: ['notifications'] },
    ],
  },
  {
    id: 'entry',
    code: 'app.you.group_entry',
    rows: [
      { id: 'security', icon: 'shield', code: 'app.mfa.title', path: '/profile/security', routes: ['security'] },
      { id: 'sessions', icon: 'user-check', code: 'app.you.row_sessions', titleCode: 'app.sessions.title', path: '/profile/sessions', routes: ['sessions'] },
      { id: 'devices', icon: 'arrow-right-left', code: 'app.you.row_devices', titleCode: 'app.devices.title', path: '/profile/devices', routes: ['devices'] },
    ],
  },
  {
    id: 'acting',
    code: 'app.you.group_acting',
    rows: [
      { id: 'grants', icon: 'link', code: 'app.you.row_grants', titleCode: 'app.grants.title', path: '/profile/apps', routes: ['grants'] },
      { id: 'tokens', icon: 'key', code: 'app.tokens.title', path: '/profile/tokens', routes: ['tokens'] },
    ],
  },
];

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
  // And the same for Your settings, which is a section of its own since ADR-0065 decision 3: its
  // screens are the area, and the list should not have to name each of them twice.
  if (route.area === 'profile') return 'profile';
  if (route.name === null) return undefined;
  const keeping = KEEPING.find((each) => each.routes.includes(route.name as string));
  if (keeping) return keeping.id;
  return DESTINATIONS.find((destination) => destination.routes.includes(route.name as string))?.id;
}

/**
 * The screen a section's own address opens (ADR-0065 decision 1).
 *
 * A section with a navigation column needs no overview, because the column *is* the overview: an
 * index beside it is the same list drawn twice, and arriving at it means arriving at a page of
 * links to where the reader was already going. So `/administration` answers with the section's
 * first screen.
 *
 * The first row with a route of its own, which is what skips the way back: that row leads out of
 * the section, and a front door that led out would be a door somebody falls through.
 */
export function firstScreen(groups: readonly SectionGroup[]): string {
  for (const group of groups) {
    for (const row of group.rows) {
      if (row.routes.length > 0) return row.path;
    }
  }
  // Unreachable for a section that has a screen; a section that has none has nothing to open.
  return '/';
}
