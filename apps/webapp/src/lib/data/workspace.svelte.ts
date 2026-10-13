// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace as the people inside it see it, and the four things they may change.
 *
 * **`/tenant`, not `/admin/tenants`.** That listing crosses workspaces and belongs to the
 * installation operator; this answers one workspace to the people inside it and answers no other.
 *
 * **Merge-patch, so only what moved is sent.** An absent key changes nothing, and an explicit
 * `null` reads as absent rather than as "clear it". The one field with an absent state, the week
 * start, is cleared with the empty string; a workspace always has a name, a locale and a zone.
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
  /** The backup target the audit chain's end is anchored to once a day; null or absent is off. */
  readonly audit_anchor_target_id?: string | null;
  readonly created_at: string;
  readonly updated_at?: string | null;
  readonly version: number;
  /**
   * Whether to offer a private hub at all: true where the workspace has more than one person.
   * A workspace of one has nobody to keep a hub from (UC-ID-16 check 8).
   */
  readonly private_hubs_offered?: boolean;
  /**
   * Whether more than one person can act here: where private hubs, and the switch that allows
   * them, mean anything at all (UC-ID-16 checks 7, 8).
   */
  readonly several_people?: boolean;
  /** The day this workspace's weeks start on for a member who chose none; absent where it decided none. */
  readonly default_week_start?: string | null;
  /** What the installation set as the default language, zone and week start - only what it decided. */
  readonly installation_defaults?: {
    readonly locale?: string;
    readonly time_zone?: string;
    readonly week_start?: string;
  };
}

/** What may move. Every key optional, which is what merge-patch means. */
export interface WorkspaceChange {
  display_name?: string;
  default_locale?: string;
  default_time_zone?: string;
  /** The empty string clears it. */
  default_week_start?: string;
}

class WorkspaceStore {
  #state = $state<ResourceState<Workspace>>({ status: 'idle' });

  get state(): ResourceState<Workspace> {
    return this.#state;
  }

  get workspace(): Workspace | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /**
   * The week start's link above the account: the workspace's own day, otherwise the installation's
   * default - the same link the server resolves for this reader (i18n-l10n.md §4).
   */
  get weekStartAbove(): string | null {
    const held = this.workspace;
    return held?.default_week_start || held?.installation_defaults?.week_start || null;
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
