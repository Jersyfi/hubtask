// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The definitions in force, and writing one value on one entry.
 *
 * **A bare array, and scoped by a query parameter.** `GET /custom-fields?collection_id=` answers
 * the collection's own definitions *and* the workspace-wide ones above it, in one list. There is
 * no page: the number of fields a workspace adds is bounded by what people will fill in.
 *
 * **Defining, changing and deleting are `STRUCTURE`.** The gate is the caller's; this module makes
 * the calls and lets the server refuse what the gate mispredicted.
 *
 * **A deletion is soft and this module promises nothing more.** The values stay in the entries and
 * stop being visible — "rewriting `custom_fields` across every entry in a collection would be an
 * unbounded write from one request" — and a definition recreated under the same key is a new one
 * that shows none of them. The dialog says so; nothing here pretends otherwise.
 */

import type {
  CustomFieldDefinition,
  CustomFieldDefinitionCreate,
  CustomFieldDefinitionUpdate,
  ResourceState,
  WorkItem,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

/**
 * Where a scope's definitions live.
 *
 * The workspace-wide ones have their own path — naming no collection is a different question, not
 * the same one with a missing argument — and the engine keys a resource on the path, so the two
 * are two subscriptions rather than one that changes its mind.
 */
export const definitionsPath = (collectionId?: string) =>
  collectionId ? `/custom-fields?collection_id=${collectionId}` : '/custom-fields';

/**
 * What a definition write invalidates.
 *
 * `/custom-fields` covers both scopes, because the engine matches by prefix and the collection's
 * path begins with it. `/items` is there because a definition decides what an entry *shows*: a
 * field taken out of use has to stop being drawn on the entries that hold a value for it.
 */
const TOUCHES = ['/custom-fields', '/items'];

class CustomFields {
  #scopes = $state<Record<string, ResourceState<readonly CustomFieldDefinition[]>>>({});

  /** One scope's definitions, empty until they have been read. */
  of(collectionId?: string): readonly CustomFieldDefinition[] {
    const state = this.#scopes[definitionsPath(collectionId)];
    return state?.status === 'ready' ? state.data : [];
  }

  stateOf(collectionId?: string): ResourceState<readonly CustomFieldDefinition[]> | undefined {
    return this.#scopes[definitionsPath(collectionId)];
  }

  /** Starts one scope's list. **From `untrack`**, for the reason the other stores record. */
  open(collectionId?: string): () => void {
    const path = definitionsPath(collectionId);
    return engine.subscribe<readonly CustomFieldDefinition[]>({ path }, (next) => {
      this.#scopes = { ...this.#scopes, [path]: next };
    });
  }

  /** Defines a field. The key and the kind are settled here and never again. */
  async define(
    body: CustomFieldDefinitionCreate,
    idempotencyKey: string,
  ): Promise<CustomFieldDefinition> {
    return engine.mutate<CustomFieldDefinition>('POST', '/custom-fields', body, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
  }

  /**
   * Changes what a definition permits: its options, whether it is required, which types carry it.
   *
   * Narrowing the options does not rewrite the entries holding a value that is no longer offered —
   * they keep it until somebody sets the field again — so nothing here tries to reconcile them.
   */
  async update(
    fieldId: string,
    body: CustomFieldDefinitionUpdate,
    version: number,
  ): Promise<CustomFieldDefinition> {
    return engine.mutate<CustomFieldDefinition>('PATCH', `/custom-fields/${fieldId}`, body, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /** Takes it out of use. Soft: the values stay where they are. */
  async remove(fieldId: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `/custom-fields/${fieldId}`, undefined, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * Writes one key on one entry, and only one.
   *
   * The merge rule is per key (`offline-sync.md` §4.2), so two devices setting two different keys
   * converge to both — and that is only true of a client that writes them separately. `null`
   * clears; the two are different requests and a body that left `value` out would be neither.
   */
  async setValue(itemId: string, key: string, value: unknown, version: number): Promise<WorkItem> {
    return engine.mutate<WorkItem>(
      'PUT',
      `/items/${itemId}/custom-fields/${encodeURIComponent(key)}`,
      { value },
      { ifMatch: etagFor(version), invalidates: ['/items'] },
    );
  }
}

export const customFields = new CustomFields();
