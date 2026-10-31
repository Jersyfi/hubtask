// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's entry types, read once where the workflows module is on (UC-FLW-01). One list for
 * the whole workspace: a type is the workspace's, not a collection's.
 */

import {
  TransportError,
  type EntryForm, type EntryFormDraft, type EntryFormState, type EntryType, type EntryTypeChange, type EntryTypeCreate,
  type EntryTypeList, type ResourceState, type StoredRecord, type WorkItem,
} from '@hubtask/sync-engine';

import type { FormActor } from '@hubtask/expression';
import { engine } from './engine.ts';
import { actorOf, offlineFormState, withOwnGrants, type HeldGrant, type HeldRole } from './entryforms.ts';
import { etagFor } from './etag.ts';
import { CONFIG_CHECK } from './touches.ts';
import { roles } from './roles.svelte.ts';

export {
  TYPE_ICONS,
  TYPE_LEVELS,
  droppedBy,
  entryTypesOffered,
  iconOf,
  keyFrom,
  levelTypes,
  offeredTypes,
  reorder,
  rowMarkOf,
  systemKeyOf,
  typeOf,
  typeNameOf,
  typeOrder,
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
      invalidates: [PATH, CONFIG_CHECK],
    });
  }

  /**
   * Renames, recolours, reorders, archives or changes a type's behaviours. A system type nobody
   * changed is at version 0 and sends no precondition; its first change gives it its row.
   */
  change(type: Pick<EntryType, 'key' | 'version'>, body: EntryTypeChange): Promise<EntryType> {
    return engine.mutate<EntryType>('PATCH', `${PATH}/${encodeURIComponent(type.key)}`, body, {
      ...(type.version > 0 ? { ifMatch: etagFor(type.version) } : {}),
      invalidates: [PATH, CONFIG_CHECK],
    });
  }

  /**
   * Replaces the type's form (UC-FLW-01 check 2). A system type nobody changed sends no
   * precondition, as `change` does; its form gives it its row.
   */
  setForm(type: Pick<EntryType, 'key' | 'version'>, form: EntryForm): Promise<EntryType> {
    return engine.mutate<EntryType>('PUT', `${PATH}/${encodeURIComponent(type.key)}/form`, form, {
      ...(type.version > 0 ? { ifMatch: etagFor(type.version) } : {}),
      invalidates: [PATH, CONFIG_CHECK],
    });
  }

  /**
   * How the type's form stands for a draft - which fields are required, read-only or hidden, which
   * checks fail, whether this door may create the type. Writes nothing, so it invalidates nothing.
   */
  async evaluate(key: string, draft: EntryFormDraft): Promise<EntryFormState> {
    try {
      return await engine.mutate<EntryFormState>('POST', `${PATH}/${encodeURIComponent(key)}/form:evaluate`, draft, {
        invalidates: [],
      });
    } catch (cause) {
      // The server cannot be reached: the browser's evaluator stands in until it can.
      if (!(cause instanceof TransportError) || (cause.kind !== 'offline' && cause.kind !== 'timeout')) throw cause;
      const held = draft.item_id
        ? (await engine.storage?.get<StoredRecord<WorkItem>>('items', draft.item_id))?.document
        : undefined;
      const collectionId = draft.collection_id ?? held?.collection_id;
      const actor = collectionId
        ? await withHeldGrants(actorOf(roles.mineAt(collectionId)), collectionId)
        : actorOf(undefined);
      const state = offlineFormState(this.all.find((type) => type.key === key), draft, held, actor);
      if (!state) throw cause;
      return state;
    }
  }

  /** Deletes a type nothing uses; one in use is refused and can only be archived (check 5). */
  async remove(type: Pick<EntryType, 'key' | 'version'>): Promise<void> {
    await engine.mutate<void>('DELETE', `${PATH}/${encodeURIComponent(type.key)}`, undefined, {
      ...(type.version > 0 ? { ifMatch: etagFor(type.version) } : {}),
      invalidates: [PATH, CONFIG_CHECK],
    });
  }
}

/** The reader's own grants of custom roles along a collection's path, from the copy. */
async function withHeldGrants(actor: FormActor, collectionId: string): Promise<FormActor> {
  const storage = engine.storage;
  if (!storage) return actor;
  const collection = (await storage.get<StoredRecord<{ parent_id?: string }>>('containers', collectionId))?.document;
  const path = [collectionId, ...(collection?.parent_id ? [collection.parent_id] : [])];
  const grants = (await storage.all<StoredRecord<HeldGrant>>('role_grants')).flatMap((record) => (record.document ? [record.document] : []));
  const defined = (await storage.all<StoredRecord<HeldRole>>('custom_roles')).flatMap((record) => (record.document ? [record.document] : []));
  return withOwnGrants(actor, grants, defined, path);
}

export const entryTypes = new EntryTypes();
