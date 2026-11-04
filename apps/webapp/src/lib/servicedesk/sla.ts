// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one SLA a queue row shows (user-experience.md §7.4): the requester's promise that breaks
 * first - a breached one before a running one, a running one by its due moment, then one standing
 * still, then one kept - as words, a mark and how much of its time is used, never a colour alone.
 * A team's OLA stands in only where no requester's promise measures the entry. Pure, tested beside.
 */

import type { EntrySlaList } from '@hubtask/sync-engine';

type Goal = EntrySlaList['data'][number];

export type SlaState = 'breached' | 'at_risk' | 'running' | 'paused' | 'met';

export interface QueueSla {
  readonly state: SlaState;
  /** The message code of the words and its parameters; `span` is filled by `spanOf`. */
  readonly code: string;
  /** Milliseconds left (running) or past (breached); absent where no clock runs. */
  readonly span?: number;
  /** How much of the goal's time is used, 0 to 100; absent where it cannot be said. */
  readonly used?: number;
  readonly goal: Goal['goal'];
}

const RANK: Readonly<Record<Goal['standing'], number>> = { BREACHED: 0, RUNNING: 1, PAUSED: 2, MET: 3 };

/** The goal a row stands for, or undefined where nothing measures the entry. */
export function firstToBreak(goals: readonly Goal[]): Goal | undefined {
  const own = goals.filter((goal) => goal.kind === 'SLA');
  const pool = own.length > 0 ? own : goals;
  return [...pool].sort((a, b) => {
    const byRank = RANK[a.standing] - RANK[b.standing];
    if (byRank !== 0) return byRank;
    return Date.parse(a.due_at ?? a.breached_at ?? a.met_at ?? a.armed_at) - Date.parse(b.due_at ?? b.breached_at ?? b.met_at ?? b.armed_at);
  })[0];
}

/** What the queue row says of an entry's SLA at `now`. */
export function queueSla(goals: readonly Goal[], now: Date): QueueSla | undefined {
  const goal = firstToBreak(goals);
  if (!goal) return undefined;
  const at = now.getTime();
  switch (goal.standing) {
    case 'BREACHED':
      return { state: 'breached', code: 'app.desk_queues.sla_breached', span: goal.breached_at ? Math.max(0, at - Date.parse(goal.breached_at)) : undefined, used: 100, goal: goal.goal };
    case 'PAUSED':
      return { state: 'paused', code: 'app.desk_queues.sla_paused', goal: goal.goal };
    case 'MET':
      return { state: 'met', code: 'app.desk_queues.sla_met', goal: goal.goal };
    default: {
      if (!goal.due_at) return { state: 'running', code: 'app.desk_queues.sla_running', goal: goal.goal };
      const due = Date.parse(goal.due_at);
      // Past due but not yet marked: the sweep has not run; it reads as breached, never as time left.
      if (due <= at) return { state: 'breached', code: 'app.desk_queues.sla_breached', span: at - due, used: 100, goal: goal.goal };
      const start = Date.parse(goal.armed_at);
      // Wall time between arming and the due moment: the calendar's pauses are in that span, so
      // the bar is an approximation; the words are exact.
      const used = due > start ? Math.min(100, Math.max(0, Math.round(((at - start) / (due - start)) * 100))) : undefined;
      const isAtRisk = goal.at_risk_at !== null && Date.parse(goal.at_risk_at) <= at;
      return { state: isAtRisk ? 'at_risk' : 'running', code: 'app.desk_queues.sla_left', span: due - at, used, goal: goal.goal };
    }
  }
}

/** A span as the code of its words and their parameters: minutes, hours and minutes, days and hours. */
export function spanOf(ms: number): { code: string; params: Record<string, string> } {
  const minutes = Math.max(1, Math.round(ms / 60_000));
  if (minutes < 60) return { code: 'app.desk_queues.span_minutes', params: { minutes: String(minutes) } };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { code: 'app.desk_queues.span_hours', params: { hours: String(hours), minutes: String(minutes % 60) } };
  return { code: 'app.desk_queues.span_days', params: { days: String(Math.floor(hours / 24)), hours: String(hours % 24) } };
}
