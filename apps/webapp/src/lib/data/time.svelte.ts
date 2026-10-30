// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The time module's paths and writes (UC-TIM-01 … 05), so that every write invalidates the same
 * reads: the timer, the week it lands in, the approval queue, the kinds of work and the policy.
 * The reads are subscriptions at the call site.
 *
 * **Online only.** Time is not in the replica yet, so the engine refuses a write made offline
 * (`sync.needs_connection`) rather than queueing it; UC-TIM-01 *Where it ends* names the timer.
 */

import type { TimeExport, TimePolicy, TimeRecord, TimeTimer, Timesheet, WorkType } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export * from './time.ts';

export const TIMER = '/timer';
export const WORK_TYPES = '/time-work-types';
export const POLICY = '/time-policy';
/** A week of the caller's, or of another person's where the caller approves it. */
export const weekPath = (week: string, accountId?: string) =>
  accountId ? `/timesheets/${week}?account_id=${encodeURIComponent(accountId)}` : `/timesheets/${week}`;
/** The weeks handed in that the caller may decide, between two days. */
export const queuePath = (from: string, to: string) => `/timesheets?from=${from}&to=${to}&state=SUBMITTED`;
/** A hub's or a collection's estimates against the time recorded. */
export const actualsPath = (planId: string) => `/plans/${planId}/actuals`;

export interface RecordFields {
  entry_id?: string | null;
  work_type?: string;
  day?: string;
  minutes?: number;
  note?: string;
  billable?: boolean;
}

export interface ReportRequest {
  from: string;
  to: string;
  mine?: boolean;
  group_by?: string[];
}

class Time {
  /** Records time; `weeks` are the weeks whose reads it changes. */
  log(fields: RecordFields, weeks: readonly string[]): Promise<TimeRecord> {
    return engine.mutate<TimeRecord>('POST', '/time-records', fields, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: weeks.map((week) => weekPath(week)),
    });
  }

  update(record: TimeRecord, fields: RecordFields, weeks: readonly string[]): Promise<TimeRecord> {
    return engine.mutate<TimeRecord>('PATCH', `/time-records/${record.id}`, fields, {
      ifMatch: etagFor(record.version),
      invalidates: weeks.map((week) => weekPath(week)),
    });
  }

  async remove(record: TimeRecord, weeks: readonly string[]): Promise<void> {
    await engine.mutate<void>('DELETE', `/time-records/${record.id}`, undefined, {
      ifMatch: etagFor(record.version),
      invalidates: weeks.map((week) => weekPath(week)),
    });
  }

  /** A correction of signed minutes on a day of an open week, naming an approved or locked record. */
  correct(recordId: string, fields: { minutes: number; day: string; note?: string }, weeks: readonly string[]): Promise<TimeRecord> {
    return engine.mutate<TimeRecord>('POST', `/time-records/${recordId}:correct`, fields, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: weeks.map((week) => weekPath(week)),
    });
  }

  startTimer(fields: { entry_id?: string; work_type?: string; note?: string }): Promise<TimeTimer> {
    return engine.mutate<TimeTimer>('POST', `${TIMER}:start`, fields, { invalidates: [TIMER] });
  }

  /** Stops the timer into the day it started in `zone`; `week` is that day's week. */
  stopTimer(zone: string, week: string | undefined): Promise<TimeRecord> {
    return engine.mutate<TimeRecord>('POST', `${TIMER}:stop`, { zone }, { invalidates: week ? [TIMER, weekPath(week)] : [TIMER] });
  }

  async discardTimer(): Promise<void> {
    await engine.mutate<void>('DELETE', TIMER, undefined, { invalidates: [TIMER] });
  }

  copyPrevious(week: string): Promise<{ week_start: string; copied: number }> {
    return engine.mutate('POST', `/timesheets/${week}:copy-previous`, undefined, { invalidates: [weekPath(week)] });
  }

  submit(week: string): Promise<Timesheet> {
    return engine.mutate<Timesheet>('POST', `/timesheets/${week}:submit`, undefined, { invalidates: [weekPath(week)] });
  }

  withdraw(week: string): Promise<Timesheet> {
    return engine.mutate<Timesheet>('POST', `/timesheets/${week}:withdraw`, undefined, { invalidates: [weekPath(week)] });
  }

  approve(week: string, accountId: string, queue: string): Promise<Timesheet> {
    return engine.mutate<Timesheet>('POST', `/timesheets/${week}:approve`, { account_id: accountId }, {
      invalidates: [queue, weekPath(week, accountId)],
    });
  }

  reject(week: string, accountId: string, reason: string, queue: string): Promise<Timesheet> {
    return engine.mutate<Timesheet>('POST', `/timesheets/${week}:reject`, { account_id: accountId, reason }, {
      invalidates: [queue, weekPath(week, accountId)],
    });
  }

  defineWorkType(key: string, fields: { name: string; billable?: boolean; archived?: boolean; position?: number }): Promise<WorkType> {
    return engine.mutate<WorkType>('PUT', `${WORK_TYPES}/${key}`, fields, { invalidates: [WORK_TYPES] });
  }

  setPolicy(policy: TimePolicy, fields: { locked_through: string | null; visibility: 'FULL' | 'AGGREGATED' }): Promise<TimePolicy> {
    return engine.mutate<TimePolicy>('PUT', POLICY, { ...fields, expected_version: policy.version }, { invalidates: [POLICY] });
  }

  /** The rows of an export: a write, because the export is recorded in the audit trail. */
  export(request: ReportRequest): Promise<TimeExport> {
    return engine.mutate<TimeExport>('POST', '/time-records:export', { from: request.from, to: request.to, mine: request.mine }, { invalidates: [] });
  }
}

export const time = new Time();
