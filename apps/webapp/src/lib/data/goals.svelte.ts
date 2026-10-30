// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The goals module's paths and writes (UC-GOL-01 … 04), so that every write invalidates the same
 * reads: the periods, the goals and one goal, its check-ins, the portfolios and their updates, the
 * dashboards and their figures. The reads are subscriptions at the call site.
 *
 * **Written online.** Periods, goals, check-ins and dashboards reach a device's replica as
 * definitions, but a write needs the connection: the engine refuses one made offline
 * (`sync.needs_connection`) rather than queueing it, and every number is asked for online.
 */

import type {
  Dashboard, DashboardCreate, DashboardDigest, DashboardDigestWrite, DashboardWrite, Goal, GoalCreate, GoalPeriod, GoalWrite, Portfolio, PortfolioUpdate, PortfolioWrite,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { etagFor } from './etag.ts';

export * from './goals.ts';

export const PERIODS = '/goal-periods';
export const PORTFOLIOS = '/portfolios';
export const DASHBOARDS = '/dashboards';

/** A period's goals, or every goal the reader may read; a hub's alone with `hubId`. */
export function goalsPath(periodId?: string, hubId?: string): string {
  const query = new URLSearchParams();
  if (periodId) query.set('period_id', periodId);
  if (hubId) query.set('hub_id', hubId);
  const text = query.toString();
  return text === '' ? '/goals' : `/goals?${text}`;
}
export const goalPath = (goalId: string) => `/goals/${goalId}`;
/** The goals an entry serves that the reader may read. */
export const entryGoalsPath = (itemId: string) => `/items/${itemId}/goals`;
export const checkInsPath = (goalId: string) => `/goals/${goalId}/check-ins`;
export const portfolioPath = (portfolioId: string) => `/portfolios/${portfolioId}`;
export const updatesPath = (portfolioId: string) => `/portfolios/${portfolioId}/updates`;
export const dashboardsPath = (hubId?: string) => (hubId ? `/dashboards?hub_id=${hubId}` : DASHBOARDS);
export const dashboardPath = (dashboardId: string) => `/dashboards/${dashboardId}`;
export const figuresPath = (dashboardId: string) => `/dashboards/${dashboardId}/figures`;
/** The reader's own digest of a dashboard by mail; not found where they have none. */
export const digestPath = (dashboardId: string) => `/dashboards/${dashboardId}/digest`;

/** A goal's change reaches its parent's progress and every list, so every goal read is stale. */
const GOAL_READS = ['/goals', DASHBOARDS];

export interface CheckIn {
  status: 'ON_TRACK' | 'AT_RISK' | 'OFF_TRACK' | 'DONE' | 'DROPPED';
  confidence: number;
  note?: string;
  values?: { key_result_id: string; value: number }[];
}

class Goals {
  definePeriod(fields: { name: string; start_on: string; end_on: string }): Promise<GoalPeriod> {
    return engine.mutate<GoalPeriod>('POST', PERIODS, fields, { idempotencyKey: crypto.randomUUID(), invalidates: [PERIODS] });
  }

  /** Closes a period; a goal without a grade here is graded by its progress. */
  closePeriod(period: GoalPeriod, grades: Record<string, number> = {}): Promise<GoalPeriod> {
    return engine.mutate<GoalPeriod>('POST', `${PERIODS}/${period.id}:close`, { grades }, {
      ifMatch: etagFor(period.version),
      invalidates: [PERIODS, ...GOAL_READS],
    });
  }

  create(fields: GoalCreate): Promise<Goal> {
    return engine.mutate<Goal>('POST', '/goals', fields, { idempotencyKey: crypto.randomUUID(), invalidates: GOAL_READS });
  }

  update(goal: Goal, fields: GoalWrite): Promise<Goal> {
    return engine.mutate<Goal>('PUT', goalPath(goal.id), fields, { ifMatch: etagFor(goal.version), invalidates: GOAL_READS });
  }

  async remove(goal: Goal): Promise<void> {
    await engine.mutate<void>('DELETE', goalPath(goal.id), undefined, { ifMatch: etagFor(goal.version), invalidates: GOAL_READS });
  }

  link(goalId: string, entryIds: string[]): Promise<Goal> {
    return engine.mutate<Goal>('POST', `${goalPath(goalId)}/entries`, { entry_ids: entryIds }, { invalidates: [...GOAL_READS, '/items'] });
  }

  unlink(goalId: string, entryId: string): Promise<Goal> {
    return engine.mutate<Goal>('DELETE', `${goalPath(goalId)}/entries/${entryId}`, undefined, { invalidates: [...GOAL_READS, '/items'] });
  }

  checkIn(goalId: string, fields: CheckIn): Promise<Goal> {
    return engine.mutate<Goal>('POST', checkInsPath(goalId), fields, { idempotencyKey: crypto.randomUUID(), invalidates: GOAL_READS });
  }

  createPortfolio(fields: PortfolioWrite): Promise<Portfolio> {
    return engine.mutate<Portfolio>('POST', PORTFOLIOS, fields, { idempotencyKey: crypto.randomUUID(), invalidates: [PORTFOLIOS] });
  }

  updatePortfolio(portfolio: Portfolio, fields: PortfolioWrite): Promise<Portfolio> {
    return engine.mutate<Portfolio>('PUT', portfolioPath(portfolio.id), fields, { ifMatch: etagFor(portfolio.version), invalidates: [PORTFOLIOS] });
  }

  async removePortfolio(portfolio: Portfolio): Promise<void> {
    await engine.mutate<void>('DELETE', portfolioPath(portfolio.id), undefined, { ifMatch: etagFor(portfolio.version), invalidates: [PORTFOLIOS] });
  }

  reportStatus(portfolioId: string, fields: { status: 'ON_TRACK' | 'AT_RISK' | 'OFF_TRACK'; summary: string }): Promise<PortfolioUpdate> {
    return engine.mutate<PortfolioUpdate>('POST', updatesPath(portfolioId), fields, { idempotencyKey: crypto.randomUUID(), invalidates: [PORTFOLIOS] });
  }

  createDashboard(fields: DashboardCreate): Promise<Dashboard> {
    return engine.mutate<Dashboard>('POST', DASHBOARDS, fields, { idempotencyKey: crypto.randomUUID(), invalidates: [DASHBOARDS] });
  }

  updateDashboard(dashboard: Dashboard, fields: DashboardWrite): Promise<Dashboard> {
    return engine.mutate<Dashboard>('PUT', dashboardPath(dashboard.id), fields, { ifMatch: etagFor(dashboard.version), invalidates: [DASHBOARDS] });
  }

  async removeDashboard(dashboard: Dashboard): Promise<void> {
    await engine.mutate<void>('DELETE', dashboardPath(dashboard.id), undefined, { ifMatch: etagFor(dashboard.version), invalidates: [DASHBOARDS] });
  }

  /** Mails the reader a dashboard they see, replacing their digest of it (UC-GOL-04 check 9). */
  setDigest(dashboardId: string, fields: DashboardDigestWrite): Promise<DashboardDigest> {
    return engine.mutate<DashboardDigest>('PUT', digestPath(dashboardId), fields, { invalidates: [digestPath(dashboardId)] });
  }

  async removeDigest(dashboardId: string): Promise<void> {
    await engine.mutate<void>('DELETE', digestPath(dashboardId), undefined, { invalidates: [digestPath(dashboardId)] });
  }
}

export const goals = new Goals();
