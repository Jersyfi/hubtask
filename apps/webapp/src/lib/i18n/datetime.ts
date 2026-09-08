// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A moment, written the way the reader's locale writes moments.
 *
 * Not part of `format.ts`, and that is deliberate. The ICU subset there implements simple
 * arguments, `plural` and `select` and **refuses what it does not implement by name** — a `date`
 * argument type included — so a catalogue message cannot format a date and `catalogue.test.ts`
 * would turn red if one tried. A date reaches a message as text that was formatted here first,
 * which keeps the renderer's refusal honest.
 *
 * The **zone is a parameter now**, and `formatDateTime` keeps the device's only because a
 * timestamp with no zone of its own — an activity's `occurred_at`, a comment's `created_at` — is a
 * moment, and a moment is read on the clock of whoever is reading it. A *due date* is not that: it
 * carries its own zone, and `formatDue` is what draws one.
 */

/** The reader's rendering of an instant, or the raw text when it is not one. */
export function formatDateTime(iso: string, locale: string): string {
  const at = Date.parse(iso);
  // Never a blank and never `Invalid Date`: a value this cannot read is shown as it arrived, so
  // the reader sees something they can quote rather than nothing.
  if (Number.isNaN(at)) return iso;
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(at);
}

/**
 * A due date, drawn as what it is.
 *
 * An all-day date gets **no time in any locale**, which is the whole point of the flag: rendering
 * one would be showing a midnight that shifts with the viewer (`i18n-l10n.md` §4). A timed date is
 * drawn on the clock of the zone it was set in, and names that zone when it is not the reader's —
 * a date set in Berlin and read in São Paulo says which.
 */
export function formatDue(
  iso: string,
  locale: string,
  zone: string,
  options: { allDay?: boolean; showZone?: boolean } = {},
): string {
  const at = Date.parse(iso);
  if (Number.isNaN(at)) return iso;

  // The zone is named only where it is worth saying: on every date it would be noise on the
  // ninety-nine that are in the reader's own.
  const namesZone = !options.allDay && options.showZone === true;

  // `dateStyle`/`timeStyle` and `timeZoneName` cannot be combined — `Intl` throws "Invalid option"
  // rather than ignoring one, which is how this was found. So the variant that names the zone
  // spells its components out, and the two that do not keep the styles, which are what let each
  // locale choose its own order and separators.
  return new Intl.DateTimeFormat(
    locale,
    namesZone
      ? {
          timeZone: zone,
          year: 'numeric',
          month: 'short',
          day: 'numeric',
          hour: 'numeric',
          minute: '2-digit',
          timeZoneName: 'short',
        }
      : {
          timeZone: zone,
          dateStyle: 'medium',
          ...(options.allDay ? {} : { timeStyle: 'short' }),
        },
  ).format(at);
}

/** The units a relative phrase is built from, largest first. */
const UNITS: readonly [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 86_400_000],
  ['month', 30 * 86_400_000],
  ['week', 7 * 86_400_000],
  ['day', 86_400_000],
  ['hour', 3_600_000],
  ['minute', 60_000],
];

/**
 * How far away it is, in the reader's language — "in 3 days", "2 hours ago".
 *
 * Through `Intl.RelativeTimeFormat`, which knows the plural rules and the wording of every locale
 * the platform has. A phrase assembled from a number and a word here would be right in English and
 * wrong in Polish, which is the reason none of this client's sentences are assembled.
 *
 * It is an **addition to the date, never a replacement**: "in 3 days" does not say which day, and
 * somebody planning needs the date. The caller draws both.
 */
export function formatRelative(iso: string, locale: string, now: number = Date.now()): string {
  const at = Date.parse(iso);
  if (Number.isNaN(at)) return iso;

  const difference = at - now;
  const format = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  for (const [unit, size] of UNITS) {
    if (Math.abs(difference) >= size) {
      return format.format(Math.round(difference / size), unit);
    }
  }
  return format.format(Math.round(difference / 1000), 'second');
}
