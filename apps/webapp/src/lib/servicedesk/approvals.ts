// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Approvals as the screens draw them (UC-SVC-08 to 10): a level's standing, the trail in the order
 * it happened, and what the delegation form refuses before the server would. Pure, tested beside.
 */

import type { Approval } from '@hubtask/sync-engine';

type Decision = Approval['decisions'][number];

/** The yeses a level has so far, beside how many pass it. */
export function levelStanding(approval: Pick<Approval, 'levels' | 'decisions'>, level: number): { yes: number; needed: number } {
  const needed = approval.levels[level - 1]?.needed ?? 0;
  const yes = approval.decisions.filter((decision) => decision.level === level && decision.verdict === 'APPROVE').length;
  return { yes, needed };
}

/** The trail oldest first: who decided, for whom, when, why, on which version. */
export function trailOf(approval: Pick<Approval, 'decisions'>): readonly Decision[] {
  return [...approval.decisions].sort((a, b) => a.decided_at.localeCompare(b.decided_at) || a.level - b.level);
}

/** Whether the entry changed since the approval was asked - the trail says so beside a decision. */
export const decidedOnOlderVersion = (approval: Pick<Approval, 'item_version'>, decision: Pick<Decision, 'item_version'>) =>
  decision.item_version !== approval.item_version;

/** A badge's tone for where an approval stands. */
export function stateTone(state: Approval['state']): 'info' | 'success' | 'danger' | 'neutral' {
  if (state === 'APPROVED') return 'success';
  if (state === 'DECLINED') return 'danger';
  if (state === 'WITHDRAWN') return 'neutral';
  return 'info';
}

const YEAR_MS = 366 * 24 * 60 * 60 * 1000;

/**
 * Why a delegation cannot be set as written, as a message code, or undefined: nobody chosen,
 * oneself, an end before its start, or longer than a year - the server's own refusals, said early.
 */
export function delegationProblem(delegate: string, self: string | undefined, from: string, until: string): string | undefined {
  if (!delegate) return 'app.approvals.delegate_first';
  if (delegate === self) return 'app.approvals.delegate_not_self';
  const start = Date.parse(from);
  const end = Date.parse(until);
  if (Number.isNaN(start) || Number.isNaN(end)) return 'app.approvals.delegate_dates';
  if (end <= start) return 'app.approvals.delegate_dates';
  if (end - start > YEAR_MS) return 'app.approvals.delegate_year';
  return undefined;
}
