// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A file size, written the way the reader's locale writes one.
 *
 * Here rather than in `format.ts` for the reason `datetime.ts` is: the ICU subset refuses argument
 * types it does not implement, `number` with a unit skeleton among them, so a size reaches a
 * message as text that was formatted here first. A catalogue that could not format a size is what
 * keeps that refusal honest.
 *
 * **Decimal, not binary.** A kilobyte here is a thousand bytes, because `Intl` names the units
 * `kilobyte` and `megabyte` and those names mean powers of ten in every locale it knows. Showing
 * `1.0 MB` for 1 048 576 bytes under a name that means 1 000 000 would be the client disagreeing
 * with its own unit, and the operating systems the reader compares against — macOS, iOS, Android —
 * count the same way.
 */

const UNITS = ['byte', 'kilobyte', 'megabyte', 'gigabyte', 'terabyte'] as const;

/**
 * The size as one phrase — "2.4 MB", "980 kB", "3 bytes".
 *
 * Bytes are whole and everything above them gets one decimal: a file is either 3 bytes or it is
 * not, and "2.43 MB" is precision nobody asked for about a number that is about to change.
 */
export function formatBytes(bytes: number, locale: string): string {
  if (!Number.isFinite(bytes) || bytes < 0) return String(bytes);

  let value = bytes;
  let unit = 0;
  while (value >= 1000 && unit < UNITS.length - 1) {
    value /= 1000;
    unit += 1;
  }

  return new Intl.NumberFormat(locale, {
    style: 'unit',
    unit: UNITS[unit],
    unitDisplay: 'short',
    maximumFractionDigits: unit === 0 ? 0 : 1,
  }).format(value);
}
