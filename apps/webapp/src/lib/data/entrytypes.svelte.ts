// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's entry types, read once where the workflows module is on (UC-FLW-01). One list for
 * the whole workspace: a type is the workspace's, not a collection's.
 */

import type { EntryType, EntryTypeList, ResourceState } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export { iconOf, offeredTypes, rowMarkOf, systemKeyOf, typeOf, typesShown } from './entrytypes.ts';

const PATH = '/entry-types';

class EntryTypes {
  #state = $state<ResourceState<EntryTypeList> | undefined>(undefined);

  /** Every type, empty until read. */
  get all(): readonly EntryType[] {
    return this.#state?.status === 'ready' ? this.#state.data.data : [];
  }

  /** Starts the read. **From `untrack`**, for the reason the other stores record. */
  open(): () => void {
    return engine.subscribe<EntryTypeList>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }
}

export const entryTypes = new EntryTypes();
