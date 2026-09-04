// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The entries of a collection, read **one level at a time** — the shape the containers turned out
 * to have, and the shape `/items:query` was built for.
 *
 * `ItemQuery`'s `include_descendants: false` is the level read in so many words: *"the entries
 * directly in the collection, or the direct children of the entry"*. So a collection's top level is
 * one query and each opened entry is another, and a collapsed entry costs nothing. Reading a whole
 * subtree at once would be the `LIST_EXPANDED` layout deciding what is fetched even when nobody has
 * expanded anything.
 *
 * It is `/items:query` rather than `GET /items` because the query is the one read that takes a
 * scope: an anchored question, checked once against that scope, rather than a filter a client
 * assembles. F2-03 taught the engine that a `POST` can be a read.
 */

import type {
  FilterNode,
  ItemQueryResult,
  MoveResult,
  ResourceState,
  WorkItem,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

const QUERY = '/items:query';

/** What a write to an entry makes stale. Entries, and nothing about the container tree. */
const TOUCHES = ['/items'];

/**
 * What the reader has asked of a level beyond "show it": a filter, an order, a grouping.
 *
 * All three are the **manifest's** — built by `query.ts` from `query_fields` and never from a list
 * written here — and all three are optional, because the ordinary case is a collection shown the
 * way it was arranged. An absent one is left out of the document rather than sent as a null.
 */
export interface ItemsQuery {
  readonly filter?: FilterNode;
  readonly sort?: readonly { readonly field: string; readonly dir: 'ASC' | 'DESC' }[];
  readonly group?: { readonly field: string; readonly limit_per_group: number };
}

/** The manual order: `order_key ASC`, which is also the query's own default. */
const MANUAL = [{ field: 'order_key', dir: 'ASC' as const }];

/** The question that reads one level: a collection's own entries, or one entry's children. */
function level(
  scope: { container_id?: string; item_id?: string },
  query: ItemsQuery = {},
): {
  path: string;
  body: unknown;
} {
  return {
    path: QUERY,
    body: {
      scope: { ...scope, include_descendants: false },
      // The manual order unless the reader asked for another. Named rather than left out, because
      // a list a person can drag has to be in the order they dragged it into.
      sort: query.sort ?? MANUAL,
      ...(query.filter ? { filter: query.filter } : {}),
      expand: ['labels'],
      page: { size: 200 },
    },
  };
}

/**
 * The board: the same entries, grouped by bucket.
 *
 * `group_by` is what turns one read into columns — "one group per distinct value, each with its own
 * rows and its own cursor", which is the specification's own description and the reason a board is
 * not five queries. The entries with no bucket come back as the group whose key is null, and the
 * API puts it last.
 *
 * `count: 'exact'` because a column's count is what a WIP limit is read against, and a limit with
 * no count is a number nobody can act on. It costs a second pass — the contract says so out loud —
 * and this is the one read so far where it is worth it.
 */
function board(containerId: string, query: ItemsQuery): { path: string; body: unknown } {
  return {
    path: QUERY,
    body: {
      scope: { container_id: containerId, include_descendants: false },
      // The grouping is the caller's, and the caller reads it from the manifest. A board grouped
      // by the column an entry is in is what a board *is*, but which fields may be grouped on at
      // all is `groupable` in `query_fields` — so the field travels rather than being written here.
      ...(query.group ? { group_by: query.group } : {}),
      sort: query.sort ?? MANUAL,
      ...(query.filter ? { filter: query.filter } : {}),
      expand: ['labels'],
      count: 'exact',
    },
  };
}

const rowsOf = (state: ResourceState<ItemQueryResult> | undefined): readonly WorkItem[] =>
  state?.status === 'ready' ? (state.data.data ?? []) : [];

class Items {
  /** One entry per level that has been opened, keyed by the scope it was read for. */
  #levels = $state<Record<string, ResourceState<ItemQueryResult>>>({});

  /** The entries directly in a collection. */
  inCollection(containerId: string): readonly WorkItem[] {
    return rowsOf(this.#levels[`container:${containerId}`]);
  }

  /** The direct children of an entry. */
  childrenOf(itemId: string): readonly WorkItem[] {
    return rowsOf(this.#levels[`item:${itemId}`]);
  }

  stateOf(key: string): ResourceState<ItemQueryResult> | undefined {
    return this.#levels[key];
  }

  /**
   * Starts one level. **Call this from `untrack`** — the listener writes the store and writing it
   * reads it, so an effect that subscribes while tracking that read cancels its own subscription
   * before the answer arrives (F2-08 records the symptom).
   */
  openCollection(containerId: string, query: ItemsQuery = {}): () => void {
    return this.#open(`container:${containerId}`, level({ container_id: containerId }, query));
  }

  /**
   * A child level is read **unfiltered**, and that is deliberate rather than an omission.
   *
   * A filter narrows what the reader is looking for at the level they are looking at; applying it
   * again to the children of a row that matched would hide the entries *inside* a match, which is
   * the opposite of what expanding a row asks for.
   */
  openChildren(itemId: string): () => void {
    return this.#open(`item:${itemId}`, level({ item_id: itemId }));
  }

  /** Starts the board. **From `untrack`**, like every other subscription here. */
  openBoard(containerId: string, query: ItemsQuery = {}): () => void {
    return this.#open(`board:${containerId}`, board(containerId, query));
  }

  /**
   * The board's groups, in the order the server returned them — the field's own order, with the
   * entries that have no bucket last.
   */
  boardGroups(containerId: string): readonly NonNullable<ItemQueryResult['groups']>[number][] {
    const state = this.#levels[`board:${containerId}`];
    return state?.status === 'ready' ? (state.data.groups ?? []) : [];
  }

  #open(key: string, request: { path: string; body: unknown }): () => void {
    return engine.subscribe<ItemQueryResult>(request, (next) => {
      this.#levels = { ...this.#levels, [key]: next };
    });
  }

  /**
   * Creates an entry.
   *
   * The type and the parent are the caller's, because whether that combination is permitted is the
   * manifest's answer and `lib/data/capability.ts` is what asks it — a create that decided for
   * itself would be the hard-coded matrix §2's extension example rules out.
   */
  async create(
    body: {
      type: string;
      collection_id?: string;
      parent_id?: string | null;
      title: string;
      notes?: string | null;
    },
    idempotencyKey: string,
  ): Promise<WorkItem> {
    // `due_date_only` is required by the contract and false is what "no due date" means here;
    // F3 is where dates get a surface.
    return engine.mutate<WorkItem>('POST', '/items', { due_date_only: false, ...body }, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
  }

  /** Retitles or renotes one, against the version the reader had (ADR-0025). */
  async update(
    id: string,
    body: { title?: string; notes?: string | null },
    version: number,
  ): Promise<WorkItem> {
    return engine.mutate<WorkItem>('PATCH', `/items/${id}`, body, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * Moves an entry into a bucket, or out of every one.
   *
   * A `PATCH` rather than an action, because `bucket_id` is a field of the entry. What happens next
   * may not be only a move: `isDoneBucket` can trigger completion (`domain-model.md` §3.5), and
   * that is the server's doing — so the answer is re-read rather than predicted, and the card comes
   * back completed if it was.
   */
  async setBucket(id: string, bucketId: string | null, version: number): Promise<WorkItem> {
    return engine.mutate<WorkItem>('PATCH', `/items/${id}`, { bucket_id: bucketId }, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * Ranks an entry within the level it already sits in.
   *
   * The position travels as **the sibling to go before**, because the rank is a fractional index
   * and a fractional index has no index to hand over. That is also what keeps the neighbours
   * alone: the server writes one key between two others and renumbers nothing, and this client
   * never sends the level back to be renumbered.
   *
   * `ifMatch` although the contract declares no `If-Match` on an action. The server reads the
   * header anyway and says why (`PlacementController.go`): a rank change against a version the
   * reader no longer has is a lost race, and losing it silently is what an optimistic lock exists
   * to prevent. A pointer drag is the case that makes it worth sending — a drag takes seconds, and
   * seconds are long enough for the row to have moved underneath.
   */
  async reorder(
    id: string,
    beforeItemId: string | null,
    version: number,
    idempotencyKey: string,
  ): Promise<WorkItem> {
    return engine.mutate<WorkItem>(
      'POST',
      `/items/${id}:reorder`,
      { before_item_id: beforeItemId },
      { idempotencyKey, ifMatch: etagFor(version), invalidates: TOUCHES },
    );
  }

  /**
   * Moves an entry, and its whole subtree, to another parent or another collection.
   *
   * `:move` rather than `:reorder` the moment the parent changes, and it answers a `MoveResult`
   * rather than the entry: invariant I-W6 says a reference the destination cannot resolve is
   * **reported** rather than dropped in silence, and `dropped_references` is that report. A caller
   * that ignored it would turn a designed behaviour into data loss — the labels would simply be
   * gone, indistinguishable from a rendering fault.
   *
   * `target_collection_id` is sent only when the destination names one: an entry's collection is
   * the one its parent is in, and naming both is how a client says something the server has to
   * refuse. `target_parent_id` is always sent, because `null` is the top level of a collection and
   * omitting it means "leave the parent alone" — the value cannot tell the two apart, which is the
   * same distinction `MoveWorkItem` reads a presence map for on the server side.
   */
  async move(
    id: string,
    destination: {
      parentId: string | null;
      collectionId?: string;
      beforeItemId?: string | null;
    },
    version: number,
    idempotencyKey: string,
  ): Promise<MoveResult> {
    return engine.mutate<MoveResult>(
      'POST',
      `/items/${id}:move`,
      {
        target_parent_id: destination.parentId,
        ...(destination.collectionId ? { target_collection_id: destination.collectionId } : {}),
        before_item_id: destination.beforeItemId ?? null,
      },
      { idempotencyKey, ifMatch: etagFor(version), invalidates: TOUCHES },
    );
  }

  /**
   * Archives an entry, or brings it back.
   *
   * Archived is **read-only, not hidden** (I-W4): the entry stays where it is, stays readable, and
   * every control on it is off with the reason. That is the screen's half; this is the write.
   */
  async setArchived(id: string, isArchived: boolean, idempotencyKey: string): Promise<WorkItem> {
    return engine.mutate<WorkItem>(
      'POST',
      `/items/${id}:${isArchived ? 'archive' : 'unarchive'}`,
      undefined,
      { idempotencyKey, invalidates: TOUCHES },
    );
  }

  /**
   * Moves an entry and everything under it to the trash.
   *
   * A **soft** delete, and a batch: the subtree goes in under one deletion sharing a
   * `trash_batch_id` "so that restoring is atomic" (I-C2). Nothing here says how long it stays —
   * that is the workspace's retention period, and it is not this client's to assert.
   *
   * `If-Match`, because the contract declares one on this `DELETE`: deleting a subtree that moved
   * underneath the reader is the case an optimistic lock is for.
   */
  async trash(id: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `/items/${id}`, undefined, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * Takes one deletion back out of the trash, whole.
   *
   * Exactly what went in together comes back, because a restore is keyed on the **deletion** and
   * not on the subtree — so a younger deletion inside it stays where it is. An entry archived when
   * it was deleted comes back archived: restoring undoes the deletion and nothing else.
   */
  async restore(id: string, idempotencyKey: string): Promise<WorkItem> {
    return engine.mutate<WorkItem>('POST', `/items/${id}:restore`, undefined, {
      idempotencyKey,
      // The trash changes and so does whatever level the entry came back to.
      invalidates: ['/items', '/trash'],
    });
  }

  /**
   * Destroys an entry in the trash, and everything under it, for good.
   *
   * Irreversible — "no restore brings it back, and a backup taken before it does not either" — so
   * the screen that calls this asks first and says what will go. A legal hold refuses it, and the
   * answer says at which level; that refusal is rendered as its own sentence rather than as a
   * generic failure, which is `problem.ts`'s doing and the reason the code travels.
   */
  async purge(id: string, idempotencyKey: string): Promise<void> {
    await engine.mutate<void>('POST', `/items/${id}:purge`, undefined, {
      idempotencyKey,
      invalidates: ['/items', '/trash'],
    });
  }

  /**
   * Ticks an entry off, or puts it back.
   *
   * Two operations rather than a field, because the server has two: `:complete` and `:reopen`. And
   * the answer is re-read rather than assumed — with `completionPolicy = ROLLUP` a parent completes
   * when its children do (I-W5), which is a change this client learns by asking rather than by
   * predicting.
   */
  async setCompleted(id: string, isCompleted: boolean, idempotencyKey: string): Promise<WorkItem> {
    return engine.mutate<WorkItem>(
      'POST',
      `/items/${id}:${isCompleted ? 'complete' : 'reopen'}`,
      undefined,
      { idempotencyKey, invalidates: TOUCHES },
    );
  }
}

export const items = new Items();
