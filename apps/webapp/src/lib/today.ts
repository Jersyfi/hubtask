// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What *Today* draws, decided from what the reads answered (UC-WRK-34).
 *
 * Pure and tested, because the rules that matter are here and nowhere else:
 *
 * * **Where the day ends is the reader's.** An entry is due today when its date falls before the
 *   reader's next day begins - a timed date by the instant, an all-day date by the day it names in
 *   its own zone - and overdue as `data/due.ts` says, the one answer to it (UC-WRK-20 check 1).
 * * **Earlier, today, coming up.** Overdue entries keep a band of their own, but the band's word
 *   is a time and not a verdict: nothing on the page says what the person did not do (check 6).
 * * **A list is one tile, not its entries** (check 2). A collection whose figure is *open* is drawn
 *   as itself with its count, so its entries are left out of the bands.
 * * **A project shows its next due step** (check 3): a collection whose figure is *progress*.
 */

import type { Container, WorkItem } from '@hubtask/sync-engine';

import { dateIn, instantOf, todayIn } from './i18n/zone.ts';
import { isOverdue, nextDay } from './data/due.ts';

/** The three bands of what is on the reader, each soonest first as the server sorted them. */
export interface DayBands {
  /** Due before now: overdue, said as a time. */
  readonly earlier: readonly WorkItem[];
  /** Due before the reader's day ends, and not yet past. */
  readonly today: readonly WorkItem[];
  /** Due after today. UC-WRK-20's *next*. */
  readonly later: readonly WorkItem[];
}

/** Whether the entry falls due before the reader's next day begins. */
export function isDueByTonight(item: WorkItem, readerZone: string, now: number): boolean {
  if (item.completion?.is_completed || !item.due_at) return false;
  const today = todayIn(readerZone, now);
  if (item.due_date_only) {
    const day = dateIn(item.due_at, item.due_time_zone ?? readerZone);
    return day !== undefined && day <= today;
  }
  const tomorrow = instantOf(nextDay(today), undefined, readerZone);
  const at = Date.parse(item.due_at);
  return tomorrow !== undefined && !Number.isNaN(at) && at < Date.parse(tomorrow);
}

/**
 * The entries on the reader, split where their day divides them. An entry of a list is left out:
 * the list's tile stands for it (`isList`).
 */
export function bandsOf(
  items: readonly WorkItem[],
  readerZone: string,
  now: number,
  isList: (collectionId: string) => boolean,
): DayBands {
  const earlier: WorkItem[] = [];
  const today: WorkItem[] = [];
  const later: WorkItem[] = [];
  for (const item of items) {
    if (isList(item.collection_id)) continue;
    if (isOverdue(item, readerZone, now)) earlier.push(item);
    else if (isDueByTonight(item, readerZone, now)) today.push(item);
    else later.push(item);
  }
  return { earlier, today, later };
}

/** Whether a collection counts what is open: a list, drawn on *Today* as one tile. */
export function isListFigure(collection: Container): boolean {
  return collection.type === 'COLLECTION' && collection.figure?.kind === 'OPEN';
}

/** Whether a collection counts its progress: a project, drawn with its next due step. */
export function isProjectFigure(collection: Container): boolean {
  return collection.type === 'COLLECTION' && collection.figure?.kind === 'PROGRESS';
}

/**
 * The read of a project's next due step: its open entries with a date, at any level, soonest
 * first, one of them. `items:query` because it is anchored to the collection the tile names.
 */
export function nextStepQuery(collectionId: string) {
  return {
    scope: { container_id: collectionId, include_descendants: true },
    filter: {
      op: 'AND',
      nodes: [
        { op: 'EQ', field: 'is_completed', value: false },
        { op: 'NOT', nodes: [{ op: 'IS_NULL', field: 'due_at' }] },
      ],
    },
    sort: [{ field: 'due_at', dir: 'ASC' as const }],
    page: { size: 1 },
  };
}
