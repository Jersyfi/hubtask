// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which day a week starts on (i18n-l10n.md §6 line 5, F5-09).
 *
 * Three answers, in order: the account's own `week_start`, which the person chose (M-06); the
 * installation's word for the locale, `supported_locales[].week_start` (M-05); and the locale's
 * own week information from `Intl.Locale`, which is what a browser knows about `en-US` starting
 * on Sunday without anybody saying so. Monday only when none of the three answers - which is a
 * fallback, not a decision the client made for the reader.
 */

import type { SupportedLocale } from './locale.ts';

/** A weekday as `Date` counts it: 0 for Sunday through 6 for Saturday. */
export type Weekday = 0 | 1 | 2 | 3 | 4 | 5 | 6;

const BY_NAME: Record<string, Weekday> = { SUNDAY: 0, MONDAY: 1, SATURDAY: 6 };

/** The locale's first day as `Intl` reports it, where the engine reports one. */
function localeFirstDay(locale: string): Weekday | undefined {
  try {
    const info = new Intl.Locale(locale) as Intl.Locale & {
      getWeekInfo?: () => { firstDay: number };
      weekInfo?: { firstDay: number };
    };
    const first = info.getWeekInfo?.().firstDay ?? info.weekInfo?.firstDay;
    if (first === undefined) return undefined;
    // `Intl` counts Monday as 1 through Sunday as 7; `Date` counts Sunday as 0.
    return (first % 7) as Weekday;
  } catch {
    return undefined;
  }
}

/** The first weekday of the reader's week, as `Date` counts it. */
export function firstWeekdayOf(
  accountWeekStart: string | null | undefined,
  locale: string,
  supported: readonly SupportedLocale[] = [],
): Weekday {
  if (accountWeekStart && accountWeekStart in BY_NAME) return BY_NAME[accountWeekStart]!;
  const declared = supported.find((entry) => entry.locale === locale)?.week_start;
  if (declared && declared in BY_NAME) return BY_NAME[declared]!;
  return localeFirstDay(locale) ?? 1;
}

/** The same answer as the RFC 5545 two-letter key a recurrence rule uses. */
export function weekStartKeyOf(
  accountWeekStart: string | null | undefined,
  locale: string,
  supported: readonly SupportedLocale[] = [],
): 'MO' | 'TU' | 'WE' | 'TH' | 'FR' | 'SA' | 'SU' {
  const keys = ['SU', 'MO', 'TU', 'WE', 'TH', 'FR', 'SA'] as const;
  return keys[firstWeekdayOf(accountWeekStart, locale, supported)];
}
