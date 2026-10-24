// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The arithmetic of "what this device opened last": what a kept list is worth reading back, and
 * where one more row goes.
 *
 * Pure and beside the store that holds it, for the reason `searchquery.ts` is: none of this needs
 * a browser, and a rule about what a list may hold is
 * something to read in a test rather than to walk in one. What it decides is in
 * `recents.svelte.ts`'s note at the top; this is the half with no state.
 */

/** How many are kept. A glance, not a history: what fell off the end is what the tree is for. */
export const MAX = 5;

/** Where one account's list is kept. The id is in the key, so two people sharing a browser never read each other's. */
export const PREFIX = 'hubtask.recent.';

export function keyFor(accountId: string): string {
  return PREFIX + accountId;
}

/** What was opened: an entry, or a container of either level. */
export type RecentKind = 'item' | 'collection' | 'hub';

export interface Recent {
  readonly kind: RecentKind;
  readonly id: string;
  /** The title as it read when it was opened. A title that has since changed is a stale word on a row, not a wrong link. */
  readonly title: string;
  /** When it was opened, in milliseconds - for "5 minutes ago" on its tile. Absent in a list kept before it was noted. */
  readonly at?: number;
  /** Where it lives: an entry's collection, a collection's hub - for its tile's mark and place. Absent on a hub. */
  readonly placeId?: string;
}

const KINDS: readonly string[] = ['item', 'collection', 'hub'];

function isRecent(row: unknown): row is Recent {
  if (typeof row !== 'object' || row === null) return false;
  const each = row as Partial<Recent>;
  return (
    typeof each.id === 'string' &&
    typeof each.title === 'string' &&
    KINDS.includes(each.kind as string) &&
    (each.at === undefined || typeof each.at === 'number') &&
    (each.placeId === undefined || typeof each.placeId === 'string')
  );
}

/**
 * What survives a reload, as far as it can be trusted.
 *
 * Anything else is discarded rather than repaired: this is a convenience, and a convenience that
 * throws on a value somebody's other tab wrote in an older version is worse than an empty list.
 */
export function parse(written: string | null): readonly Recent[] {
  if (!written) return [];
  try {
    const read: unknown = JSON.parse(written);
    return Array.isArray(read) ? read.filter(isRecent).slice(0, MAX) : [];
  } catch {
    return [];
  }
}

/**
 * Puts one at the front, wherever it was before.
 *
 * Moved rather than added, so that opening the same entry twice does not fill the list with it —
 * and so that the order is "last opened" rather than "first opened", which is the only order a
 * list this short can be read in.
 */
export function promote(rows: readonly Recent[], row: Recent): readonly Recent[] {
  return [row, ...rows.filter((each) => !(each.id === row.id && each.kind === row.kind))].slice(0, MAX);
}

/** Whether promoting would change anything: the same row, already at the front, under the same title. */
export function isUnchanged(rows: readonly Recent[], row: Recent): boolean {
  const first = rows[0];
  return first !== undefined && first.id === row.id && first.kind === row.kind && first.title === row.title;
}
