// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the palette offers wherever the reader is, beside what the screen on view offers
 * (`commands.svelte.ts`): the actions of the frame - a hub, a collection in the hub one stands in,
 * capture - the rows of one's own settings screens, and the places this device opened last
 * (UC-WRK-27 check 2).
 *
 * **It takes answers, it asks no question.** Whether the reader may create a hub, shape this hub
 * or capture is handed in as the frame's controls ask it (`hubCreationHere`, `STRUCTURE` on the
 * hub, `mayCaptureHere`); an answer "not known yet" is "not listed" (P-05). A settings row is the
 * reader's own and listed for every account, except where its screen draws nothing to set.
 *
 * Pure, over a translator handed in, and tested beside itself.
 */

import type { Container } from '@hubtask/sync-engine';

import { archivalOf } from '../data/containers.ts';
import type { Recent } from '../recents.ts';
import type { Offer, Translate } from './sources.ts';

// ---------------------------------------------------------------------------------------------
// The frame's actions
// ---------------------------------------------------------------------------------------------

/** What the frame knows about what the reader may do where they stand. */
export interface Doing {
  /** Which hub the reader may create: as the overview's head and the tree's *New* ask it. */
  readonly hubCreation: 'shared' | 'private' | 'none' | undefined;
  /** The hub the reader stands in - on it, in one of its collections, on one of its entries. */
  readonly hub?: Container;
  /** Whether the reader's role on that hub holds `STRUCTURE`; `false` while unknown. */
  readonly mayShapeHub: boolean;
  /** Whether the hub's own page is on view: its head offers the collection already. */
  readonly isOnHubPage: boolean;
  /** Whether the capture control is drawn (UC-WRK-26 check 1). */
  readonly mayCapture: boolean;
}

/**
 * The frame's actions this reader may perform here: create a hub (or a private one), create a
 * collection in the hub they stand in, capture. Each carries the words that find a way of making
 * something in the reader's language, since a reader who types "new" or "anlegen" means all three.
 */
export function frameActionOffers(doing: Doing, t: Translate): readonly Offer[] {
  const making = t('app.palette.words_create');
  const offers: Offer[] = [];
  if (doing.mayCapture) {
    offers.push({ kind: 'action', id: 'frame:capture', label: t('app.nav.capture'), keywords: making, shortcut: 'capture', target: { kind: 'capture' } });
  }
  const { hub } = doing;
  if (hub && !doing.isOnHubPage && doing.mayShapeHub && archivalOf(hub) === 'active') {
    offers.push({
      kind: 'action',
      id: 'frame:create-collection',
      label: t('app.palette.create_collection_in', { hub: hub.name }),
      keywords: making,
      target: { kind: 'create', type: 'COLLECTION', parentId: hub.id },
    });
  }
  if (doing.hubCreation === 'shared' || doing.hubCreation === 'private') {
    const isPrivateOnly = doing.hubCreation === 'private';
    offers.push({
      kind: 'action',
      id: 'frame:create-hub',
      label: t(isPrivateOnly ? 'app.workspace.create_private_hub' : 'app.workspace.create_hub'),
      keywords: making,
      target: { kind: 'create', type: 'HUB', isPrivateOnly },
    });
  }
  return offers;
}

// ---------------------------------------------------------------------------------------------
// The rows of one's own settings
// ---------------------------------------------------------------------------------------------

/** One row of a settings screen, where the screen marks it `data-setting={id}`. */
interface SettingRow {
  readonly id: string;
  readonly code: string;
  readonly path: string;
  /** The screen's name, said beside the row. */
  readonly screen: string;
  /** Other words that find it, as a message code. */
  readonly words?: string;
}

/** The rows of *You*, *On this device* and *Password and sign-in*, in the screens' own order. */
export const SETTING_ROWS: readonly SettingRow[] = [
  { id: 'name', code: 'app.profile.display_name', path: '/profile', screen: 'app.profile.title', words: 'app.palette.words_name' },
  { id: 'language', code: 'app.profile.language', path: '/profile', screen: 'app.profile.title', words: 'app.palette.words_language' },
  { id: 'zone', code: 'app.profile.zone', path: '/profile', screen: 'app.profile.title' },
  { id: 'week-start', code: 'app.profile.week_start', path: '/profile', screen: 'app.profile.title', words: 'app.palette.words_week' },
  { id: 'celebrations', code: 'app.profile.celebrations', path: '/profile', screen: 'app.profile.title' },
  { id: 'theme', code: 'app.profile.theme', path: '/profile/appearance', screen: 'app.profile.device_section', words: 'app.palette.words_theme' },
  { id: 'motion', code: 'app.profile.motion', path: '/profile/appearance', screen: 'app.profile.device_section', words: 'app.palette.words_motion' },
  { id: 'open', code: 'app.profile.open', path: '/profile/appearance', screen: 'app.profile.device_section', words: 'app.palette.words_open' },
  { id: 'password', code: 'app.password.change_title', path: '/profile/security', screen: 'app.security.title' },
  { id: 'passkeys', code: 'app.passkeys.title', path: '/profile/security', screen: 'app.security.title' },
  { id: 'second-factor', code: 'app.mfa.title', path: '/profile/security', screen: 'app.security.title', words: 'app.palette.words_second_factor' },
];

/** What decides which settings rows are drawn: the notification categories, where mail can reach. */
export interface Settable {
  /** The manifest's categories; empty for an account without a mailbox, whose screen draws none. */
  readonly categories: readonly string[];
}

/**
 * Every settings row as a row that opens its screen at it: the fixed rows above, then one per
 * notification category, as the notifications screen draws them.
 */
export function settingOffers(settable: Settable, t: Translate): readonly Offer[] {
  const told = t('app.you.row_notifications');
  return [
    ...SETTING_ROWS.map(
      (row): Offer => ({
        kind: 'setting',
        id: row.id,
        label: t(row.code),
        keywords: row.words ? t(row.words) : undefined,
        context: t(row.screen),
        area: 'profile',
        target: { kind: 'setting', path: row.path, row: row.id },
      }),
    ),
    ...settable.categories.map(
      (category): Offer => ({
        kind: 'setting',
        id: `notify-${category}`,
        label: t(`app.profile.category_${category}`),
        keywords: told,
        context: told,
        area: 'profile',
        target: { kind: 'setting', path: '/profile/notifications', row: `notify-${category}` },
      }),
    ),
  ];
}

// ---------------------------------------------------------------------------------------------
// Where the reader was
// ---------------------------------------------------------------------------------------------

/** The address of a place this device opened last. */
export function recentPath(recent: Recent): string {
  if (recent.kind === 'item') return `/items/${recent.id}`;
  return recent.kind === 'hub' ? `/hubs/${recent.id}` : `/collections/${recent.id}`;
}

/** The places this device opened last, most recent first, as rows that open them again. */
export function recentOffers(rows: readonly Recent[]): readonly Offer[] {
  return rows.map((recent) => ({ kind: 'recent', id: `${recent.kind}:${recent.id}`, label: recent.title, target: { kind: 'route', path: recentPath(recent) } }));
}
