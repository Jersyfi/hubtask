// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel
/**
 * The change of the workspace's address a member without an email address is told of at the next
 * sign-in (multi-tenancy.md §3.1, UC-INS-19 check 8): the old and the new address, read while signed
 * in, shown by the frame until the member acknowledges it on the server - so it does not come back
 * on another device. A member who was mailed has none.
 */
import type { AddressChangeNotice, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const PATH = '/accounts/me/address-notices';

class AddressNotices {
  #state = $state<ResourceState<AddressChangeNotice[]>>({ status: 'idle' });

  /** The notice to show, or none. */
  get notice(): AddressChangeNotice | undefined {
    return this.#state.status === 'ready' ? this.#state.data[0] : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason every other store records. */
  open(): () => void {
    return engine.subscribe<AddressChangeNotice[]>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  /** Records that the member read it. */
  async acknowledge(): Promise<void> {
    await engine.mutate<unknown>('POST', `${PATH}:acknowledge`, undefined, { invalidates: [PATH] });
  }
}

export const addressNotices = new AddressNotices();
