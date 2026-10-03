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

/**
 * Why *Remove* waits, or nothing where it may act (ADR-0077 §2): removing deletes every connection
 * between a person and the provider, and offering it again does not restore them - so a provider
 * still offered and used goes only after its withdrawal. While none is announced the answer is to
 * withdraw it first; while one is, it is the day the offer ends. The server asks the same and
 * refuses with `identity_provider.withdraw_first` or `identity_provider.remove_after_withdrawal`.
 */
export type RemovalWait = 'withdraw_first' | 'after_withdrawal';

export function removalWait(
  provider: {
    readonly enabled: boolean;
    readonly withdraw_at?: string | null;
    readonly offered_workspaces?: number | null;
  },
  now: number = Date.now(),
): RemovalWait | undefined {
  if ((provider.offered_workspaces ?? 0) === 0) return undefined;
  switch (withdrawalPhase(provider, now)) {
    case 'offered':
      return 'withdraw_first';
    case 'withdrawing':
      return 'after_withdrawal';
    default:
      return undefined;
  }
}
