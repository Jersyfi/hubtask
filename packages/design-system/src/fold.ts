// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A long section folds itself (work-surface concept ET-3): a description from twelve lines, links
// from five per group, files from twenty, comments to the newest three. The arithmetic of which
// rows stay in sight is here, where a test can hold it; `FoldingSection` only draws it.

/** Which end of a list stays in sight while it is folded: the start, or the newest at the end. */
export type FoldFrom = 'start' | 'end';

/** The rows in sight, `[start, end)`, and how many are folded away. */
export interface FoldWindow {
  readonly start: number;
  readonly end: number;
  readonly hidden: number;
}

/**
 * The window of a list of `total` rows folded to `limit`. Open, or short enough, it is the whole
 * list. A fold that would hide a single row hides nothing: *Show all 1* costs the reader a press
 * to save one line.
 */
export function foldWindow(total: number, limit: number, from: FoldFrom = 'start', isOpen = false): FoldWindow {
  const count = Math.max(0, Math.floor(total));
  const keep = Math.max(1, Math.floor(limit));
  if (isOpen || count - keep <= 1) return { start: 0, end: count, hidden: 0 };
  return from === 'start' ? { start: 0, end: keep, hidden: count - keep } : { start: count - keep, end: count, hidden: count - keep };
}

/** The line bounds a text section may fold at; the stylesheet has a clamp for each. */
export const FOLD_LINES = [3, 5, 8, 12] as const;
export type FoldLines = (typeof FOLD_LINES)[number];
