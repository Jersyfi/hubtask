// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The signed-in person's stars, read once and shared by the sidebar and every ☆ (§6.2). A write
 * sends the whole list with the version it read (`If-Match`), because two devices reorder.
 */

import type { ResourceState, Star, StarList } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { toggled } from './stars.ts';

export const starsPath = (accountId: string) => `/accounts/${accountId}/stars`;

class Stars {
  #state = $state<ResourceState<StarList> | undefined>(undefined);
  #accountId: string | undefined;

  get all(): readonly Star[] {
    return this.#state?.status === 'ready' ? (this.#state.data.stars ?? []) : [];
  }

  /** Reads the account's list for as long as the caller's effect lives. */
  open(accountId: string): () => void {
    this.#accountId = accountId;
    return engine.subscribe<StarList>({ path: starsPath(accountId) }, (next) => {
      this.#state = next;
    });
  }

  /** Stars the place, or takes its star away; nothing past the bound. */
  async toggle(kind: Star['kind'], id: string): Promise<void> {
    const accountId = this.#accountId;
    const state = this.#state;
    if (!accountId || state?.status !== 'ready') return;
    const next = toggled(state.data.stars, kind, id);
    if (!next) return;
    await engine.mutate<StarList>('PUT', starsPath(accountId), { stars: next }, {
      ifMatch: `"${state.data.version ?? 0}"`,
      invalidates: [starsPath(accountId)],
    });
  }

  /** Writes the list in a new order (UC-WRK-37 check 6): personal, without asking. */
  async reorder(next: readonly Star[]): Promise<void> {
    const accountId = this.#accountId;
    const state = this.#state;
    if (!accountId || state?.status !== 'ready') return;
    await engine.mutate<StarList>('PUT', starsPath(accountId), { stars: [...next] }, {
      ifMatch: `"${state.data.version ?? 0}"`,
      invalidates: [starsPath(accountId)],
    });
  }
}

export const stars = new Stars();
