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

import type { PrivateHubAccess, PrivateHubSummary, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';

export { isRunning, NOTICE_DAYS, recentAccesses } from './privatehubs.ts';

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

  /** Whether there is at least one. */
  get hasAny(): boolean {
    return this.hubs.length > 0;
  }

  /** Whether the reader may read the list at all: the server answered it rather than refused. */
  get isReadable(): boolean {
    return this.#state.status === 'ready';
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly PrivateHubSummary[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * The workspace owner's emergency access (ADR-0073 §4): opens the hub for one hour with the
   * reason every member reads. Behind the step-up, through the wrapper every privileged act uses:
   * it sends without a proof and asks for one only when the server says to. The list is read again,
   * because the row now says until when it is open.
   */
  async openHub(hubId: string, reason: string): Promise<PrivateHubAccess> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<PrivateHubAccess>('POST', accessesPath(hubId), { reason }, {
        stepUpToken,
        invalidates: [PATH, '/containers'],
      }),
    );
  }

  /** A hub's accesses, for whoever reads the hub. **From `untrack`**. */
  accessesOf(hubId: string, onNext: (state: ResourceState<readonly PrivateHubAccess[]>) => void): () => void {
    return engine.subscribe<readonly PrivateHubAccess[]>({ path: accessesPath(hubId) }, onNext);
  }
}

const accessesPath = (hubId: string) => `${PATH}/${encodeURIComponent(hubId)}/accesses`;

export const privateHubs = new PrivateHubs();
