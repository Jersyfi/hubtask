// SPDX-License-Identifier: BUSL-1.1
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

import type { Bucket, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const bucketsPath = (collectionId: string) => `/containers/${collectionId}/buckets`;

/** A bucket write changes the board's columns and where the entries sit in them. */
const TOUCHES = ['/containers', '/items'];

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
  async remove(collectionId: string, bucketId: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `${bucketsPath(collectionId)}/${bucketId}`, undefined, {
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

export const buckets = new Buckets();
