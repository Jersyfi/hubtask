// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * How long a pending credential still has, as a clock somebody can read.
 *
 * Pure, and therefore testable without a browser: the instant comes from the caller so the same
 * function answers a tick, a test and a screenshot. Minutes and seconds rather than a sentence,
 * because that is the one number shape that needs no translation - and it is written with
 * `tabular-nums` where it is drawn, so it does not jitter as it counts down.
 */
export function remaining(expiresAt: string, now: number): string | undefined {
  const at = Date.parse(expiresAt);
  if (Number.isNaN(at)) return undefined;
  const seconds = Math.max(0, Math.floor((at - now) / 1000));
  const minutes = Math.floor(seconds / 60);
  return `${minutes}:${String(seconds % 60).padStart(2, '0')}`;
}

/**
 * Where the wait stands (UC-ID-02 check 2): running, in its last minute, or over.
 *
 * The last minute is when the line under the countdown says the step is about to end - early
 * enough to type a code, late enough not to hurry somebody who has four minutes. Over is zero
 * and after: the server refuses the credential from that instant, so the card does not offer a
 * step it cannot finish.
 */
export type Phase = 'running' | 'closing' | 'over';

/** The last minute, in seconds. */
export const CLOSING_SECONDS = 60;

export function phaseOf(expiresAt: string, now: number): Phase | undefined {
  const at = Date.parse(expiresAt);
  if (Number.isNaN(at)) return undefined;
  const left = Math.floor((at - now) / 1000);
  if (left <= 0) return 'over';
  return left <= CLOSING_SECONDS ? 'closing' : 'running';
}
