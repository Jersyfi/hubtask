// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One list for everything (concept round 2, §2): what a list draws besides each entry's title is
 * an arrangement, not a different list.
 *
 * Four lists had four rows - the simple list, the entry list, the subtree, Today's and search's -
 * each for a good reason, and each reason is a setting: which **fields** stand as columns beside
 * the title (L2, L3), how the rows are **grouped** (L4), and where what is **done** goes (L5). A
 * template brings its own arrangement (`defaultArrangement`); the reader changes it with *Fields*
 * and *Group*, the device keeps it per collection, and a saved view carries the fields.
 *
 * Pure, so what a list holds and in which order is read in a test rather than walked.
 */

import type { LayoutStore } from './layoutmemory.ts';

/** What can stand beside a title, in the order the columns are drawn. */
export type ListField = 'amount' | 'column' | 'due' | 'people' | 'labels' | 'progress';
export const LIST_FIELDS: readonly ListField[] = ['amount', 'column', 'due', 'people', 'labels', 'progress'];

/** How the top level is grouped: nothing, or by one value each entry has. */
export type ListGrouping = 'none' | 'category' | 'column' | 'assignee' | 'due';
export const LIST_GROUPINGS: readonly ListGrouping[] = ['none', 'category', 'column', 'assignee', 'due'];

/** Where a done entry goes: where it stands, gathered at the end, or out of sight. */
export type DonePlacement = 'inline' | 'bottom' | 'hidden';
export const DONE_PLACEMENTS: readonly DonePlacement[] = ['inline', 'bottom', 'hidden'];

export interface ListArrangement {
  readonly fields: readonly ListField[];
  readonly grouping: ListGrouping;
  readonly done: DonePlacement;
}

/** What the collection can show at all: a field it does not carry is not offered (P-05). */
export interface ListOffer {
  /** One level in force: the simple list (UC-WRK-25). */
  readonly isSimple: boolean;
  /** More than one level: rows have parts, so their progress is a field. */
  readonly isLevelled: boolean;
  readonly offersAmount: boolean;
  readonly hasColumns: boolean;
  readonly carriesDue: boolean;
  readonly carriesPeople: boolean;
  readonly carriesLabels: boolean;
}

/** The fields the collection offers, in column order. */
export function offeredFields(offer: ListOffer): readonly ListField[] {
  return LIST_FIELDS.filter((field) => {
    switch (field) {
      case 'amount':
        return offer.offersAmount;
      case 'column':
        return offer.hasColumns && !offer.isSimple;
      case 'due':
        return offer.carriesDue;
      case 'people':
        return offer.carriesPeople;
      case 'labels':
        return offer.carriesLabels;
      case 'progress':
        return offer.isLevelled;
    }
  });
}

/** The groupings the collection offers: a value it does not hold is not a way to group by. */
export function offeredGroupings(offer: ListOffer): readonly ListGrouping[] {
  return LIST_GROUPINGS.filter((grouping) => {
    switch (grouping) {
      case 'none':
        return true;
      case 'category':
        return offer.carriesLabels;
      case 'column':
        return offer.hasColumns && !offer.isSimple;
      case 'assignee':
        return offer.carriesPeople;
      case 'due':
        return offer.carriesDue;
    }
  });
}

/**
 * What a template brings (L3): a list its amount, grouped by category with what is done gathered
 * at the end - the shopping list as it always was -; anything with levels its dates, people,
 * labels and the progress of its parts, where they stand.
 */
export function defaultArrangement(offer: ListOffer): ListArrangement {
  const offered = offeredFields(offer);
  if (offer.isSimple) {
    return {
      fields: offered.filter((field) => field === 'amount'),
      grouping: offer.carriesLabels ? 'category' : 'none',
      done: 'bottom',
    };
  }
  return {
    fields: offered.filter((field) => field === 'due' || field === 'people' || field === 'labels' || field === 'progress'),
    grouping: 'none',
    done: 'inline',
  };
}

/** Where the device keeps a collection's arrangement for one account. */
export const arrangementKey = (accountId: string, collectionId: string): string =>
  `hubtask.list.${accountId}.${collectionId}`;

/**
 * The arrangement in force: what the device kept, as far as the collection still offers it, else
 * the template's. A kept value the collection no longer offers falls back rather than failing.
 */
