// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The devices somebody's own account synchronises from (F6-07, offline-sync.md §6, N-03).
 *
 * **One's own, and never anybody else's** — the route's decision: a device is the person's, like a
 * session, and there is no account parameter here to pass. A device registers by turning up (its
 * first pull or push), and this list is what the server keeps of it: the platform, the name it
 * introduced itself by, when it was last seen, and whether it was forgotten.
 *
 * **Forgetting is blocking, not erasing.** The row stays, marked `blocked`, until the retention
 * sweep removes it, so a person can see what they ended; the device's next push is refused with
 * `sync.device_revoked` and it starts over with a new identity - the engine's half (F6-05).
 *
 * *This device* is the one whose identifier the engine minted into this browser's store.
 */

import type { ResourceState, SyncDevice } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export const DEVICES_PATH = '/sync/devices';

class Devices {
  #state = $state<ResourceState<readonly SyncDevice[]> | undefined>(undefined);

  /** The rows, most recently seen first as the server orders them; empty until read. */
  get all(): readonly SyncDevice[] {
    return this.#state?.status === 'ready' ? this.#state.data : [];
  }

  get state(): ResourceState<readonly SyncDevice[]> | undefined {
    return this.#state;
  }

  /** Whether this row is the device this browser synchronises as. */
  isThisDevice(id: string): boolean {
    return engine.device?.id === id;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly SyncDevice[]>({ path: DEVICES_PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Forgets one device, and re-reads the list: what the server did is what the screen shows,
   * and the row comes back marked rather than gone.
   */
  async forget(id: string): Promise<void> {
    await engine.mutate('DELETE', `${DEVICES_PATH}/${id}`, undefined, {
      invalidates: [DEVICES_PATH],
    });
  }
}

export const devices = new Devices();
