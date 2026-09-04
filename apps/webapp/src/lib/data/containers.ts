// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The question about a container that is worth answering away from a component.
 *
 * It was three, then two. A `groupByHub` lived here on the assumption that `GET /containers`
 * answers hubs and collections together — it does not. `ListContainers` reads **one level**: an
 * empty `parent_id` is the hubs, a named one is that hub's collections, and the two ask different
 * permission questions, which is why no read answers both. The store fetches a level at a time and
 * there is no flat list left to group, so the function and its tests went with the assumption.
 *
 * `siblingBefore` left for a better reason: entries rank exactly the way containers do, so it is
 * `rank.ts`'s `anchorFor` now and there is one implementation of it rather than two.
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
