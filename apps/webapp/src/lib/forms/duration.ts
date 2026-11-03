// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A duration held in minutes and written in minutes, hours or business days (user-experience.md
 * §5.4, `DurationInput`). The server's field stays minutes; the conversion is the client's, so
 * nobody has to work out that a four-hour goal is 240. A business day is as long as the caller
 * says - a calendar's working minutes on a day - and is offered only when it is given.
 */

export type DurationUnit = 'MINUTES' | 'HOURS' | 'DAYS';

/** The units offered: business days only with a day length. */
export function unitsFor(day?: number): DurationUnit[] {
  return day && day > 0 ? ['MINUTES', 'HOURS', 'DAYS'] : ['MINUTES', 'HOURS'];
}

function factor(unit: DurationUnit, day?: number): number {
  if (unit === 'HOURS') return 60;
  if (unit === 'DAYS' && day && day > 0) return day;
  return 1;
}

/** The unit a held value is shown in: the largest of those offered that it fills exactly. */
export function unitFor(minutes: string, day?: number): DurationUnit {
  const value = Number(minutes);
  if (minutes.trim() === '' || !Number.isInteger(value) || value <= 0) return 'MINUTES';
  const fits = unitsFor(day).filter((unit) => value % factor(unit, day) === 0);
  return fits[fits.length - 1] ?? 'MINUTES';
}

/** The held minutes as an amount of the unit; anything unreadable as it is. */
export function amountIn(minutes: string, unit: DurationUnit, day?: number): string {
  const value = Number(minutes);
  if (minutes.trim() === '' || !Number.isFinite(value)) return minutes;
  return String(value / factor(unit, day));
}

/** The amount typed in the unit as minutes; empty and unreadable text are handed on unchanged. */
export function minutesOf(amount: string, unit: DurationUnit, day?: number): string {
  const text = amount.trim().replace(',', '.');
  const value = Number(text);
  if (text === '' || !Number.isFinite(value)) return amount;
  return String(Math.round(value * factor(unit, day)));
}
