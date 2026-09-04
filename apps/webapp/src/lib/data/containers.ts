// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The two questions about a container that are worth answering away from a component.
 *
 * It was three. A `groupByHub` lived here as well, on the assumption that `GET /containers`
 * answers hubs and collections together — it does not. `ListContainers` reads **one level**: an
 * empty `parent_id` is the hubs, a named one is that hub's collections, and the two ask different
 * permission questions, which is why no read answers both. The store fetches a level at a time and
 * there is no flat list left to group, so the function and its tests went with the assumption.
 */

import type { Container } from '@hubtask/sync-engine';

/** Where a container stands in the archive, which is **two** questions rather than one. */
export type Archival =
  /** Live and writable. */
  | 'active'
  /** Archived in its own right. Unarchiving it is what brings it back. */
  | 'archived'
  /** Live itself, read-only because the hub above it is archived (I-C3). */
  | 'inherited';

/**
 * Which of the three.
 *
 * The schema separates `archived_at` from `effective_archived` precisely so a client can tell
 * "archived" from "inside an archived hub", and they are different sentences and different offers:
 * the first has an unarchive control, the second has one on the hub above and none here. A screen
 * that read only `effective_archived` would offer to unarchive a collection and leave it read-only
 * afterwards.
 */
export function archivalOf(container: Container): Archival {
  if (container.archived_at) return 'archived';
  return container.effective_archived ? 'inherited' : 'active';
}

/**
 * The sibling to place a container *before* in order to land at `position`, or `null` to append.
 *
 * The API takes "the one to go before" rather than an index, because a fractional index has no
 * index to give it. Converting is this function's whole job, and the awkward case is the one that
 * is easy to get wrong: moving a container **down** past its own position, where the sibling to
 * land before is one further on than the target index suggests.
 */
export function siblingBefore(
  siblings: readonly Container[],
  movingId: string,
  position: number,
): string | null {
  const without = siblings.filter((container) => container.id !== movingId);
  const target = without[position];
  return target?.id ?? null;
}
