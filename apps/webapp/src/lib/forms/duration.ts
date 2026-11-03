// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A duration held in minutes and written in minutes or hours (user-experience.md §5.4,
 * `DurationInput`). The server's field stays minutes; the conversion is the client's, so nobody
 * has to work out that a four-hour goal is 240.
 */

export type DurationUnit = 'MINUTES' | 'HOURS';
export const DURATION_UNITS: readonly DurationUnit[] = ['MINUTES', 'HOURS'];

const FACTOR: Readonly<Record<DurationUnit, number>> = { MINUTES: 1, HOURS: 60 };

/** The unit a held value is shown in: hours where it is whole hours, minutes otherwise. */
export function unitFor(minutes: string): DurationUnit {
  const value = Number(minutes);
  return minutes.trim() !== '' && Number.isInteger(value) && value > 0 && value % 60 === 0 ? 'HOURS' : 'MINUTES';
}

/** The held minutes as an amount of the unit; anything unreadable as it is. */
export function amountIn(minutes: string, unit: DurationUnit): string {
  const value = Number(minutes);
  if (minutes.trim() === '' || !Number.isFinite(value)) return minutes;
  return String(value / FACTOR[unit]);
}

/** The amount typed in the unit as minutes; empty and unreadable text are handed on unchanged. */
export function minutesOf(amount: string, unit: DurationUnit): string {
  const text = amount.trim().replace(',', '.');
  const value = Number(text);
  if (text === '' || !Number.isFinite(value)) return amount;
  return String(Math.round(value * FACTOR[unit]));
}
