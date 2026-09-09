// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The sessions somebody's own account holds (F4-03, H-01).
 *
 * **One's own, and never anybody else's** — that is the route's own decision, not this module's:
 * "a session is the person's, and an administrator who suspects one acts by disabling the account,
 * not by reading its sessions". So there is no account parameter here, and nothing to pass one to.
 *
 * **What a row carries is a hint, not an address.** The client that signed in as it introduced
 * itself, and the IP *class* recorded at sign-in — enough to recognise one's own devices, and
 * deliberately not a precise address (T-01, the data catalogue). This module renders neither; it
 * hands both to the screen, which says what they are.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export const SESSIONS_PATH = '/auth/sessions';

/**
 * One session, as the contract answers it.
 *
 * Hand-written rather than taken from `@hubtask/api-client`, for the reason every other store in
 * this directory that names an array response does: the operation answers a bare array and the
 * generator makes no named type for one. The fields are the contract's, and the contract test is
 * what keeps them in step.
 */
export interface HeldSession {
  readonly id: string;
  readonly created_at: string;
  readonly last_used_at?: string | null;
  readonly user_agent?: string | null;
  readonly ip_class?: string | null;
  /** Whether this is the session answering the very call that read the list. */
  readonly current: boolean;
}

class Sessions {
  #state = $state<ResourceState<readonly HeldSession[]> | undefined>(undefined);

  /** The rows, newest first as the server orders them, and empty until they have been read. */
  get all(): readonly HeldSession[] {
    return this.#state?.status === 'ready' ? this.#state.data : [];
  }

  get state(): ResourceState<readonly HeldSession[]> | undefined {
    return this.#state;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly HeldSession[]>({ path: SESSIONS_PATH }, (next) => {
      this.#state = next;
    });
  }

  /**
   * Ends one session, and re-reads the list.
   *
   * `invalidates` names the path rather than removing the row locally: what the server did is what
   * the screen should show, and a row taken out of a list optimistically is a row that comes back
   * on the next read if the delete did not land.
   */
  async end(id: string): Promise<void> {
    await engine.mutate('DELETE', `${SESSIONS_PATH}/${id}`, undefined, {
      invalidates: [SESSIONS_PATH],
    });
  }

  /**
   * Ends every session of the account, this one included.
   *
   * Nothing is invalidated afterwards and nothing needs to be: the caller's own credential is
   * among the ones that just died, so what follows is a sign-out rather than a re-read.
   */
  async endAll(): Promise<void> {
    await engine.mutate('DELETE', SESSIONS_PATH, undefined);
  }
}

export const sessions = new Sessions();
