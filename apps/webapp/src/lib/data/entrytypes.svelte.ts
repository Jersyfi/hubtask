// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's entry types, read once where the workflows module is on (UC-FLW-01). One list for
 * the whole workspace: a type is the workspace's, not a collection's.
 */

import type {
  EntryForm, EntryFormDraft, EntryFormState, EntryType, EntryTypeChange, EntryTypeCreate, EntryTypeList, ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export {
  TYPE_ICONS,
  TYPE_LEVELS,
  iconOf,
  keyFrom,
  levelTypes,
  offeredTypes,
  reorder,
  rowMarkOf,
  systemKeyOf,
  typeOf,
  typesShown,
} from './entrytypes.ts';

const PATH = '/entry-types';

class EntryTypes {
  #state = $state<ResourceState<EntryTypeList> | undefined>(undefined);

  /** The read itself, for a screen that draws its waiting and its refusal. */
  get state(): ResourceState<EntryTypeList> | undefined {
    return this.#state;
  }

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

  /** Adds a type of the workspace's own (UC-FLW-01 check 1). */
  create(body: EntryTypeCreate): Promise<EntryType> {
    return engine.mutate<EntryType>('POST', PATH, body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [PATH],
    });
  }

  /**
   * Renames, recolours, reorders, archives or changes a type's behaviours. A system type nobody
   * changed is at version 0 and sends no precondition; its first change gives it its row.
   */
  change(type: Pick<EntryType, 'key' | 'version'>, body: EntryTypeChange): Promise<EntryType> {
    return engine.mutate<EntryType>('PATCH', `${PATH}/${encodeURIComponent(type.key)}`, body, {
      ...(type.version > 0 ? { ifMatch: etagFor(type.version) } : {}),
      invalidates: [PATH],
    });
  }

  /**
   * Replaces the type's form (UC-FLW-01 check 2). A system type nobody changed sends no
   * precondition, as `change` does; its form gives it its row.
   */
  setForm(type: Pick<EntryType, 'key' | 'version'>, form: EntryForm): Promise<EntryType> {
    return engine.mutate<EntryType>('PUT', `${PATH}/${encodeURIComponent(type.key)}/form`, form, {
      ...(type.version > 0 ? { ifMatch: etagFor(type.version) } : {}),
      invalidates: [PATH],
    });
  }

  /**
   * How the type's form stands for a draft - which fields are required, read-only or hidden, which
   * checks fail, whether this door may create the type. Writes nothing, so it invalidates nothing.
   */
  evaluate(key: string, draft: EntryFormDraft): Promise<EntryFormState> {
    return engine.mutate<EntryFormState>('POST', `${PATH}/${encodeURIComponent(key)}/form:evaluate`, draft, {
      invalidates: [],
    });
  }

  /** Deletes a type nothing uses; one in use is refused and can only be archived (check 5). */
  async remove(type: Pick<EntryType, 'key' | 'version'>): Promise<void> {
    await engine.mutate<void>('DELETE', `${PATH}/${encodeURIComponent(type.key)}`, undefined, {
      ...(type.version > 0 ? { ifMatch: etagFor(type.version) } : {}),
      invalidates: [PATH],
    });
  }
}

export const entryTypes = new EntryTypes();