export function openingArrangement(options: {
  readonly store: LayoutStore | undefined;
  readonly accountId: string | undefined;
  readonly collectionId: string;
  readonly offer: ListOffer;
}): ListArrangement {
  const fallback = defaultArrangement(options.offer);
  if (!options.store || !options.accountId) return fallback;
  let kept: unknown;
  try {
    kept = JSON.parse(options.store.getItem(arrangementKey(options.accountId, options.collectionId)) ?? 'null');
  } catch {
    return fallback;
  }
  if (typeof kept !== 'object' || kept === null) return fallback;
  const read = kept as Partial<Record<keyof ListArrangement, unknown>>;
  const offeredF = offeredFields(options.offer);
  const offeredG = offeredGroupings(options.offer);
  const fields = Array.isArray(read.fields)
    ? LIST_FIELDS.filter((field) => (read.fields as unknown[]).includes(field) && offeredF.includes(field))
    : fallback.fields;
  const grouping = offeredG.includes(read.grouping as ListGrouping) ? (read.grouping as ListGrouping) : fallback.grouping;
  const done = DONE_PLACEMENTS.includes(read.done as DonePlacement) ? (read.done as DonePlacement) : fallback.done;
  return { fields, grouping, done };
}

export function rememberArrangement(
  store: LayoutStore | undefined,
  accountId: string | undefined,
  collectionId: string,
  arrangement: ListArrangement,
): void {
  if (!store || !accountId) return;
  try {
    store.setItem(arrangementKey(accountId, collectionId), JSON.stringify(arrangement));
  } catch {
    // A browser that refuses storage keeps the arrangement for as long as the page is open.
  }
}

/** A field toggled on or off, the columns kept in their one order. */
export function toggleField(fields: readonly ListField[], field: ListField): readonly ListField[] {
  const next = fields.includes(field) ? fields.filter((each) => each !== field) : [...fields, field];
  return LIST_FIELDS.filter((each) => next.includes(each));
}

/** One group of a list: its key (null for the entries without the value), and what it holds. */
export interface ListGroup<T> {
  readonly key: string | null;
  readonly rows: readonly T[];
}

/**
 * The top level in groups (L4), each top-level row with what is open under it.
 *
 * `rows` is the flattened list; a block is a top-level row and the rows after it at a greater
 * depth. A block goes to the group of its top-level row's key; the groups come in `order`, the
 * entries without the value last. What is done goes where `done` says: its own group at the end
 * (key `'done'`), nowhere, or where it stands.
 */
export function groupRows<T>(
  rows: readonly T[],
  options: {
    readonly depthOf: (row: T) => number;
    readonly keyOf: (row: T) => string | null;
    readonly isDone: (row: T) => boolean;
    readonly order: readonly string[];
    readonly done: DonePlacement;
    readonly grouping: ListGrouping;
  },
): { readonly groups: readonly ListGroup<T>[]; readonly done: readonly T[] } {
  const blocks: T[][] = [];
  for (const row of rows) {
    if (options.depthOf(row) === 0 || blocks.length === 0) blocks.push([row]);
    else blocks[blocks.length - 1]!.push(row);
  }

  const done: T[] = [];
  const open: T[][] = [];
  for (const block of blocks) {
    if (options.done !== 'inline' && options.isDone(block[0]!)) {
      if (options.done === 'bottom') done.push(...block);
    } else {
      open.push(block);
    }
  }

  if (options.grouping === 'none') return { groups: [{ key: null, rows: open.flat() }], done };

  const byKey = new Map<string | null, T[]>();
  for (const block of open) {
    const key = options.keyOf(block[0]!);
    byKey.set(key, [...(byKey.get(key) ?? []), ...block]);
  }
  const keys = [...byKey.keys()];
  const ordered = [
    ...options.order.filter((key) => byKey.has(key)),
    ...keys.filter((key): key is string => key !== null && !options.order.includes(key)),
    ...(byKey.has(null) ? [null] : []),
  ];
  return { groups: ordered.map((key) => ({ key, rows: byKey.get(key) ?? [] })), done };
}

/** The due bands a list groups by: what is late, today, this week, later, and without a date. */
export type DueBand = 'overdue' | 'today' | 'week' | 'later';
export const DUE_BANDS: readonly DueBand[] = ['overdue', 'today', 'week', 'later'];

/** Which band a due instant falls in, read against the reader's day (`today` and `weekEnd` as YYYY-MM-DD there). */
export function dueBandOf(dueDay: string | undefined, today: string, weekEnd: string): DueBand | null {
  if (!dueDay) return null;
  if (dueDay < today) return 'overdue';
  if (dueDay === today) return 'today';
  if (dueDay <= weekEnd) return 'week';
  return 'later';
}
