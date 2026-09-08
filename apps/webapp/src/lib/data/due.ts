// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The due date, both ways across the wire, and the window a timeline draws.
 *
 * **Three fields, one date.** `due_at`, `due_date_only` and `due_time_zone` are written together
 * and cleared together, because "none of them means anything alone" — so this converts the three
 * into the one value `DueDateControl` takes, and back, and never reads one without the others.
 *
 * **An all-day date is a date in its zone.** It is stored as an instant like any other, and the
 * flag is what says the instant is to be read as a day rather than a moment. A client that
 * rendered a time for it would be showing a midnight that shifts with the viewer, which is the
 * defect `i18n-l10n.md` §4 is about.
 *
 * **Overdue is a fact about an instant, not about a day.** It compares instants, so an entry due at
 * 17:00 in Berlin is overdue for a reader in São Paulo at the same moment it is overdue in Berlin —
 * and an all-day date is not overdue until its day is over *in its own zone*.
 */

import type { DueDate } from '@hubtask/design-system/components';
import type { WorkItem } from '@hubtask/sync-engine';

import { dateIn, instantOf, timeIn } from '../i18n/zone.ts';

/** What the three fields say, as the one value a control takes. `null` when there is no due date. */
export function dueOf(item: WorkItem | undefined, readerZone: string): DueDate | null {
  const at = item?.due_at;
  if (!at) return null;

  // The entry's own zone decides what date this is. Without one it is the reader's, which is what
  // the contract means by a null `due_time_zone`.
  const zone = item?.due_time_zone ?? readerZone;
  const date = dateIn(at, zone);
  if (!date) return null;

  return {
    date,
    // Absent means all-day — never `00:00`, which is a different statement.
    time: item?.due_date_only ? undefined : timeIn(at, zone),
    // Absent means "the reader's", so an entry in their own zone carries none and the control
    // draws no note about it.
    zone: zone === readerZone ? undefined : zone,
  };
}

/** The three fields, from the one value. The zone is always stated, even when it is the reader's. */
export function dueInputFor(
  value: DueDate,
  readerZone: string,
): { due_at: string; due_date_only: boolean; due_time_zone: string } | undefined {
  const zone = value.zone ?? readerZone;
  const at = instantOf(value.date, value.time, zone);
  if (!at) return undefined;
  return { due_at: at, due_date_only: value.time === undefined, due_time_zone: zone };
}

/**
 * Whether the entry is past its due date, and not finished.
 *
 * A completed entry is never overdue: the date said when it was wanted, and it arrived. An all-day
 * date is overdue once its day has ended in **its own zone**, which is a different instant from
 * the end of the reader's day.
 */
export function isOverdue(item: WorkItem, readerZone: string, now: number): boolean {
  if (item.completion?.is_completed) return false;
  const at = item.due_at;
  if (!at) return false;

  if (!item.due_date_only) return Date.parse(at) < now;

  const zone = item.due_time_zone ?? readerZone;
  const date = dateIn(at, zone);
  if (!date) return false;
  // The end of that day where the date lives: the instant the next day begins there.
  const endOfDay = instantOf(nextDay(date), undefined, zone);
  return endOfDay !== undefined && Date.parse(endOfDay) <= now;
}

/** Whether it falls due within the window, and is not overdue or finished already. */
export function isDueSoon(item: WorkItem, readerZone: string, now: number, withinMs: number): boolean {
  if (item.completion?.is_completed || !item.due_at) return false;
  if (isOverdue(item, readerZone, now)) return false;
  const at = Date.parse(item.due_at);
  return !Number.isNaN(at) && at - now <= withinMs;
}

// --- days and windows -------------------------------------------------------------------------
//
// All of it in `YYYY-MM-DD` and all of it through `Date.UTC`, deliberately: a day is a label here
// rather than a moment, and arithmetic that went through local time would move a window because
// the machine is west of Greenwich.

const DAY = 86_400_000;

const asDay = (date: string) => Date.parse(`${date}T00:00:00Z`);
const asDate = (instant: number) => new Date(instant).toISOString().slice(0, 10);

/** The day after. What "the end of this day" is measured from. */
export function nextDay(date: string): string {
  return asDate(asDay(date) + DAY);
}

/** A day some number of days away, which may be negative. */
export function addDays(date: string, days: number): string {
  return asDate(asDay(date) + days * DAY);
}

/** How many days from the first to the second, inclusive of neither end. */
export function daysBetween(from: string, to: string): number {
  return Math.round((asDay(to) - asDay(from)) / DAY);
}

/** A window a timeline draws, inclusive at both ends. */
export interface Window {
  readonly from: string;
  readonly to: string;
}

/** Which day of the week a date is, 0 for Sunday, as every `Date` counts it. */
const weekdayOf = (date: string) => new Date(asDay(date)).getUTCDay();

/** What the account's `week_start` means as a weekday number. Monday where it says nothing. */
export function firstWeekday(weekStart: string | null | undefined): number {
  if (weekStart === 'SUNDAY') return 0;
  if (weekStart === 'SATURDAY') return 6;
  // MONDAY, and the fallback. The contract's enum has three values and the field is nullable; a
  // client that guessed Sunday for a null would be picking one of the three at random.
  return 1;
}

/**
 * The week a date is in, starting on the account's first day.
 *
 * `week_start` is what the account field has been for since F1, and this is the first thing that
 * reads it: a week that always began on Monday would be a week this client had decided on.
 */
export function weekOf(date: string, weekStart: string | null | undefined): Window {
  const first = firstWeekday(weekStart);
  const back = (weekdayOf(date) - first + 7) % 7;
  const from = addDays(date, -back);
  return { from, to: addDays(from, 6) };
}

/** The calendar month a date is in, whole. */
export function monthOf(date: string): Window {
  const from = `${date.slice(0, 7)}-01`;
  // The last day of the month is the day before the first of the next, which needs no table of
  // month lengths and is right in a leap year without knowing that it is one.
  const [year, month] = [Number(date.slice(0, 4)), Number(date.slice(5, 7))];
  const firstOfNext = month === 12 ? `${year + 1}-01-01` : `${year}-${String(month + 1).padStart(2, '0')}-01`;
  return { from, to: addDays(firstOfNext, -1) };
}

/** The same window, moved by its own length. What "next" and "previous" mean on a timeline. */
export function shift(window: Window, direction: 1 | -1): Window {
  const span = daysBetween(window.from, window.to) + 1;
  return { from: addDays(window.from, span * direction), to: addDays(window.to, span * direction) };
}

/** Whether a day is inside a window, both ends included. */
export function contains(window: Window, date: string): boolean {
  return date >= window.from && date <= window.to;
}
