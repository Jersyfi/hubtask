// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's releases' paths and writes (UC-PLN-08), so every write invalidates the same set: the
 * list and the release read. The reads are subscriptions at the call site, as the sprints' are.
 */

import type { Release } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export * from './releases.ts';

export const releasesPath = (hubId: string) => `/hubs/${hubId}/releases`;
export const releasePath = (releaseId: string) => `/releases/${releaseId}`;

class Releases {
  private touched(hubId: string, ...releaseIds: (string | undefined)[]): string[] {
    return [releasesPath(hubId), ...releaseIds.filter((id): id is string => !!id).map(releasePath)];
  }

  create(hubId: string, fields: { name: string; description: string; release_on: string }): Promise<Release> {
    return engine.mutate<Release>('POST', releasesPath(hubId), fields, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: this.touched(hubId),
    });
  }

  update(hubId: string, release: Release, fields: { name?: string; description?: string; release_on?: string }): Promise<Release> {
    return engine.mutate<Release>('PATCH', releasePath(release.id), fields, {
      ifMatch: etagFor(release.version),
      invalidates: this.touched(hubId, release.id),
    });
  }

  async remove(hubId: string, releaseId: string): Promise<void> {
    await engine.mutate<void>('DELETE', releasePath(releaseId), undefined, { invalidates: this.touched(hubId, releaseId) });
  }

  /** Releases it: open work into `carryTo` or out of it, and the notes written when asked. */
  publish(hubId: string, releaseId: string, writeNotes: boolean, carryTo: string | undefined): Promise<Release> {
    const body: Record<string, unknown> = { write_notes: writeNotes };
    if (carryTo) body.carry_to = carryTo;
    return engine.mutate<Release>('POST', `${releasePath(releaseId)}:publish`, body, { invalidates: this.touched(hubId, releaseId, carryTo) });
  }

  add(hubId: string, releaseId: string, itemIds: readonly string[]): Promise<Release> {
    return engine.mutate<Release>('POST', `${releasePath(releaseId)}/entries`, { item_ids: [...itemIds] }, { invalidates: this.touched(hubId, releaseId) });
  }

  take(hubId: string, releaseId: string, itemIds: readonly string[]): Promise<Release> {
    return engine.mutate<Release>('POST', `${releasePath(releaseId)}/entries:remove`, { item_ids: [...itemIds] }, { invalidates: this.touched(hubId, releaseId) });
  }
}

export const releases = new Releases();
