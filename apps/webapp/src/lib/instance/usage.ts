// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The installation's *Usage* (user-experience.md §8.4) as the screen reads it: each workspace's
 * latest day of people, entries and stored bytes from `GET /admin/usage`. Counts, never content.
 */

import type { WorkspaceUsage } from '@hubtask/sync-engine';

/** The metrics the screen draws, the quota's own names, in the order of its columns. */
export const USAGE_COLUMNS = ['accounts', 'items', 'media_bytes'] as const;
export type UsageColumn = (typeof USAGE_COLUMNS)[number];

/** How many days back the screen asks for; the latest is what a row shows. */
export const USAGE_DAYS = 30;

/** The range the screen asks for, as the contract's dates, ending on today's UTC day. */
export function usageRange(now: number = Date.now()): { readonly from: string; readonly to: string } {
  const day = (at: number) => new Date(at).toISOString().slice(0, 10);
  return { from: day(now - (USAGE_DAYS - 1) * 86_400_000), to: day(now) };
}

/** The latest day's value of each drawn metric; absent where the answer carries none. */
export function latestOf(usage: WorkspaceUsage): Partial<Record<UsageColumn, number>> {
  const latest: Partial<Record<UsageColumn, number>> = {};
  for (const name of USAGE_COLUMNS) {
    const days = usage.metrics?.[name] ?? [];
    const last = [...days].sort((a, b) => a.day.localeCompare(b.day)).at(-1);
    if (last) latest[name] = last.value;
  }
  return latest;
}
