// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a timeline is a picture of: a scale, the window it draws, and what a drag across it means.
 *
 * All of it out here rather than in the component, for the reason `reorder.ts` gives about a rank
 * change: "how many days is that" and "which window shows the work" are questions about numbers,
 * and a component that answered them inline could only be checked by driving a browser.
 *
 * **A scale is how far you are looking and how finely it is ruled, together.** Day rules every
 * day and shows a fortnight; week rules every week and shows eight; month rules every month and
 * shows six. One control, because the two are one question — a fortnight ruled by months says
 * nothing and half a year ruled by days is unreadable.
 *
 * **The window opens where the work is, not on the current month.** The collection walked on
 * 2026-09-22 had two dated entries, one on 25 September and one on 2 October, and the timeline
 * opened on 1-30 September: the first was drawn and the second was outside the window with
 * nothing saying so (ADR-0063 decision 12). So the anchor is today where today's window holds any
 * of the work, and otherwise the dated day nearest to it.
 *
 * Every date here is `YYYY-MM-DD` and every sum goes through `due.ts`'s UTC arithmetic: a day is a
 * label rather than a moment, and a window that moved because the machine is west of Greenwich
 * would be a window nobody asked for.
 */

import { addDays, contains, daysBetween, monthOf, weekOf, type Window } from './due.ts';

/** How far the timeline looks, and how finely it is ruled. */
export type Scale = 'day' | 'week' | 'month';

export const SCALES: readonly Scale[] = ['day', 'week', 'month'];

/** Whether a word is one of the three. Anything else is the middle one. */
export function scaleOf(value: string | null | undefined): Scale {
  return (SCALES as readonly string[]).includes(value ?? '') ? (value as Scale) : 'week';
}

/** The first day of the month a date is in. */
const firstOfMonth = (date: string) => `${date.slice(0, 7)}-01`;

/** The first of the month `count` months along, which may be negative. */
function monthsFrom(date: string, count: number): string {
  const year = Number(date.slice(0, 4));
  const month = Number(date.slice(5, 7)) - 1 + count;
  const shifted = new Date(Date.UTC(year, month, 1));
  return shifted.toISOString().slice(0, 10);
}

/**
 * The window a scale draws around a day.
 *
 * Each begins before the anchor, because an entry is usually read from what is already running
 * rather than from what starts today: a window whose first column is the anchor would put the work
 * in progress off its left edge.
 */
export function windowOf(anchor: string, scale: Scale, weekStart: number): Window {
  if (scale === 'day') {
    const from = addDays(anchor, -3);
    return { from, to: addDays(from, 13) };
  }
  if (scale === 'week') {
    const from = weekOf(addDays(anchor, -7), weekStart).from;
    return { from, to: addDays(from, 8 * 7 - 1) };
  }
  const from = monthsFrom(firstOfMonth(anchor), -1);
  // The last day of the sixth month is the day before the first of the seventh, which needs no
  // table of month lengths and is right in a leap year without knowing that it is one.
  return { from, to: addDays(monthsFrom(from, 6), -1) };
}

/**
 * The day the window opens around: today, or the work.
 *
 * Nearest rather than earliest, and the earlier of two equals: a collection whose work is all
 * behind it opens on the end of it, and one whose work is all ahead opens on the beginning.
 */
export function anchorOf(days: readonly string[], today: string, scale: Scale, weekStart: number): string {
  if (days.length === 0) return today;
  if (days.some((day) => contains(windowOf(today, scale, weekStart), day))) return today;
  return [...days].sort((left, right) => {
    const distance = Math.abs(daysBetween(today, left)) - Math.abs(daysBetween(today, right));
    return distance !== 0 ? distance : left < right ? -1 : 1;
  })[0];
}

/**
 * The days inside the window that carry a gridline: every day, week or month, by the scale.
 *
 * The week's lines sit on the reader's own first weekday, so the ruling agrees with the weeks they
 * count in. The dates are answered here and worded by the caller — how a month is spelled is a
 * question this module answers in no language (ADR-0011).
 */
export function gridOf(window: Window, scale: Scale, weekStart: number): readonly string[] {
  const days: string[] = [];
  for (let day = window.from; day <= window.to; day = addDays(day, 1)) {
    if (scale === 'day') days.push(day);
    else if (scale === 'week') {
      if (weekOf(day, weekStart).from === day) days.push(day);
    } else if (firstOfMonth(day) === day) days.push(day);
  }
  // A window that begins mid-week or mid-month would otherwise carry no line until the next one,
  // and a scale with no line at its own left edge does not say where it starts.
  if (days[0] !== window.from) days.unshift(window.from);
  return days;
}

/**
 * How many days a pointer travelled sideways, at a column width. Zero where nothing is known.
 *
 * Rounded away from zero on a half, not upwards: `Math.round(-1.5)` is `-1` and `Math.round(1.5)`
 * is `2`, so the plain call would move a bar a different distance depending on which way it was
 * dragged. A gesture that is not symmetric is one the reader cannot learn.
 */
export function daysMoved(travelled: number, columnWidth: number): number {
  if (!Number.isFinite(travelled) || !Number.isFinite(columnWidth) || columnWidth <= 0) return 0;
  const columns = travelled / columnWidth;
  return Math.sign(columns) * Math.round(Math.abs(columns));
}

/** What a drag takes hold of: the whole bar, or one of its ends. */
export type Grab = 'bar' | 'start' | 'due';

/** The two dates an entry is drawn from. Absent is "not set", never an empty string. */
export interface Span {
  readonly start?: string;
  readonly due?: string;
}

/**
 * Where a drag of `days` leaves the span it took hold of.
 *
 * The bar moves both dates and keeps its length; an end moves one and may not pass the other,
 * because a due date before its start is a span with no reading. A date that is not set is not
 * invented: dragging the bar of an entry that has only a due date moves the due date, and the
 * entry still has no start.
 */
export function draggedTo(span: Span, grab: Grab, days: number): Span {
  if (days === 0) return span;
  if (grab === 'bar') {
    return {
      start: span.start === undefined ? undefined : addDays(span.start, days),
      due: span.due === undefined ? undefined : addDays(span.due, days),
    };
  }
  if (grab === 'start') {
    if (span.start === undefined) return span;
    const moved = addDays(span.start, days);
    return { ...span, start: span.due !== undefined && moved > span.due ? span.due : moved };
  }
  if (span.due === undefined) return span;
  const moved = addDays(span.due, days);
  return { ...span, due: span.start !== undefined && moved < span.start ? span.start : moved };
}

/** The first dates a drag across the axis gives an entry that had none: whichever way it was drawn. */
export function placedBetween(from: string, to: string): Span {
  return from <= to ? { start: from, due: to } : { start: to, due: from };
}
