// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A collection's sprints' paths and writes (UC-PLN-06, UC-PLN-07), so every write invalidates the
 * same set: the list, the sprint read, the backlog and the velocity. The reads are subscriptions at
 * the call site, as the plan's are.
 */

import type { EntryEstimate, Sprint, SprintClosure } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';
import type { EstimateInput } from './sprints.ts';

export * from './sprints.ts';

export const sprintsPath = (collectionId: string) => `/collections/${collectionId}/sprints`;
export const backlogPath = (collectionId: string) => `/collections/${collectionId}/sprint-backlog`;
export const velocityPath = (collectionId: string) => `/collections/${collectionId}/velocity`;
/** A sprint read in the reader's zone: a day of the burndown is a day on their clock. */
export const sprintPath = (sprintId: string, zone: string) => `/sprints/${sprintId}?zone=${encodeURIComponent(zone)}`;

class Sprints {
  private touched(collectionId: string, sprintId: string | undefined, zone: string): string[] {
    const paths = [sprintsPath(collectionId), backlogPath(collectionId), velocityPath(collectionId)];
    return sprintId ? [...paths, sprintPath(sprintId, zone)] : paths;
  }

  create(collectionId: string, zone: string, fields: { name: string; goal: string; start_on: string; end_on: string; unit: 'POINTS' | 'HOURS' }): Promise<Sprint> {
    return engine.mutate<Sprint>('POST', sprintsPath(collectionId), fields, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: this.touched(collectionId, undefined, zone),
    });
  }

  update(collectionId: string, zone: string, sprint: Sprint, fields: { name?: string; goal?: string; start_on?: string; end_on?: string }): Promise<Sprint> {
    return engine.mutate<Sprint>('PATCH', `/sprints/${sprint.id}`, fields, {
      ifMatch: etagFor(sprint.version),
      invalidates: this.touched(collectionId, sprint.id, zone),
    });
  }

  async remove(collectionId: string, zone: string, sprintId: string): Promise<void> {
    await engine.mutate<void>('DELETE', `/sprints/${sprintId}`, undefined, { invalidates: this.touched(collectionId, sprintId, zone) });
  }

  start(collectionId: string, zone: string, sprintId: string): Promise<Sprint> {
    return engine.mutate<Sprint>('POST', `/sprints/${sprintId}:start`, undefined, { invalidates: this.touched(collectionId, sprintId, zone) });
  }

  /** Closes a running sprint; the unfinished entries go into `carryTo`, or back to the backlog. */
  close(collectionId: string, zone: string, sprintId: string, carryTo: string | undefined): Promise<SprintClosure> {
    const touched = this.touched(collectionId, sprintId, zone);
    return engine.mutate<SprintClosure>('POST', `/sprints/${sprintId}:close`, carryTo ? { carry_to_sprint_id: carryTo } : {}, {
      invalidates: carryTo ? [...touched, sprintPath(carryTo, zone)] : touched,
    });
  }

  add(collectionId: string, zone: string, sprintId: string, itemIds: readonly string[]): Promise<Sprint> {
    return engine.mutate<Sprint>('POST', `/sprints/${sprintId}/entries`, { item_ids: [...itemIds] }, { invalidates: this.touched(collectionId, sprintId, zone) });
  }

  take(collectionId: string, zone: string, sprintId: string, itemIds: readonly string[]): Promise<Sprint> {
    return engine.mutate<Sprint>('POST', `/sprints/${sprintId}/entries:remove`, { item_ids: [...itemIds] }, { invalidates: this.touched(collectionId, sprintId, zone) });
  }

  /** An entry's estimate set, or cleared with `null`. */
  async estimate(collectionId: string, zone: string, sprintId: string | undefined, itemId: string, input: EstimateInput | null): Promise<EntryEstimate | undefined> {
    const invalidates = this.touched(collectionId, sprintId, zone);
    if (input === null) {
      await engine.mutate<void>('DELETE', `/items/${itemId}/estimate`, undefined, { invalidates });
      return undefined;
    }
    return engine.mutate<EntryEstimate>('PUT', `/items/${itemId}/estimate`, input, { invalidates });
  }
}

export const sprints = new Sprints();
