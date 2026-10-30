// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A plan's paths and writes (UC-PLN-01 … 05), so every write invalidates the same set: the
 * schedule, the baselines, a dependency's timing and an entry's mode. The reads are subscriptions
 * at the call site, as the whiteboards' are.
 */

import type {
  PlanTolerance,
  BaselineComparison, EntryLink, EntryScheduling, PlanBaseline, PlanMoves, WorkItem,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export * from './planning.ts';

/** The schedule, counted in the reader's zone: a day on the chart is a day on their clock. */
export const schedulePath = (planId: string, zone: string) => `/plans/${planId}/schedule?zone=${encodeURIComponent(zone)}`;
export const baselinesPath = (planId: string) => `/plans/${planId}/baselines`;
export const baselinePath = (baselineId: string) => `/baselines/${baselineId}`;
export const tolerancePath = (planId: string, zone: string) => `/plans/${planId}/tolerance?zone=${encodeURIComponent(zone)}`;

class Planning {
  /** Applies the moves accepted, all or none (check 4); the plan is scheduled again by the server. */
  apply(planId: string, zone: string, itemIds: readonly string[]): Promise<PlanMoves> {
    return engine.mutate<PlanMoves>('POST', `/plans/${planId}/schedule:apply`, { item_ids: [...itemIds], zone }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [schedulePath(planId, zone), ...itemIds.map((id) => `/items/${id}`)],
    });
  }

  /** Whether the scheduler may move an entry, and whether it is a milestone (checks 2, PLN-05). */
  setScheduling(planId: string, zone: string, itemId: string, change: { mode?: 'MANUAL' | 'AUTOMATIC'; milestone?: boolean }): Promise<EntryScheduling> {
    return engine.mutate<EntryScheduling>('PUT', `/items/${itemId}/scheduling`, change, {
      invalidates: [schedulePath(planId, zone)],
    });
  }

  /** A dependency's kind and lag (check 1). */
  setTiming(planId: string, zone: string, linkId: string, kind: 'FS' | 'SS' | 'FF' | 'SF', lag: number): Promise<EntryLink> {
    return engine.mutate<EntryLink>('PUT', `/links/${linkId}/timing`, { dependency_kind: kind, lag_days: lag }, {
      invalidates: [schedulePath(planId, zone)],
    });
  }

  /**
   * An entry's dates written from a bar dragged, as `TimelineView` writes them: the start as a
   * scalar, the due as an all-day date. The entry is read first, because the schedule carries no
   * version and a write without one would overwrite a change the reader never saw.
   */
  async moveEntry(planId: string, zone: string, itemId: string, start: string | undefined, due: { due_at: string; due_date_only: boolean; due_time_zone: string } | undefined): Promise<void> {
    const read = await engine.refresh<WorkItem>({ path: `/items/${itemId}` });
    if (read.status === 'failed') throw read.error;
    if (read.status !== 'ready') return;
    let version = read.data.version;
    const touches = [schedulePath(planId, zone), `/items/${itemId}`];
    if (start !== undefined) {
      version = (await engine.mutate<WorkItem>('PATCH', `/items/${itemId}`, { start_at: start }, { ifMatch: etagFor(version), invalidates: touches })).version;
    }
    if (due) await engine.mutate<WorkItem>('PUT', `/items/${itemId}/due`, due, { ifMatch: etagFor(version), invalidates: touches });
  }

  /** A dependency drawn on the chart: the waiting entry depends on the other. */
  link(planId: string, zone: string, sourceId: string, targetId: string, type: string): Promise<EntryLink> {
    return engine.mutate<EntryLink>('POST', `/items/${sourceId}/links`, { target_id: targetId, type }, {
      invalidates: [schedulePath(planId, zone), `/items/${sourceId}/links`, `/items/${targetId}/links`],
    });
  }

  /** Keeps the plan as it is agreed now (PLN-04). */
  takeBaseline(planId: string, name: string): Promise<PlanBaseline> {
    return engine.mutate<PlanBaseline>('POST', baselinesPath(planId), { name }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [baselinesPath(planId)],
    });
  }

  async deleteBaseline(planId: string, baselineId: string): Promise<void> {
    await engine.mutate<void>('DELETE', baselinePath(baselineId), undefined, { invalidates: [baselinesPath(planId), baselinePath(baselineId)] });
  }

  /** Sets the plan's time tolerance, locked for the collections below, or - null - leaves it to the level above. */
  setTolerance(planId: string, zone: string, days: number | null, lock: boolean): Promise<PlanTolerance> {
    return engine.mutate<PlanTolerance>('PUT', `/plans/${planId}/tolerance`, { time_tolerance_days: days, lock }, {
      invalidates: [tolerancePath(planId, zone)],
    });
  }

  /** A baseline beside today's plan. */
  async compare(baselineId: string): Promise<BaselineComparison> {
    const state = await engine.refresh<BaselineComparison>({ path: baselinePath(baselineId) });
    if (state.status === 'failed') throw state.error;
    if (state.status !== 'ready') throw new Error('baseline');
    return state.data;
  }
}

export const planning = new Planning();
