// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's quota standing, and — because it is the same question — whether this reader
 * reaches the administration area at all.
 *
 * **The gate is the server's answer, not a rule this client computes.** `GET /quotas` is refused
 * unless the caller holds `STRUCTURE` or the auditor's `READ_CONFIGURATION` (`quota/Read.go`),
 * which is exactly the area's own condition. A client that worked the condition out for itself —
 * from the actor's role and the manifest's role matrix — would be a second implementation of an
 * authorisation decision, which is the thing ADR-0005 forbids by putting authorisation in one
 * layer and one layer only. It would also be wrong for an auditor: `AUDITOR` holds no `READ`, so
 * it cannot list the memberships that would say what role it has.
 *
 * So the frame subscribes to this, and the entry appears when the read succeeds. One request,
 * answering two questions, and the second answer is the first one's.
 *
 * **There is no control to raise a limit.** `/admin/tenants/{id}/quotas` is the installation
 * operator's (`multi-tenancy.md`, 0.6.0 decision 6), and a button the server would refuse is worse
 * than no button: the screen names who to ask instead.
 */

import type { ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/quotas';

/** One limit as it applies to this workspace, as `QuotaStanding` answers it. */
export interface Standing {
  readonly quota: string;
  readonly limit: number;
  readonly used?: number | null;
  readonly ratio?: number | null;
  readonly configured?: boolean;
}

class Quotas {
  #state = $state<ResourceState<readonly Standing[]>>({ status: 'idle' });

  get state(): ResourceState<readonly Standing[]> {
    return this.#state;
  }

  /** The standings, or nothing while they are unread or refused. */
  get standings(): readonly Standing[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  /**
   * Whether this reader reaches the administration area.
   *
   * `undefined` until the read has answered — the third value every gate in this client has, and
   * the one a screen gets wrong: an entry shown before the answer arrives is an entry that
   * disappears a moment later.
   */
  get isReachable(): boolean | undefined {
    if (this.#state.status === 'ready') return true;
    if (this.#state.status === 'failed') {
      // A refusal is an answer: this reader holds neither permission. Anything else — unreachable,
      // a server fault — is not, and leaves the entry out rather than claiming the area is closed.
      return this.#state.error.status === 403 ? false : undefined;
    }
    return undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly Standing[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }
}

export const quotas = new Quotas();
export { PATH as quotasPath };
