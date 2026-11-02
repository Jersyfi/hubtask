// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The handler's queues across a hub's desks (user-experience.md §2.2, §7.4; UC-SVC-03): *Mine*,
 * *Unassigned*, *About to breach* and *All open*, each one `POST /items:query` anchored to the hub
 * and narrowed to its desk collections, by what breaks first (`sla_due`), with its exact count.
 * Anchored per hub rather than one unanchored search: the query is the read UC-SVC-03 proves the
 * order and the count of, and a hub the reader may not read is refused rather than left out.
 */

import type { EntryType } from '@hubtask/sync-engine';

import { entryTypesOffered } from '../data/entrytypes.ts';
import { actsAsDesk } from '../portal/portal.ts';

export const DESK_QUEUES = ['mine', 'unassigned', 'breaching', 'open'] as const;
export type DeskQueue = (typeof DESK_QUEUES)[number];

/** How many of a queue's entries a home draws per hub; the rest is the desk's own list. */
export const QUEUE_SHOWN = 25;

const NARROWING: Readonly<Record<DeskQueue, readonly Record<string, unknown>[]>> = {
  // `@me` is resolved on the server (view.PlaceholderMe): the client never names who is signed in.
  mine: [{ op: 'EQ', field: 'assignee_id', value: '@me' }],
  unassigned: [{ op: 'IS_NULL', field: 'assignee_id' }],
  // A breached promise is still the handler's to keep first, so it stays in the queue it ran out in.
  breaching: [{ op: 'IN', field: 'sla_standing', value: ['AT_RISK', 'BREACHED'] }],
  open: [],
};

/** The query of one queue at one hub; undefined where the hub has no desk to look in. */
export function queueQuery(queue: DeskQueue, hubId: string, deskIds: readonly string[], size = QUEUE_SHOWN): Record<string, unknown> | undefined {
  if (deskIds.length === 0) return undefined;
  return {
    scope: { container_id: hubId, include_descendants: true },
    filter: {
      op: 'AND',
      nodes: [
        { op: 'IN', field: 'collection_id', value: [...deskIds] },
        { op: 'EQ', field: 'is_completed', value: false },
        ...NARROWING[queue],
      ],
    },
    sort: [{ field: 'sla_due', dir: 'ASC', nulls: 'LAST' }],
    page: { size },
    count: 'exact',
  };
}

/** A queue's count across hubs; undefined until every hub named has answered. */
export function queueTotal(totals: Readonly<Record<string, number | null | undefined>>, hubIds: readonly string[]): number | undefined {
  let sum = 0;
  for (const hubId of hubIds) {
    const total = totals[hubId];
    if (typeof total !== 'number') return undefined;
    sum += total;
  }
  return sum;
}

/** A hub's collections that act as a desk (UC-SVC-05): those offering a type the portal takes. */
export function desksOf(
  collections: readonly { readonly id: string; readonly parent_id?: string | null; readonly structure?: { readonly settings?: { entry_types?: Record<string, readonly string[]> } } | null }[],
  types: readonly EntryType[],
): string[] {
  return collections
    .filter((collection) => actsAsDesk(types, entryTypesOffered(collection.structure?.settings ?? undefined, 'TASK'), collection.parent_id))
    .map((collection) => collection.id);
}
