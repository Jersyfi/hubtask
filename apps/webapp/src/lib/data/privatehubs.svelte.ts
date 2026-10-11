// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's private hubs as an administrator may see them (UC-ID-16 check 3): whose they are
 * and how much they hold, never a name - the answer has none to give.
 *
 * Read wherever the administration is open, because whether its *Private hubs* row is drawn at all
 * is this answer: a workspace without a private hub has nothing to show there, and a reader the
 * server refuses (`MANAGE_MEMBERS` is what it asks) has no row either.
 */

import type { PrivateHubSummary, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/private-hubs';

class PrivateHubs {
  #state = $state<ResourceState<readonly PrivateHubSummary[]>>({ status: 'idle' });

  get state(): ResourceState<readonly PrivateHubSummary[]> {
    return this.#state;
  }

  /** The hubs, or none while they are unread or refused. */
  get hubs(): readonly PrivateHubSummary[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  /** Whether the administration draws the row: read, and at least one there. */
  get hasAny(): boolean {
    return this.hubs.length > 0;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly PrivateHubSummary[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }
}

export const privateHubs = new PrivateHubs();
