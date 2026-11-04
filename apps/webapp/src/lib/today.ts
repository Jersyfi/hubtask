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

/** The part of the day a greeting names, by the hour where the person is. */
export type DayPart = 'morning' | 'day' | 'evening';

/**
 * Morning from five, the day from eleven, the evening from six - read in the person's zone, not the
 * device's, so a person travelling is greeted by the day they live in.
 */
export function greetingOf(zone: string, now: number): DayPart {
  const hour = Number(new Intl.DateTimeFormat('en-GB', { hour: 'numeric', hourCycle: 'h23', timeZone: zone }).format(now));
  if (hour >= 5 && hour < 11) return 'morning';
  if (hour >= 11 && hour < 18) return 'day';
  return 'evening';
}

/** A band *Today* draws for a product (user-experience.md §3.3), in the order it is drawn. */
export type ProductBand = 'approvals' | 'agent_acts' | 'time';

/**
 * The product bands, each only where its module is on (UC-WRK-34 check 8): none while the modules
 * are unknown, because a band whose read the server refuses would be an error, not a day.
 */
export function productBandsOf(modulesOn: readonly string[] | undefined): ProductBand[] {
  const on = new Set(modulesOn ?? []);
  return [
    ...(on.has('SERVICE_DESK') ? ['approvals' as const] : []),
    ...(on.has('AGENTS') ? ['agent_acts' as const] : []),
    ...(on.has('TIME') ? ['time' as const] : []),
  ];
}

/**
 * The count the *Today* row carries (user-experience.md §3.3): the approvals and the agents' held
 * acts waiting for the reader's yes. Nothing while unknown or none, so the row never says "0".
 */
export function todayRowCountOf(
  approvals: readonly { readonly state: string }[] | undefined,
  acts?: readonly { readonly status: string }[],
  approvalsHaveMore = false,
): number | string | undefined {
  const count = (approvals?.filter((approval) => approval.state === 'PENDING').length ?? 0)
    + (acts?.filter((act) => act.status === 'PENDING').length ?? 0);
  if (count === 0) return undefined;
  return approvalsHaveMore ? `${count}+` : count;
}

/** One held act on Today: whose agent, since when, and where it is decided. */
export interface HeldActRow {
  readonly id: string;
  readonly name: string;
  readonly at: string;
  /** An act that cannot be undone is approved where its preview is read, never from a row. */
  readonly mayApproveHere: boolean;
  readonly path: string | undefined;
}

/**
 * The agents' acts waiting for the reader (`GET /agent-requests` answers only those the reader
 * may decide), each leading to the agents' held acts narrowed to its agent (user-experience.md
 * §7.8) where the reader may decide it: their own agent's, or anybody's where they reach the
 * administration.
 */
export function heldActRows(
  acts: readonly { readonly id: string; readonly agent_id: string; readonly class: string; readonly status: string; readonly created_at: string }[] | undefined,
  agents: readonly { readonly id: string; readonly name: string; readonly owner_id: string | null }[],
  viewerId: string | undefined,
  isAdministrationReachable: boolean,
): HeldActRow[] {
  const byId = new Map(agents.map((agent) => [agent.id, agent]));
  return (acts ?? [])
    .filter((act) => act.status === 'PENDING')
    .map((act) => {
      const agent = byId.get(act.agent_id);
      const isOwn = agent !== undefined && viewerId !== undefined && agent.owner_id === viewerId;
      return {
        id: act.id,
        name: agent?.name ?? '',
        at: act.created_at,
        mayApproveHere: act.class === 'REVERSIBLE',
        path: isOwn || isAdministrationReachable ? `/agents?tab=requests&agent=${encodeURIComponent(act.agent_id)}` : undefined,
      };
    });
}

/** What *Time today* says: today's recorded minutes, the week's, and the timer while it runs. */
export interface TimeToday {
  readonly today: number;
  readonly week: number;
  readonly running: { readonly minutes: number; readonly entryId: string | null | undefined; readonly entryTitle?: string | undefined } | undefined;
}

/** Undefined until the week is read: a zero shown before then would be a figure nobody measured. */
export function timeTodayOf(
  week: { readonly minutes: number; readonly days: readonly { readonly day: string; readonly minutes: number }[] } | undefined,
  day: string,
  timer: { readonly running: boolean; readonly elapsed_minutes?: number; readonly entry_id?: string | null; readonly entry_title?: string } | undefined,
): TimeToday | undefined {
  if (!week) return undefined;
  return {
    today: week.days.find((each) => each.day === day)?.minutes ?? 0,
    week: week.minutes,
    running: timer?.running ? { minutes: timer.elapsed_minutes ?? 0, entryId: timer.entry_id, ...(timer.entry_title ? { entryTitle: timer.entry_title } : {}) } : undefined,
  };
}
