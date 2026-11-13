// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * *My deadlines kept* on the desk home (UC-SVC-02 check 15, the owner's decision of 2026-11-12):
 * every kind of deadline over the handler's own tickets that ended in the window, worst first, at
 * most three rows and "+ n more" for the full table; the phone draws only the worst. The share is
 * `kept / ended`, the client's arithmetic over `GET /deadlines-kept`.
 */

import type { DeadlinesKept } from '@hubtask/sync-engine';

export const KEPT_SHOWN = 3;

export interface KeptRow {
  readonly kind: 'SLA' | 'OLA';
  readonly goal: 'FIRST_RESPONSE' | 'NEXT_RESPONSE' | 'RESOLUTION';
  readonly groupId: string | null;
  readonly ended: number;
  readonly kept: number;
  readonly breached: number;
  readonly share: number;
}

/** The kinds that ended in the window, the lowest share first; an equal share, more breaches first. */
export function keptRows(data: DeadlinesKept['data']): KeptRow[] {
  return data
    .filter((each) => each.ended > 0)
    .map((each) => ({
      kind: each.kind, goal: each.goal, groupId: each.group_id ?? null, ended: each.ended, kept: each.kept,
      breached: each.ended - each.kept, share: each.kept / each.ended,
    }))
    .sort((a, b) => a.share - b.share || b.breached - a.breached);
}

/** A share as a whole percent, rounded down: one breach in two hundred is 99 %, never 100 %. */
export function percentOf(share: number): number {
  return Math.floor(share * 100 + 1e-9);
}
