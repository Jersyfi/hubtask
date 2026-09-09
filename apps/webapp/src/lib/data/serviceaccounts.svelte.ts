// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The accounts that exist only to be acted through (`security.md` §5).
 *
 * **A service account is not a person, and that is the whole point of it.** No mail address, no
 * sign-in anywhere, tokens as its only credential — so an integration, a script or a rule keeps
 * running after the person who wrote it has left. It holds rights the way a person does, through
 * memberships granted to it, which is why a service account with none can do nothing at all.
 *
 * **Behind `MANAGE_MEMBERS`**, because that is the person who answers for who holds access.
 */

import type { Account, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/auth/service-accounts';

class ServiceAccounts {
  #state = $state<ResourceState<readonly Account[]>>({ status: 'idle' });

  get state(): ResourceState<readonly Account[]> {
    return this.#state;
  }

  get all(): readonly Account[] {
    return this.#state.status === 'ready' ? this.#state.data : [];
  }

  /** Starts the listing. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<readonly Account[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Creates one. A name and nothing else: there is no address, because there is nobody to write to. */
  async create(displayName: string): Promise<Account> {
    return engine.mutate<Account>(
      'POST',
      PATH,
      { display_name: displayName },
      { idempotencyKey: crypto.randomUUID(), invalidates: [PATH] },
    );
  }
}

export const serviceAccounts = new ServiceAccounts();
export { PATH as serviceAccountsPath };
