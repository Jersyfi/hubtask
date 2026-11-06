// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The reasoning behind *Apps you have allowed* (UC-SYN-07/3, ADR-0096): the order of the rows,
 * the mark of a third-party app, and how recently a grant was used, said in days rather than as a
 * timestamp. Only reasoning; the screen draws the words.
 */

interface Ordered {
  readonly client_name: string;
  readonly app_platform?: string | null;
}

/** The installed apps first - they are this person's own devices - then by name. */
export function grantOrder<T extends Ordered>(grants: readonly T[]): T[] {
  return [...grants].sort((a, b) => {
    const isAppA = a.app_platform ? 0 : 1;
    const isAppB = b.app_platform ? 0 : 1;
    if (isAppA !== isAppB) return isAppA - isAppB;
    return a.client_name.localeCompare(b.client_name);
  });
}

/** First and last word's first letter: a mark, not a picture. */
export function initialsOf(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  const first = [...(words[0] ?? '')][0] ?? '';
  const last = words.length > 1 ? ([...(words[words.length - 1] ?? '')][0] ?? '') : '';
  return `${first}${last}`.toUpperCase();
}

/** When a grant was last used, in the shape the row's sentence takes. */
export type GrantUse =
  | { readonly kind: 'never' }
  | { readonly kind: 'today'; readonly at: string }
  | { readonly kind: 'days'; readonly phrase: string }
  | { readonly kind: 'day'; readonly at: string };

const DAY_MS = 86_400_000;

/** The reader's calendar day of an instant, as a count of days. */
function dayNumber(at: number): number {
  const date = new Date(at);
  return Math.floor(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) / DAY_MS);
}

/**
 * Today with its time, within the week as "yesterday" or "3 days ago" in the reader's language
 * (`Intl.RelativeTimeFormat`), and the day itself further back. Counted in calendar days on the
 * reader's clock, so last night at eleven is yesterday this morning.
 */
export function grantUse(lastUsed: string | null | undefined, locale: string, now: number = Date.now()): GrantUse {
  if (!lastUsed) return { kind: 'never' };
  const at = Date.parse(lastUsed);
  if (Number.isNaN(at)) return { kind: 'never' };
  const days = dayNumber(now) - dayNumber(at);
  if (days <= 0) return { kind: 'today', at: lastUsed };
  if (days < 7) {
    return { kind: 'days', phrase: new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(-days, 'day') };
  }
  return { kind: 'day', at: lastUsed };
}
