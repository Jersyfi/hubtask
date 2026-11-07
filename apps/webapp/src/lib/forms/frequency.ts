// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A job's schedule as choices - daily, weekly on a day, monthly on a date, at a time - and the
 * RFC 5545 rule they stand for (user-experience.md §5.4, `FrequencyPicker`).
 *
 * Reading is strict on purpose: a rule is read into the choices only when writing the choices back
 * gives the same rule. Anything the choices cannot say (an interval, two days, a count, no fixed
 * hour) stays the raw rule, never quietly rewritten into a different schedule. The occurrences are
 * the server's; nothing here expands a rule.
 */

export type Weekday = 'MO' | 'TU' | 'WE' | 'TH' | 'FR' | 'SA' | 'SU';
export const WEEKDAYS: readonly Weekday[] = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'];

export type FrequencyKind = 'DAILY' | 'WEEKLY' | 'MONTHLY';
export const FREQUENCIES: readonly FrequencyKind[] = ['DAILY', 'WEEKLY', 'MONTHLY'];

export interface Frequency {
  readonly freq: FrequencyKind;
  /** The day of a weekly schedule; carried, unused, by the others. */
  readonly weekday: Weekday;
  /** The date of a monthly schedule; carried, unused, by the others. */
  readonly monthDay: number;
  readonly hour: number;
  readonly minute: number;
}

const whole = (text: string | undefined, low: number, high: number): number | undefined => {
  if (text === undefined || !/^\d{1,2}$/u.test(text)) return undefined;
  const value = Number(text);
  return value >= low && value <= high ? value : undefined;
};

/** The choices a rule stands for, or undefined when they cannot say it exactly. */
export function frequencyOf(rule: string): Frequency | undefined {
  const parts = new Map<string, string>();
  for (const part of rule.trim().split(';')) {
    const [key, value, ...extra] = part.split('=');
    if (!key || value === undefined || extra.length > 0 || parts.has(key.toUpperCase())) return undefined;
    parts.set(key.toUpperCase(), value.toUpperCase());
  }
  const freq = parts.get('FREQ');
  if (freq !== 'DAILY' && freq !== 'WEEKLY' && freq !== 'MONTHLY') return undefined;
  const hour = whole(parts.get('BYHOUR'), 0, 23);
  const minute = whole(parts.get('BYMINUTE'), 0, 59);
  if (hour === undefined || minute === undefined) return undefined;
  const interval = parts.get('INTERVAL');
  if (interval !== undefined && interval !== '1') return undefined;

  const allowed = new Set(['FREQ', 'BYHOUR', 'BYMINUTE', 'INTERVAL', ...(freq === 'WEEKLY' ? ['BYDAY'] : []), ...(freq === 'MONTHLY' ? ['BYMONTHDAY'] : [])]);
  if ([...parts.keys()].some((key) => !allowed.has(key))) return undefined;

  let weekday: Weekday = 'MO';
  let monthDay = 1;
  if (freq === 'WEEKLY') {
    const day = parts.get('BYDAY') as Weekday | undefined;
    if (!day || !WEEKDAYS.includes(day)) return undefined;
    weekday = day;
  }
  if (freq === 'MONTHLY') {
    const day = whole(parts.get('BYMONTHDAY'), 1, 31);
    if (day === undefined) return undefined;
    monthDay = day;
  }
  return { freq, weekday, monthDay, hour, minute };
}

/** The rule the choices stand for. */
export function ruleOf(choice: Frequency): string {
  const at = `BYHOUR=${choice.hour};BYMINUTE=${choice.minute}`;
  if (choice.freq === 'WEEKLY') return `FREQ=WEEKLY;BYDAY=${choice.weekday};${at}`;
  if (choice.freq === 'MONTHLY') return `FREQ=MONTHLY;BYMONTHDAY=${choice.monthDay};${at}`;
  return `FREQ=DAILY;${at}`;
}
