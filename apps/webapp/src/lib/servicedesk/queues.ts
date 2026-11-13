// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The handler's queues across a hub's desks (user-experience.md §2.2, §7.4; UC-SVC-03): *Mine*,
 * *Unassigned*, *About to breach* and *All open*, each one `POST /items:query` anchored to the hub
 * and narrowed to its desk collections, by what breaks first (`sla_due`), with its exact count.
 * Anchored per hub rather than one unanchored search: the query is the read UC-SVC-03 proves the
 * order and the count of, and a hub the reader may not read is refused rather than left out.
 */

import type { HubQueueList, ResourceState } from '@hubtask/sync-engine';


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

/**
 * The desks of each hub as the server names them (`desks` of `GET /hubs/{id}/queues`); undefined
 * until every hub has answered. The home asks the server rather than its own reading of the entry
 * types, which said "no desk takes requests" beside a desk with open requests (concept §10). A hub
 * that refused the read names none.
 */
export function desksAnswered(
  answers: Readonly<Record<string, ResourceState<HubQueueList> | undefined>>,
  hubIds: readonly string[],
): Record<string, string[]> | undefined {
  const out: Record<string, string[]> = {};
  for (const hubId of hubIds) {
    const answer = answers[hubId];
    if (answer === undefined || answer.status === 'idle' || answer.status === 'loading') return undefined;
    out[hubId] = answer.status === 'ready' ? [...(answer.data.desks ?? [])] : [];
  }
  return out;
}

/**
 * An entry's priority as a queue shows it: the value of the first priority field the policies
 * name that the entry answers, else of the shipped desks' `priority`; only a choice's text.
 */
export function priorityOf(item: { readonly custom_fields?: Record<string, unknown> | null }, fields: readonly (string | null | undefined)[]): string | undefined {
  for (const field of [...fields, 'priority']) {
    const value = field ? item.custom_fields?.[field] : undefined;
    if (typeof value === 'string' && value !== '') return value;
  }
  return undefined;
}

const PRIORITY_WORDS: Readonly<Record<string, 1 | 2 | 3 | 4>> = { highest: 1, critical: 1, urgent: 1, high: 2, medium: 3, normal: 3, low: 4, lowest: 4 };

/** A priority's level for its mark: by the number it carries (*P2*), else by its word; else medium. */
export function priorityLevel(value: string): 1 | 2 | 3 | 4 {
  const number = /[1-4]/u.exec(value)?.[0];
  if (number) return Number(number) as 1 | 2 | 3 | 4;
  return PRIORITY_WORDS[value.trim().toLowerCase()] ?? 3;
}

/**
 * The rows by their status as a queue's groups (WB-4): a group stands where its first row stood,
 * so the group of what breaks first leads, and each keeps the server's order inside it.
 */
export function groupedByStatus<T>(rows: readonly T[], statusOf: (row: T) => string): T[] {
  const groups = new Map<string, T[]>();
  for (const row of rows) {
    const status = statusOf(row);
    groups.set(status, [...(groups.get(status) ?? []), row]);
  }
  return [...groups.values()].flat();
}
