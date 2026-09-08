// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a change record makes stale, in this application's own paths.
 *
 * **The engine does not learn what a hub is.** It reads `entity`, `entity_id`, `container_id` and
 * `op`, hands them here, and invalidates what comes back — so this function is the one place that
 * knows a comment lives under `/items/{id}/comments` and a label under a container. That split is
 * `packages/sync-engine/CLAUDE.md`'s: the seam is the network, the paths are the product.
 *
 * **A record is a signal to re-read, never data to apply.** Nothing here touches `payload`. Applying
 * it would be a merge, and merging is the server's (ADR-0021, `offline-sync.md` §4) — which is also
 * why a record for something this client does not draw returns nothing at all rather than an
 * invalidation of everything.
 *
 * **Two names for an entry, deliberately.** The change log records a work item as `item`
 * (`CreateWorkItem.recordChange`) and the purge records one as `work_item` (`Purge.go`). Both are
 * accepted here rather than one being picked: a client that guessed would go blind for whichever
 * writer it guessed against, and being tolerant of a name is cheaper than being wrong about one.
 */

import type { ChangeRecord } from '@hubtask/sync-engine';

/** The paths a change to an entry makes stale. */
function itemPaths(id: string, containerId: string | null | undefined): readonly string[] {
  return [
    // Every list and board of the collection it is in, and the entry's own document. `/items` is
    // the prefix both share, and the engine matches by prefix — so one name covers the level read,
    // the board's query and `/items/{id}` together.
    '/items',
    // Its history, which is a different path and a different question: a change made anywhere is a
    // step somebody may be reading.
    `/items/${id}/activity`,
    ...(containerId ? [`/containers/${containerId}`] : []),
  ];
}

/**
 * What this record makes stale. Empty means "this changes nothing this client is showing".
 *
 * Empty is a real answer and the common one: a record about an entity this version does not draw
 * should cost nothing, and invalidating everything "to be safe" would turn one person's edit into
 * a reload of every screen in the workspace.
 */
export function pathsFor(record: ChangeRecord): readonly string[] {
  const id = record.entity_id;
  const container = record.container_id;

  switch (record.entity) {
    case 'item':
    case 'work_item':
      return itemPaths(id, container);

    case 'container':
      // The tree, and the entries under it: a renamed collection is a sidebar row and a heading.
      return ['/containers', '/items'];

    case 'comment':
      // The thread it is in. `container_id` is the collection, so the entry is not in the record —
      // and a thread is read at `/items/{itemId}/comments`, which this record cannot name.
      // Invalidating every thread is right and cheap: only the open one is watched.
      return ['/items'];

    case 'label':
    case 'bucket':
      // Both belong to a collection and both are drawn on the entries in it.
      return container ? [`/containers/${container}`, '/items'] : ['/containers', '/items'];

    case 'reminder':
      return [`/items/${id}/reminders`, '/items'];

    case 'recurrence_rule':
      return ['/items'];

    case 'media_object':
      return [`/media/${id}`, '/items'];

    case 'membership':
    case 'group':
      return ['/memberships', '/groups'];

    case 'custom_field_definition':
      return ['/custom-fields', '/items'];

    case 'saved_view':
      return ['/views'];

    case 'template':
      return ['/templates'];

    case 'account':
      return [`/accounts/${id}`];

    default:
      // A newer server's entity. Nothing here draws it, so nothing here is stale — and a client
      // that reloaded everything on a name it had never met would punish the installation for
      // being ahead of it.
      return [];
  }
}

/**
 * The container a record says access was revoked to, or nothing when it is not that kind.
 *
 * `offline-sync.md` §6 and §9 rule 3 bind a client to **delete what it holds** for a container it
 * lost. This client holds a cache and nothing more, so "delete" is "drop it and stop showing it" —
 * but the obligation is the same one, and pretending a record we cannot act on is only a hint would
 * be reading the rule as advice.
 */
export function revokedContainerOf(record: ChangeRecord): string | undefined {
  if (record.op !== 'ACCESS_REVOKED') return undefined;
  // The container itself is the subject when the entity is a container; otherwise the record names
  // the container the lost thing lived in.
  if (record.entity === 'container') return record.entity_id;
  return record.container_id ?? undefined;
}
