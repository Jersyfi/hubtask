// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * One onboarding order (user-experience.md §3.1): a newcomer meets one thing at a time - the
 * starter, then the base tour on what it made, then a product's first-visit mark. The tour and the
 * starter never open together.
 */

export interface Onboarding {
  /** The workspace offers its first start and nobody answered or skipped it yet. */
  readonly starterOffered: boolean;
  /** The starter's dialog is open in this tab. */
  readonly starterOpen: boolean;
}

/** Whether the base tour may start now: not while the starter is offered or open. */
export function tourMayStart(state: Onboarding): boolean {
  return !state.starterOffered && !state.starterOpen;
}
