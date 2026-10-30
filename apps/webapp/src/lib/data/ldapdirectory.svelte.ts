// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The LDAP directory a workspace synchronises its people and groups from (UC-ID-31).
 *
 * **One per workspace, set whole.** `PUT` binds to the directory before anything is stored, so a
 * save that comes back is a directory that answered. The password goes one way: sealed on the way
 * in, a member of no answer; omitting it keeps the sealed one.
 *
 * **A `404` is "none set up"**, not a failure: the screen then offers the form empty.
 *
 * **A run is the server's to refuse.** A directory that answers nobody, or would switch off more
 * people than the guard allows, comes back `REFUSED` with the count; giving that count back - with
 * a fresh step-up - is the one way through, and it is the person's decision, never this module's.
 */

import type { LdapDirectory, LdapDirectoryConfiguration, LdapSyncRun, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { stepUp } from './stepup.svelte.ts';

const PATH = '/directory/ldap';

class LdapDirectoryStore {
  #state = $state<ResourceState<LdapDirectory>>({ status: 'idle' });

  get state(): ResourceState<LdapDirectory> {
    return this.#state;
  }

  /** The directory once read; `undefined` before, where none is set up, and where it was refused. */
  get directory(): LdapDirectory | undefined {
    return this.#state.status === 'ready' ? this.#state.data : undefined;
  }

  /** Whether the read answered that none is set up. */
  get none(): boolean {
    return this.#state.status === 'failed' && this.#state.error.status === 404;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<LdapDirectory>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Sets the directory whole; the server binds to it first. */
  async configure(body: LdapDirectoryConfiguration): Promise<LdapDirectory> {
    return stepUp.around((stepUpToken) =>
      engine.mutate<LdapDirectory>('PUT', PATH, body, { stepUpToken, invalidates: [PATH] }),
    );
  }

  /** Stops synchronising; the people it provisioned stay. */
  async remove(): Promise<void> {
    await stepUp.around((stepUpToken) =>
      engine.mutate<unknown>('DELETE', PATH, undefined, { stepUpToken, invalidates: [PATH] }),
    );
  }

  /**
   * Runs now, or plans with `dryRun`. A `confirm` count lets a guarded run through and is the one
   * case that needs a step-up.
   */
  async sync(dryRun: boolean, confirm?: number): Promise<LdapSyncRun> {
    const body = { dry_run: dryRun, ...(confirm === undefined ? {} : { confirm_switch_off: confirm }) };
    if (confirm === undefined) {
      return engine.mutate<LdapSyncRun>('POST', `${PATH}:sync`, body, { invalidates: dryRun ? [] : [PATH] });
    }
    return stepUp.around((stepUpToken) =>
      engine.mutate<LdapSyncRun>('POST', `${PATH}:sync`, body, {
        stepUpToken,
        invalidates: dryRun ? [] : [PATH],
      }),
    );
  }
}

export const ldapDirectory = new LdapDirectoryStore();
