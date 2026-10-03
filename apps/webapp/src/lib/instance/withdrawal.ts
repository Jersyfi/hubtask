// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The withdrawal of an offered provider, as the operator's screen reads and writes it (ADR-0076).
 *
 * **A day, not a timestamp.** The operator names the day the offer ends; it ends when that day
 * begins on the operator's own clock. The workspaces read the instant the server stores and draw it
 * on theirs. The default is two weeks ahead, which is the server's own default too - said here only
 * so the field can be filled before anybody types.
 *
 * **Withdrawn is read, never stored as a state.** A row is withdrawn from the moment its date has
 * come, which is the same reading the server makes wherever the offer is read.
 */

import { instantOf, todayIn } from '../i18n/zone.ts';

/** ADR-0076 §2's default notice, in days. */
export const NOTICE_DAYS = 14;

/** The day `days` after today in `zone`, as `YYYY-MM-DD`. */
function daysAhead(zone: string, days: number, now: number): string {
  const [year = 0, month = 1, day = 1] = todayIn(zone, now).split('-').map(Number);
  return new Date(Date.UTC(year, month - 1, day + days)).toISOString().slice(0, 10);
}

/** The day two weeks from today in `zone`. */
export function defaultWithdrawalDate(zone: string, now: number = Date.now()): string {
  return daysAhead(zone, NOTICE_DAYS, now);
}

/**
 * The earliest day an announced withdrawal may name: the day after tomorrow. The server counts less
 * than a day's notice as *Withdraw now* and asks for the count, and tomorrow's midnight can be a few
 * hours away - so the first day this control offers is one that is always a day ahead.
 */
export function earliestWithdrawalDate(zone: string, now: number = Date.now()): string {
  return daysAhead(zone, 2, now);
}

/** The instant a chosen day begins in `zone`, or nothing for a value that is not a day. */
export function withdrawalMoment(date: string, zone: string): string | undefined {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) return undefined;
  return instantOf(date, '00:00', zone);
}

/** Where a row stands. */
export type WithdrawalPhase = 'offered' | 'withdrawing' | 'withdrawn';

export function withdrawalPhase(
  provider: { readonly enabled: boolean; readonly withdraw_at?: string | null },
  now: number = Date.now(),
): WithdrawalPhase {
  if (!provider.enabled) return 'withdrawn';
  if (!provider.withdraw_at) return 'offered';
  const at = Date.parse(provider.withdraw_at);
  if (Number.isNaN(at)) return 'offered';
  return now < at ? 'withdrawing' : 'withdrawn';
}
