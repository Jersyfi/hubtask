// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What counts in a collection, as words: the server counts the figure on the collection's read
 * (domain-model.md §2) and sends numbers and dates; this turns them into a message code and its
 * parameters, for the collection's head and its row in the navigation (UC-WRK-29 check 1).
 *
 * Pure and tested, because the rules that matter are here and nowhere else:
 *
 * * **A percentage only for PROGRESS** (check 2). No other kind has a share to show, and the server
 *   never answers PROGRESS for a list - a collection of one level - so a list never shows one.
 * * **Rhythm never shows zero and never names a missed day** (check 3): one mark per day with a
 *   completion and nothing for the others, and a week with none yet says it is new, with no number.
 * * **No figure is invented.** A figure the server could not count says so (P-11); a figure the
 *   reader's day has moved past is read again rather than shown as today's.
 */

import type { CollectionFigure } from '@hubtask/sync-engine';

/** A message and what goes into it. */
export interface FigureMessage {
  readonly code: string;
  readonly params: Readonly<Record<string, string | number>>;
}

/** The share done, rounded down, so that 100 % means everything is. `undefined` without leaves. */
export function shareOf(done: number, total: number): number | undefined {
  if (total <= 0) return undefined;
  return Math.floor((Math.min(done, total) / total) * 100);
}

function formatNumber(value: number, locale: string, options?: Intl.NumberFormatOptions): string {
  try {
    return new Intl.NumberFormat(locale, options).format(value);
  } catch {
    return new Intl.NumberFormat('en', options).format(value);
  }
}

const DAY_SECONDS = 86_400;

/**
 * A flow's average time, as the reader's language writes a duration: days with one decimal from a
 * day upwards, hours below - "2.1 days", "5 hours", "2,1 Tage".
 */
export function durationText(seconds: number, locale: string): string {
  const days = seconds / DAY_SECONDS;
  if (days >= 1) return formatNumber(days, locale, { style: 'unit', unit: 'day', unitDisplay: 'long', maximumFractionDigits: 1 });
  const hours = Math.max(1, Math.round(seconds / 3_600));
  return formatNumber(hours, locale, { style: 'unit', unit: 'hour', unitDisplay: 'long', maximumFractionDigits: 0 });
}

/** The share as the reader's language writes a percentage: "38%", "38 %". */
function percentText(share: number, locale: string): string {
  return formatNumber(share / 100, locale, { style: 'percent', maximumFractionDigits: 0 });
}

/**
 * What the collection's head says. `undefined` where there is nothing to say - a project with no
 * leaves yet.
 */
export function headMessage(figure: CollectionFigure, locale: string): FigureMessage | undefined {
  if (figure.unavailable) return { code: 'app.figure.unavailable', params: {} };
  switch (figure.kind) {
    case 'OPEN':
      return { code: 'app.figure.open', params: { count: figure.open ?? 0 } };
    case 'FIELD_SUM': {
      const open = figure.open ?? 0;
      if (figure.sum === undefined || figure.sum === null) return { code: 'app.figure.open', params: { count: open } };
      const sum = formatNumber(figure.sum, locale, { maximumFractionDigits: 2 });
      // "About" where some open entry carries no value: the sum is then a floor, not the total.
      const code = (figure.summed ?? 0) < open ? 'app.figure.field_sum_about' : 'app.figure.field_sum';
      return { code, params: { count: open, sum } };
    }
    case 'DUE':
      return { code: 'app.figure.due', params: { count: figure.due ?? 0 } };
    case 'RHYTHM': {
      const days = figure.days?.length ?? 0;
      if (days === 0) return { code: 'app.figure.rhythm_new_week', params: {} };
      return { code: 'app.figure.rhythm', params: { count: days } };
    }
    case 'PROGRESS': {
      const share = shareOf(figure.done ?? 0, figure.total ?? 0);
      if (share === undefined) return undefined;
      return {
        code: 'app.figure.progress',
        params: { share: percentText(share, locale), done: figure.done ?? 0, total: figure.total ?? 0 },
      };
    }
    case 'FLOW': {
      const count = figure.completed ?? 0;
      const average = figure.average_seconds;
      // No average is invented (P-11): not for a week with nothing completed, and not where none
      // of the week's completions went from the first column to the last - then the count alone.
      if (count === 0) return { code: 'app.figure.flow_none', params: {} };
      if (average === undefined || average === null) return { code: 'app.figure.short.flow', params: { count } };
      return { code: 'app.figure.flow', params: { count, duration: durationText(average, locale) } };
    }
  }
  return undefined;
}

