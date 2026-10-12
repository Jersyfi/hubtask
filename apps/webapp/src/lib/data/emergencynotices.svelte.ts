// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The person's own notices that a private hub of theirs was opened through the emergency access
 * (ADR-0073 §4), from the record written when it was opened - whatever the mail about it did.
 *
 * This is how somebody without a mailbox is told: a child's managed account, or anybody whose mail
 * did not arrive. Read once the frame is up and shown on every page for thirty days; there is
 * nothing to mark read, because the server keeps no read state and the notice is not a list of
 * notifications (UC-NOT-01).
 */

import type { PrivateHubAccess, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/accounts/me/emergency-notices';

class EmergencyNotices {
  #state = $state<ResourceState<readonly PrivateHubAccess[]>>({ status: 'idle' });

  /** The notices, newest first; none while unread or refused. */
  get notices(): readonly PrivateHubAccess[] {
    // An array or nothing: a server of the previous release answers the route as not there, and a
    // page envelope from anything in between must not reach a screen that maps over it.
    return this.#state.status === 'ready' && Array.isArray(this.#state.data) ? this.#state.data : [];
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly PrivateHubAccess[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }
}

export const emergencyNotices = new EmergencyNotices();
