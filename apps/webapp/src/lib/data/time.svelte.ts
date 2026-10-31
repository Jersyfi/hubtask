// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The time module's paths and writes (UC-TIM-01 … 08), so that every write invalidates the same
 * reads: the timer, the week it lands in, the approval queue, the kinds of work, the policy, a
 * person's capacity, the absences, the cost rates, a plan's billing rates and its budget.
 * The reads are subscriptions at the call site.
 *
 * **Offline**, the timer is started, stopped and discarded from the queue with the device's moments
 * (`replica.ts`, `timerOf`), and the timer, a person's own week, capacities, the absences and the
 * rates are read from the copy; every other time write goes online, and the engine refuses it made
 * offline (`sync.needs_connection`).
 */

import type {
  Absence, BillingRate, BudgetBurn, Capacity, CostRate, TimeExport, TimePolicy, TimeRecord, TimeTimer, Timesheet, TransportDocument, WorkType,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export * from './time.ts';

export const TIMER = '/timer';
export const WORK_TYPES = '/time-work-types';
export const POLICY = '/time-policy';
export const SPREADSHEET = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
/** A week of the caller's, or of another person's where the caller approves it. */
export const weekPath = (week: string, accountId?: string) =>
  accountId ? `/timesheets/${week}?account_id=${encodeURIComponent(accountId)}` : `/timesheets/${week}`;
/** The weeks handed in that the caller may decide, between two days. */
export const queuePath = (from: string, to: string) => `/timesheets?from=${from}&to=${to}&state=SUBMITTED`;
/** A hub's or a collection's estimates against the time recorded. */
export const actualsPath = (planId: string) => `/plans/${planId}/actuals`;
/** A hub's or a collection's workload from a Monday for some weeks. */
export const workloadPath = (planId: string, from: string, weeks: number) => `/plans/${planId}/workload?from=${from}&weeks=${weeks}`;
/** A hub's or a collection's billing rates: what an hour of its billable time is charged. */
export const billingRatesPath = (planId: string) => `/plans/${planId}/billing-rates`;
/** A hub's or a collection's budget and its burn. */
export const budgetPath = (planId: string) => `/plans/${planId}/budget`;
/** The caller's capacities. */
export const CAPACITIES = '/time-capacities';
/** The shared calendar of absences between two days. */
export const absencesPath = (from: string, to: string) => `/time-absences?from=${from}&to=${to}`;
export const RATES = '/cost-rates';

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
  /** Also each record's rate and cost (UC-TIM-05 check 5); only for whoever manages costs. */
  costs?: boolean;
}

function exportBody(request: ReportRequest): Record<string, unknown> {
  return { from: request.from, to: request.to, mine: request.mine, ...(request.costs ? { costs: true } : {}) };
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
    return engine.mutate<TimeExport>('POST', '/time-records:export', exportBody(request), { invalidates: [] });
  }

  /**
   * The same rows as one sheet of a spreadsheet, rendered by the server: the client has no writer
   * for the format, and the cells keep their types there (minutes as numbers, a text never a formula).
   */
  exportSpreadsheet(request: ReportRequest): Promise<TransportDocument> {
    return engine.document('/time-records:export', exportBody(request), {
      timeoutMs: 120_000, accept: SPREADSHEET,
    });
  }

  setCapacity(from: string, weekMinutes: readonly number[]): Promise<Capacity> {
    return engine.mutate<Capacity>('PUT', `${CAPACITIES}/${from}`, { week_minutes: weekMinutes }, { invalidates: [CAPACITIES] });
  }

  async removeCapacity(from: string): Promise<void> {
    await engine.mutate<void>('DELETE', `${CAPACITIES}/${from}`, undefined, { invalidates: [CAPACITIES] });
  }

  /** Records an absence of the caller's; `calendar` is the month's read it changes. */
  recordAbsence(fields: { from: string; to: string; kind: string; half: boolean }, calendar: string): Promise<Absence> {
    return engine.mutate<Absence>('POST', '/time-absences', fields, { idempotencyKey: crypto.randomUUID(), invalidates: [calendar] });
  }

  updateAbsence(absence: Absence, fields: { from: string; to: string; kind: string; half: boolean }, calendar: string): Promise<Absence> {
    return engine.mutate<Absence>('PATCH', `/time-absences/${absence.id}`, fields, { ifMatch: etagFor(absence.version), invalidates: [calendar] });
  }

  async removeAbsence(absence: Absence, calendar: string): Promise<void> {
    await engine.mutate<void>('DELETE', `/time-absences/${absence.id}`, undefined, { ifMatch: etagFor(absence.version), invalidates: [calendar] });
  }

  addRate(fields: { account_id?: string; role?: string; hour_minor: number; currency: string; from: string; to?: string }): Promise<CostRate> {
    return engine.mutate<CostRate>('POST', RATES, fields, { idempotencyKey: crypto.randomUUID(), invalidates: [RATES] });
  }

  changeRate(rate: CostRate, fields: { hour_minor: number; currency: string; from: string; to?: string }): Promise<CostRate> {
    return engine.mutate<CostRate>('PATCH', `${RATES}/${rate.id}`, fields, { ifMatch: etagFor(rate.version), invalidates: [RATES] });
  }

  async removeRate(rate: CostRate): Promise<void> {
    await engine.mutate<void>('DELETE', `${RATES}/${rate.id}`, undefined, { ifMatch: etagFor(rate.version), invalidates: [RATES] });
  }

  addBillingRate(planId: string, fields: { hour_minor: number; currency: string; from: string; to?: string }): Promise<BillingRate> {
    const path = billingRatesPath(planId);
    return engine.mutate<BillingRate>('POST', path, fields, { idempotencyKey: crypto.randomUUID(), invalidates: [path] });
  }

  changeBillingRate(rate: BillingRate, fields: { hour_minor: number; currency: string; from: string; to?: string }): Promise<BillingRate> {
    const path = billingRatesPath(rate.plan_id);
    return engine.mutate<BillingRate>('PATCH', `${path}/${rate.id}`, fields, { ifMatch: etagFor(rate.version), invalidates: [path] });
  }

  async removeBillingRate(rate: BillingRate): Promise<void> {
    const path = billingRatesPath(rate.plan_id);
    await engine.mutate<void>('DELETE', `${path}/${rate.id}`, undefined, { ifMatch: etagFor(rate.version), invalidates: [path] });
  }

  setBudget(planId: string, burn: BudgetBurn, fields: { minutes: number; amount_minor: number; currency: string }): Promise<BudgetBurn> {
    return engine.mutate<BudgetBurn>('PUT', budgetPath(planId), { ...fields, expected_version: burn.version }, { invalidates: [budgetPath(planId)] });
  }
}

export const time = new Time();