/**
 * The flow's warning beside its figure (UC-WRK-31 check 3): how many columns hold more than their
 * limit, or nothing where none does. Reported, never a refusal.
 */
export function overLimitMessage(figure: CollectionFigure): FigureMessage | undefined {
  if (figure.unavailable || figure.kind !== 'FLOW' || (figure.over_limit ?? 0) === 0) return undefined;
  return { code: 'app.figure.flow_over_limit', params: { count: figure.over_limit ?? 0 } };
}

/**
 * What the row in the navigation says: shorter, and nothing at all where the head would say there
 * is nothing yet - a row is not where a new week is announced or a failure explained.
 */
export function rowMessage(figure: CollectionFigure, locale: string): FigureMessage | undefined {
  if (figure.unavailable) return undefined;
  switch (figure.kind) {
    case 'OPEN':
    case 'FIELD_SUM':
      return { code: 'app.figure.short.open', params: { count: figure.open ?? 0 } };
    case 'DUE':
      return { code: 'app.figure.short.due', params: { count: figure.due ?? 0 } };
    case 'RHYTHM': {
      const days = figure.days?.length ?? 0;
      return days === 0 ? undefined : { code: 'app.figure.short.rhythm', params: { count: days } };
    }
    case 'PROGRESS': {
      const share = shareOf(figure.done ?? 0, figure.total ?? 0);
      return share === undefined ? undefined : { code: 'app.figure.short.progress', params: { share: percentText(share, locale) } };
    }
    case 'FLOW':
      // A full column is what the row has room for, before the week's count.
      return (figure.over_limit ?? 0) > 0
        ? { code: 'app.figure.short.flow_over_limit', params: { count: figure.over_limit ?? 0 } }
        : { code: 'app.figure.short.flow', params: { count: figure.completed ?? 0 } };
  }
  return undefined;
}

/** The share a bar draws, for PROGRESS alone. */
export function barShare(figure: CollectionFigure): number | undefined {
  if (figure.unavailable || figure.kind !== 'PROGRESS') return undefined;
  return shareOf(figure.done ?? 0, figure.total ?? 0);
}

/** Rhythm's marks: the days with a completion, in order, and nothing for any other day. */
export function rhythmMarks(figure: CollectionFigure): readonly string[] {
  if (figure.unavailable || figure.kind !== 'RHYTHM') return [];
  return [...new Set(figure.days ?? [])].sort();
}

/** Whether the reader's day has moved past the day the figure was counted for. */
export function isStale(figure: CollectionFigure | undefined, today: string): boolean {
  return figure !== undefined && figure.today !== today;
}

/**
 * Which reads a batch of changed collections makes stale: each collection's own document where
 * one is open, and the level of collections of each hub that holds one. A collection's hub is
 * looked up, because a change record names the collection and not the hub.
 */
export function figureTargets(
  collectionIds: Iterable<string>,
  parentOf: (id: string) => string | null | undefined,
): { readonly collections: readonly string[]; readonly hubs: readonly string[] } {
  const collections = new Set<string>();
  const hubs = new Set<string>();
  for (const id of collectionIds) {
    collections.add(id);
    const hub = parentOf(id);
    if (hub) hubs.add(hub);
  }
  return { collections: [...collections], hubs: [...hubs] };
}
