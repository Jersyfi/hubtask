// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A duration typed the way a person writes one, carried as whole minutes (a `DURATION` field).
 *
 * Hours and minutes as `1:30` - a spelling every locale reads the same, unlike unit words - and a
 * whole number alone as minutes. A decimal in the reader's notation is hours (`1,5` under `de` is
 * ninety minutes), because nobody writes half a minute. Anything else is `undefined`, which the
 * field shows as not a duration rather than sending.
 */

import { parseDecimal } from './number.ts';

/** The longest duration the contract accepts: ten years in minutes. */
const MAX_MINUTES = 10 * 366 * 24 * 60;

/** Whole minutes as `h:mm`. */
export function formatDuration(minutes: number): string {
  const whole = Math.max(0, Math.round(minutes));
  const hours = Math.floor(whole / 60);
  const rest = whole % 60;
  return `${hours}:${rest < 10 ? '0' : ''}${rest}`;
}

/** The minutes a person typed, `null` for an empty field, `undefined` for text that is not one. */
export function parseDuration(text: string, separator: string): number | null | undefined {
  const trimmed = text.trim();
  if (trimmed === '') return null;

  const clock = /^(\d+):([0-5]\d)$/.exec(trimmed);
  if (clock) return bounded(Number(clock[1]) * 60 + Number(clock[2]));

  if (/^\d+$/.test(trimmed)) return bounded(Number(trimmed));

  const hours = parseDecimal(trimmed, separator);
  if (hours === null || hours === undefined || hours < 0) return undefined;
  return bounded(Math.round(hours * 60));
}

function bounded(minutes: number): number | undefined {
  return Number.isSafeInteger(minutes) && minutes >= 0 && minutes <= MAX_MINUTES ? minutes : undefined;
}
