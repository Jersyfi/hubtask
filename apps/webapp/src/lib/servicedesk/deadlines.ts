// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A ticket's deadlines as the rail draws them (ET-6, second round): one row per deadline, the
// most urgent first - breached, then at risk, then by the share of time used, the kept ones last -
// and the head's one chip: what is breached, else the next one due.

import type { EntrySlaGoal } from '@hubtask/sync-engine';

export type Urgency = 'breached' | 'atRisk' | 'running' | 'paused' | 'met';

export function urgencyOf(goal: EntrySlaGoal, now: number): Urgency {
  if (goal.standing === 'BREACHED') return 'breached';
  if (goal.standing === 'MET') return 'met';
  if (goal.standing === 'PAUSED') return 'paused';
  // Past due but not yet marked: the sweep has not run; it reads as breached (as `queueSla`).
  if (goal.due_at !== null && Date.parse(goal.due_at) <= now) return 'breached';
  return goal.at_risk_at !== null && Date.parse(goal.at_risk_at) <= now ? 'atRisk' : 'running';
}

/** How much of the time is used, 0 to 100; a breached one is full, a kept one what it took. */
export function shareOf(goal: EntrySlaGoal, now: number): number {
  if (goal.standing === 'BREACHED') return 100;
  const start = goal.armed_at ? Date.parse(goal.armed_at) : NaN;
  const due = goal.due_at ? Date.parse(goal.due_at) : NaN;
  if (Number.isNaN(start) || Number.isNaN(due) || due <= start) return 0;
  const end = goal.met_at ? Date.parse(goal.met_at) : now;
  return Math.max(0, Math.min(100, Math.round(((end - start) / (due - start)) * 100)));
}

const RANK: Record<Urgency, number> = { breached: 0, atRisk: 1, running: 2, paused: 3, met: 4 };

export function byUrgency(goals: readonly EntrySlaGoal[], now: number): EntrySlaGoal[] {
  return [...goals].sort(
    (a, b) => RANK[urgencyOf(a, now)] - RANK[urgencyOf(b, now)] || shareOf(b, now) - shareOf(a, now) || (a.due_at ?? '').localeCompare(b.due_at ?? ''),
  );
}

/** The head's one chip: the first breached deadline, else the next one running. */
export function chipOf(goals: readonly EntrySlaGoal[], now: number): { goal: EntrySlaGoal; isBreached: boolean } | undefined {
  const sorted = byUrgency(goals, now);
  const first = sorted.find((goal) => urgencyOf(goal, now) !== 'met' && urgencyOf(goal, now) !== 'paused');
  return first ? { goal: first, isBreached: urgencyOf(first, now) === 'breached' } : undefined;
}

/**
 * What a row says of its time: how long ago it fell due, or how much is left; nothing for a goal
 * standing still, kept or without a due moment.
 */
export function timeOf(goal: EntrySlaGoal, now: number): { isOverdue: boolean; span: number } | undefined {
  if (goal.standing === 'MET' || goal.standing === 'PAUSED') return undefined;
  const due = goal.due_at ? Date.parse(goal.due_at) : NaN;
  const breached = goal.breached_at ? Date.parse(goal.breached_at) : NaN;
  if (goal.standing === 'BREACHED') {
    const since = Number.isNaN(due) ? breached : due;
    return Number.isNaN(since) ? undefined : { isOverdue: true, span: Math.max(0, now - since) };
  }
  if (Number.isNaN(due)) return undefined;
  return due <= now ? { isOverdue: true, span: now - due } : { isOverdue: false, span: due - now };
}
