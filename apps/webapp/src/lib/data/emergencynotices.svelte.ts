// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The person's own notices that a private hub of theirs was opened through the emergency access
 * (ADR-0073 §4, UC-ID-16 checks 10 and 11), from the record written when it was opened - whatever
 * the mail about it did. This is how somebody without a mailbox is told: a child's managed account,
 * or anybody whose mail did not arrive.
 *
 * Each notice says when this person confirmed it (`acknowledged_at`), so a confirmation holds on
 * every device. What was shown and closed without confirming is kept in this tab only: the notice
 * asks again the next time the app opens. A confirmation is never queued - it is the proof that
 * the person read it, and a queued one could be sent for a notice they never saw again.
 */

import type { EmergencyNotice, ResourceState } from '@hubtask/sync-engine';

import { confirmEach, currentNotices, waitingNotices } from '../frame/emergencynotice.ts';
import { session } from '../session.svelte.ts';
import { engine } from './engine.ts';

const PATH = '/accounts/me/emergency-notices';

class EmergencyNotices {
  #state = $state<ResourceState<readonly EmergencyNotice[]>>({ status: 'idle' });
  /** Shown since the app was opened, and closed without a confirmation. */
  #seen = $state<ReadonlySet<string>>(new Set());
  /** Whether the notice is on the screen now, for the tour, which waits while it is. */
  isShown = $state(false);

  /** The notices of the last thirty days, newest first; none while unread, refused or signed out. */
  get notices(): readonly EmergencyNotice[] {
    // An array or nothing: a server of the previous release answers the route as not there, and a
    // page envelope from anything in between must not reach a screen that maps over it.
    if (!session.isSignedIn || this.#state.status !== 'ready' || !Array.isArray(this.#state.data)) return [];
    return currentNotices(this.#state.data);
  }

  /** The unconfirmed ones not yet shown since the app was opened. */
  get waiting(): readonly EmergencyNotice[] {
    return waitingNotices(this.notices, this.#seen);
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly EmergencyNotice[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Records that these were shown and closed, so they wait for the next opening of the app. */
  seen(ids: readonly string[]): void {
    this.#seen = new Set([...this.#seen, ...ids]);
  }

  /**
   * Confirms each access on the server and reads the notices again; answers the first refusal,
   * or nothing when every confirmation landed. Offline it is refused like any other write that
   * cannot reach the server.
   */
  async acknowledge(ids: readonly string[]): Promise<unknown> {
    return confirmEach(ids, (id) => engine.mutate<unknown>('POST', `${PATH}/${id}:acknowledge`, undefined, { invalidates: [PATH] }));
  }
}

export const emergencyNotices = new EmergencyNotices();
