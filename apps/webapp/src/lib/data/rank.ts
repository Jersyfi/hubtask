// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one question this API asks about a position, for every level that has one.
 *
 * Rank is a **fractional index** everywhere in this system, and the operations say why:
 * "an insertion between two neighbours renumbers nothing else and two offline devices can insert
 * into the same list without either one's order being discarded". A fractional index has no index
 * to hand over, so `:reorder` and `:move` take *the sibling to go before* — and converting a
 * position into that sibling is this module's whole job.
 *
 * It lives here rather than in `containers.ts`, where it started, because entries rank the same way
 * containers do: `before_container_id`, `before_item_id` and `before_bucket_id` are one question in
 * three spellings, and two implementations of it would eventually disagree about the case below.
 *
 * **The client never re-sends the level.** It names one neighbour and the server writes one key;
 * a client that sent the whole list back would rewrite every neighbouring key and throw away the
 * property the fractional index exists for.
 */

/** Anything the API ranks. The id is all this needs: a level is a list of them, in order. */
export interface Ranked {
  readonly id: string;
}

/**
 * The sibling to place an entry *before* in order to land at `position`, or `null` to append.
 *
 * The awkward case is the one that is easy to get wrong, and it is the reason this is a function
 * with tests rather than an index arithmetic written at four call sites: moving an entry **down**
 * past its own position lands before the sibling one further on than the target index suggests,
 * because the entry is taken out of the list before it is put back.
 */
export function anchorFor(
  siblings: readonly Ranked[],
  movingId: string,
  position: number,
): string | null {
  const without = siblings.filter((sibling) => sibling.id !== movingId);
  return without[position]?.id ?? null;
}
