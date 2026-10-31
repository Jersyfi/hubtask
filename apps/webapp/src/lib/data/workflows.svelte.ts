// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's workflows and their scheme, read once where the workflows module is on
 * (UC-FLW-02 … 05), and the steps of one entry.
 */

import type {
  EntrySteps, ResourceState, StepVerdict, WorkItem, Workflow, WorkflowList, WorkflowPreview,
  WorkflowSchemeChange, WorkflowSchemeResult, WorkflowShape,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';
import { CONFIG_CHECK, TOUCHES_ANY_ENTRY } from './touches.ts';

export * from './workflows.ts';

const PATH = '/workflows';
const SCHEME = '/workflow-scheme';
const stepsPath = (itemId: string) => `/items/${itemId}/steps`;

class Workflows {
  #state = $state<ResourceState<WorkflowList> | undefined>(undefined);

  get state(): ResourceState<WorkflowList> | undefined {
    return this.#state;
  }

  /** The list and the scheme, undefined until read. */
  get list(): WorkflowList | undefined {
    return this.#state?.status === 'ready' ? this.#state.data : undefined;
  }

  /** Starts the read. **From `untrack`**, for the reason the other stores record. */
  open(): () => void {
    return engine.subscribe<WorkflowList>({ path: PATH }, (next) => {
      this.#state = next;
    });
  }

  create(key: string, name: string, shape: WorkflowShape): Promise<Workflow> {
    return engine.mutate<Workflow>('POST', PATH, { key, name, shape }, {
      idempotencyKey: crypto.randomUUID(), invalidates: [PATH, CONFIG_CHECK],
    });
  }

  /** Saves the draft beside what is in force (UC-FLW-02 check 3). */
  saveDraft(workflow: Pick<Workflow, 'key' | 'version'>, shape: WorkflowShape, name?: string): Promise<Workflow> {
    return engine.mutate<Workflow>('PATCH', `${PATH}/${encodeURIComponent(workflow.key)}`, { shape, ...(name ? { name } : {}) }, {
      ...(workflow.version > 0 ? { ifMatch: etagFor(workflow.version) } : {}), invalidates: [PATH, CONFIG_CHECK],
    });
  }

  discardDraft(workflow: Pick<Workflow, 'key' | 'version'>): Promise<Workflow> {
    return engine.mutate<Workflow>('PATCH', `${PATH}/${encodeURIComponent(workflow.key)}`, { discard: true }, {
      ...(workflow.version > 0 ? { ifMatch: etagFor(workflow.version) } : {}), invalidates: [PATH, CONFIG_CHECK],
    });
  }

  /** What publishing would do, changing nothing (UC-FLW-05 check 2). */
  previewPublish(key: string, mapping: Record<string, string>): Promise<WorkflowPreview> {
    return engine.mutate<WorkflowPreview>('POST', `${PATH}/${encodeURIComponent(key)}:preview-publish`, { mapping }, { invalidates: [] });
  }

  publish(workflow: Pick<Workflow, 'key' | 'version'>, mapping: Record<string, string>): Promise<Workflow> {
    return engine.mutate<Workflow>('POST', `${PATH}/${encodeURIComponent(workflow.key)}:publish`, { mapping }, {
      ...(workflow.version > 0 ? { ifMatch: etagFor(workflow.version) } : {}),
      invalidates: [PATH, CONFIG_CHECK, ...TOUCHES_ANY_ENTRY],
    });
  }

  async remove(key: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${PATH}/${encodeURIComponent(key)}`, undefined, { invalidates: [PATH, CONFIG_CHECK] });
  }

  /** What using a workflow in a hub or collection would do (UC-FLW-02 check 4). */
  previewScheme(change: WorkflowSchemeChange): Promise<WorkflowPreview> {
    return engine.mutate<WorkflowPreview>('POST', `${SCHEME}:preview`, change, { invalidates: [] });
  }

  setScheme(change: WorkflowSchemeChange): Promise<WorkflowSchemeResult> {
    return engine.mutate<WorkflowSchemeResult>('PUT', SCHEME, { ...change, confirm: true }, {
      invalidates: [PATH, CONFIG_CHECK, ...TOUCHES_ANY_ENTRY],
    });
  }

  /** Tries a step against an entry without taking it (UC-FLW-04 check 5). */
  tryStep(key: string, step: string, itemId: string, draft: boolean): Promise<StepVerdict> {
    return engine.mutate<StepVerdict>('POST', `${PATH}/${encodeURIComponent(key)}/steps/${encodeURIComponent(step)}:try`, {
      item_id: itemId, draft,
    }, { invalidates: [] });
  }

  /** Reads an entry's status and the steps the reader may take; follows every change of it. */
  steps(itemId: string, into: (state: ResourceState<EntrySteps>) => void): () => void {
    return engine.subscribe<EntrySteps>({ path: stepsPath(itemId) }, into);
  }

  /**
   * Takes a step - by key, or the first one to a status, which is what a drop on a board knows
   * (UC-FLW-03 check 2). The server refuses with what the step still needs.
   */
  take(item: Pick<WorkItem, 'id' | 'version'>, by: { step?: string; to_status?: string }): Promise<WorkItem> {
    return engine.mutate<WorkItem>('POST', `/items/${item.id}:step`, by, {
      idempotencyKey: crypto.randomUUID(), ifMatch: etagFor(item.version),
      invalidates: [...TOUCHES_ANY_ENTRY, stepsPath(item.id)],
    });
  }
}

export const workflows = new Workflows();
