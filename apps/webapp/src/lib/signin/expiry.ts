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
