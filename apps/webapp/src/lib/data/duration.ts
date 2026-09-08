// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * ISO-8601 durations, as this API accepts them.
 *
 * **Years and months are refused**, and by two features for one reason. A reminder's offset says
 * so — "they are calendar arithmetic rather than a length of time" — and a template's `due_offset`
 * says it again in its own words: "a relative date counts in weeks, days, hours, minutes and
 * seconds. Years and months would mean a different length in every month."
 *
 * One rule, so one module. The check is a **courtesy** in both places: the server validates
 * properly, and what this must never do is refuse a duration the server would accept.
 */

/** The four units a duration is entered in, largest first. */
export const UNITS = ['W', 'D', 'H', 'M'] as const;

export type Unit = (typeof UNITS)[number];

/** Whether a duration is one this API takes: well formed, and free of calendar units. */
export function isPlainDuration(duration: string): boolean {
  const body = duration.startsWith('-') ? duration.slice(1) : duration;
  if (!body.startsWith('P') || body === 'P') return false;

  // The `M` before the `T` is months and the one after it is minutes, which is why this reads the
  // date half alone rather than the whole string.
  const [datePart] = body.slice(1).split('T');
  if (/[YM]/.test(datePart ?? '')) return false;
  return /\d/.test(body);
}

/**
 * A duration from an amount and a unit — `3` days before becomes `-P3D`.
 *
 * Minutes and hours go after the `T`, days and weeks before it, which is the grammar rather than a
 * preference: `PT3D` and `P3M` are both something other than what was meant.
 */
export function durationOf(amount: number, unit: Unit, isBefore = false): string {
  const size = Math.max(0, Math.floor(Math.abs(amount)));
  const body = unit === 'H' || unit === 'M' ? `PT${size}${unit}` : `P${size}${unit}`;
  return isBefore ? `-${body}` : body;
}

/** …and back, for a control that shows an amount and a unit. */
export function partsOf(duration: string): { amount: number; unit: Unit; isBefore: boolean } | undefined {
  if (!isPlainDuration(duration)) return undefined;
  const isBefore = duration.startsWith('-');
  const body = isBefore ? duration.slice(1) : duration;
  const match = /^P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?)?$/.exec(body);
  if (!match) return undefined;

  const [, weeks, days, hours, minutes] = match;
  // The largest unit that is actually there. A duration with two units is not one this control can
  // show, so it is left to the raw field rather than rounded into something it is not.
  if (weeks && !days && !hours && !minutes) return { amount: Number(weeks), unit: 'W', isBefore };
  if (days && !weeks && !hours && !minutes) return { amount: Number(days), unit: 'D', isBefore };
  if (hours && !weeks && !days && !minutes) return { amount: Number(hours), unit: 'H', isBefore };
  if (minutes && !weeks && !days && !hours) return { amount: Number(minutes), unit: 'M', isBefore };
  return undefined;
}
