// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A collection's labels, and putting them on and off entries.
 *
 * **A label belongs to a collection** (I-W3), which is why every read here is scoped to one and
 * why there is no workspace-wide list to hold. An entry in another collection has other labels,
 * and carrying an entry between them is what resolves or reports them (I-W6) — the server's answer,
 * not this module's.
 *
 * The colour is a **token**, never a hex: `domain-model.md` §3.5 says so and the backend validates
 * against the same ten names, generated into `LabelTokens.go` from the same `tokens.json` the chip
 * paints from. There is no place in this file where a colour could be written.
 */

import type { Label, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

/**
 * A **bare array**, not a `{data, page}` envelope — which is what the contract declares and worth
 * saying out loud, because every other list in this client is paged. A collection's labels are
 * bounded by what a person will make and read in one go, so there is nothing to page through; the
 * shape says so rather than a comment claiming it.
 */
const labelsPath = (collectionId: string) => `/containers/${collectionId}/labels`;

/** A label write changes entries as well as the list: a removed label leaves the rows it was on. */
const TOUCHES = ['/containers', '/items'];

class Labels {
  #levels = $state<Record<string, ResourceState<readonly Label[]>>>({});

  /** One collection's labels, empty until it has been read. */
  of(collectionId: string): readonly Label[] {
    const state = this.#levels[collectionId];
    return state?.status === 'ready' ? state.data : [];
  }

  stateOf(collectionId: string): ResourceState<readonly Label[]> | undefined {
    return this.#levels[collectionId];
  }

  /** Starts one collection's list. **From `untrack`**, for the reason the other stores record. */
  open(collectionId: string): () => void {
    return engine.subscribe<readonly Label[]>({ path: labelsPath(collectionId) }, (next) => {
      this.#levels = { ...this.#levels, [collectionId]: next };
    });
  }

  async create(
    collectionId: string,
    body: { name: string; color_token: string; description?: string | null },
    idempotencyKey: string,
  ): Promise<Label> {
    return engine.mutate<Label>('POST', `/containers/${collectionId}/labels`, body, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
  }

  async update(
    collectionId: string,
    labelId: string,
    body: { name?: string; color_token?: string; description?: string | null },
    version: number,
  ): Promise<Label> {
    return engine.mutate<Label>('PATCH', `/containers/${collectionId}/labels/${labelId}`, body, {
      ifMatch: `"${version}"`,
      invalidates: TOUCHES,
    });
  }

  /**
   * Deletes a label from the collection.
   *
   * It comes off every entry that carried it, which is the server's doing and why `/items` is
   * invalidated too — a row still showing a label that no longer exists would be the stale state
   * the seam's invalidation exists to prevent.
   */
  async remove(collectionId: string, labelId: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `/containers/${collectionId}/labels/${labelId}`, undefined, {
      ifMatch: `"${version}"`,
      invalidates: TOUCHES,
    });
  }

  /** Puts a label on an entry, or takes it off. Only entries, never the list. */
  async setOnItem(itemId: string, labelId: string, isOn: boolean, idempotencyKey: string): Promise<void> {
    await engine.mutate<void>(
      isOn ? 'PUT' : 'DELETE',
      `/items/${itemId}/labels/${labelId}`,
      undefined,
      { idempotencyKey, invalidates: ['/items'] },
    );
  }
}

export const labels = new Labels();
