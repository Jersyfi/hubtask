// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The templates in force along a container's path, and stamping one out.
 *
 * **The list is per path, and that is the whole design.** Naming a container answers the ones
 * defined along it — the collection's, its hub's and the workspace-wide ones — "because that is
 * the set a person picking a template in a collection can choose from". One read, one list, three
 * scopes, each row saying which it is.
 *
 * **A deletion is soft and this promises nothing more.** The trees it has already stamped out are
 * ordinary entries and outlive it, and a template defined later under the same name is a new one.
 */

import type {
  Template,
  TemplateInput,
  TemplateInstance,
  TemplateInstantiation,
  TemplatePage,
  TemplateUpdate,
  ResourceState,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

/** Where a container's templates live. Naming none is a different question, so a different path. */
export const templatesPath = (containerId?: string) =>
  containerId ? `/templates?container_id=${containerId}` : '/templates';

/**
 * What a template write touches.
 *
 * `/templates` alone: defining, changing and removing a template writes no entry. An
 * *instantiation* does, and says so separately — that one makes a tree of them.
 */
const TOUCHES = ['/templates'];

class Templates {
  #paths = $state<Record<string, ResourceState<TemplatePage>>>({});

  /** One path's templates, empty until they have been read. */
  of(containerId?: string): readonly Template[] {
    const state = this.#paths[templatesPath(containerId)];
    return state?.status === 'ready' ? (state.data.data ?? []) : [];
  }

  stateOf(containerId?: string): ResourceState<TemplatePage> | undefined {
    return this.#paths[templatesPath(containerId)];
  }

  /** Starts one path's list. **From `untrack`**, for the reason every other store records. */
  open(containerId?: string): () => void {
    const path = templatesPath(containerId);
    return engine.subscribe<TemplatePage>({ path }, (next) => {
      this.#paths = { ...this.#paths, [path]: next };
    });
  }

  /** Defines one. `STRUCTURE` at the template's own scope is what this takes; the caller gates. */
  async create(body: TemplateInput, idempotencyKey: string): Promise<Template> {
    return engine.mutate<Template>('POST', '/templates', body, {
      idempotencyKey,
      invalidates: TOUCHES,
    });
  }

  /**
   * Changes one. The node tree is one member and travels whole — a shape rather than a list of
   * settings — so a caller that sends `nodes` sends all of them.
   */
  async update(id: string, body: TemplateUpdate, version: number): Promise<Template> {
    return engine.mutate<Template>('PATCH', `/templates/${id}`, body, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /** Removes it. Soft: what it already stamped out stays where it is. */
  async remove(id: string, version: number): Promise<void> {
    await engine.mutate<void>('DELETE', `/templates/${id}`, undefined, {
      ifMatch: etagFor(version),
      invalidates: TOUCHES,
    });
  }

  /**
   * Stamps it out into a collection.
   *
   * `WRITE_ITEMS` there is what this takes rather than `STRUCTURE`: using a shape is ordinary work.
   * `/items` is invalidated because a tree of entries is exactly what it made, and the idempotency
   * key is what stops a retry making a second one.
   */
  async instantiate(
    id: string,
    body: TemplateInstantiation,
    idempotencyKey: string,
  ): Promise<TemplateInstance> {
    return engine.mutate<TemplateInstance>('POST', `/templates/${id}:instantiate`, body, {
      idempotencyKey,
      invalidates: ['/items'],
    });
  }
}

export const templates = new Templates();
