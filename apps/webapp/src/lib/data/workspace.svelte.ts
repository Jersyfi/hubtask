// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace as the people inside it see it, and the four things they may change (F4-01).
 *
 * **`/tenant`, not `/admin/tenants`.** That listing crosses workspaces and belongs to the
 * installation operator; this answers one workspace to the people inside it and answers no other.
 *
 * **Merge-patch, so only what moved is sent.** An absent key changes nothing, and an explicit
 * `null` reads as absent rather than as "clear it" — which loses nothing, because none of the four
 * fields has an absent state: a workspace always has a name, a locale, a zone and an answer to the
 * enforcement question.
 *
 * **The version travels.** A concurrent write is refused rather than silently overwritten
 * (ADR-0025), which is what the `If-Match` here is for.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/tenant';

/** The workspace, as `Workspace` answers it. */
export interface Workspace {
  readonly id: string;
  readonly slug: string;
  readonly display_name: string;
  readonly status: string;
  readonly default_locale: string;
  readonly default_time_zone: string;
  readonly require_admin_totp: boolean;
  readonly created_at: string;
  readonly updated_at?: string | null;
  readonly version: number;
}

/** What may move. Every key optional, which is what merge-patch means. */
export interface WorkspaceChange {
  display_name?: string;
  default_locale?: string;
  default_time_zone?: string;
  require_admin_totp?: boolean;
}

class WorkspaceStore {
  #state = $state<ResourceState<Workspace>>({ status: 'idle' });

  get state(): ResourceState<Workspace> {
    return this.#state;
  }

  get workspace(): Workspace | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<Workspace>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Writes what moved.
   *
   * The `ETag` the read carried is presented as `If-Match`: two administrators on the same screen
   * is the ordinary case, and the second one's save has to be refused rather than quietly winning.
   */
  async change(body: WorkspaceChange): Promise<Workspace> {
    const etag = this.#state.status === 'ready' ? this.#state.etag : undefined;
    return engine.mutate<Workspace>('PATCH', PATH, body, {
      ifMatch: etag,
      invalidates: [PATH],
    });
  }
}

export const workspace = new WorkspaceStore();
export { PATH as workspacePath };
