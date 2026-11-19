// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A collection's buckets — the columns of its board.
 *
 * A **bare array**, like the labels: the contract declares one, and for the same reason. A
 * collection's columns are bounded by what fits on a board a person reads, so there is nothing to
 * page through.
 *
 * `wipLimit` and `isDoneBucket` are read and shown, never enforced. The server accepts a card that
 * takes a column past its limit — the limit is a property of the bucket (`domain-model.md` §3.5),
 * not a constraint on the write — and moving an entry into the done bucket is what completes it,
 * which the server does. A client that blocked the first or performed the second would be
 * inventing rules the workspace does not have.
 */

import type { BoardSwitch, BoardWorkflow, Bucket, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { ENTRY_LISTS } from './touches.ts';

const bucketsPath = (collectionId: string) => `/containers/${collectionId}/buckets`;

/** A bucket write changes the board's columns and where the entries sit in them. */
const TOUCHES = ['/containers', ENTRY_LISTS];

class Buckets {
  #levels = $state<Record<string, ResourceState<readonly Bucket[]>>>({});

  of(collectionId: string): readonly Bucket[] {
    const state = this.#levels[collectionId];
    return state?.status === 'ready' ? state.data : [];
  }

  stateOf(collectionId: string): ResourceState<readonly Bucket[]> | undefined {
    return this.#levels[collectionId];
  }

  /** **From `untrack`**, for the reason the other stores record. */
  open(collectionId: string): () => void {
    return engine.subscribe<readonly Bucket[]>({ path: bucketsPath(collectionId) }, (next) => {
      this.#levels = { ...this.#levels, [collectionId]: next };
    });
  }

  async create(
    collectionId: string,
    body: { name: string; wip_limit?: number | null; is_done_bucket?: boolean },
    idempotencyKey: string,
  ): Promise<Bucket> {
    return engine.mutate<Bucket>('POST', bucketsPath(collectionId), body, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
  }

  async update(
    collectionId: string,
    bucketId: string,
    body: { name?: string; wip_limit?: number | null; is_done_bucket?: boolean },
    version: number,
  ): Promise<Bucket> {
    return engine.mutate<Bucket>('PATCH', `${bucketsPath(collectionId)}/${bucketId}`, body, {
      ifMatch: `"${version}"`,
      invalidates: TOUCHES,
    });
  }

  /**
   * Deletes a column.
   *
   * What becomes of its entries is the server's answer — `FirstBucket` in the queries exists so
   * that a deleted column's items fall into the leftmost live one — so `/items` is invalidated as
   * well and the board re-reads rather than this guessing where the cards went.
   */
  async remove(collectionId: string, bucketId: string, version: number, newDoneBucketId?: string): Promise<void> {
    // Deleting the done column names the column that means done from then on (build-plan §5): the
    // server refuses it without one, `buckets.done_successor_required`, and suggests one.
    const query = newDoneBucketId ? `?new_done_bucket_id=${encodeURIComponent(newDoneBucketId)}` : '';
    await engine.mutate<void>('DELETE', `${bucketsPath(collectionId)}/${bucketId}${query}`, undefined, {
      ifMatch: `"${version}"`,
      invalidates: TOUCHES,
    });
  }

  /** Ranks a column. The fractional index again, and the same `before` shape. */
  async reorder(
    collectionId: string,
    bucketId: string,
    beforeBucketId: string | null,
    idempotencyKey: string,
  ): Promise<Bucket> {
    return engine.mutate<Bucket>(
      'POST',
      `${bucketsPath(collectionId)}/${bucketId}:reorder`,
      { before_bucket_id: beforeBucketId },
      { idempotencyKey, invalidates: TOUCHES },
    );
  }
}

/**
 * The level switches of a collection's tasks (build-plan §5): a list shown as a board and back
 * (`board:switch`), and a board's columns made a workflow (`board:workflow`). Each is asked first
 * with `dry_run`, which counts and writes nothing - that is the preview the person confirms.
 */
export const boardLevels = {
  async switchTo(collectionId: string, to: 'NONE' | 'COLUMNS', dryRun: boolean): Promise<BoardSwitch> {
    return engine.mutate<BoardSwitch>('POST', `/containers/${collectionId}/board:switch`, { to, dry_run: dryRun }, {
      ...(dryRun ? {} : { idempotencyKey: crypto.randomUUID() }),
      invalidates: TOUCHES,
    });
  },
  async makeWorkflow(collectionId: string, dryRun: boolean): Promise<BoardWorkflow> {
    return engine.mutate<BoardWorkflow>('POST', `/containers/${collectionId}/board:workflow`, { dry_run: dryRun }, {
      ...(dryRun ? {} : { idempotencyKey: crypto.randomUUID() }),
      invalidates: [...TOUCHES, '/workflows', '/items'],
    });
  },
};

export const buckets = new Buckets();
