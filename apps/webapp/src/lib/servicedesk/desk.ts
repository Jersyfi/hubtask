// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The service desk screens' arithmetic (UC-SVC-01, 02, 06): working hours from the days and one
 * span, holidays from lines, a goal's targets per priority from `P1=15, P2=60`, and the report's
 * counts per policy and goal. Pure, tested beside.
 */

import type { BusinessCalendar, SlaGoal, SlaReport } from '@hubtask/sync-engine';

export const WEEKDAYS = ['MONDAY', 'TUESDAY', 'WEDNESDAY', 'THURSDAY', 'FRIDAY', 'SATURDAY', 'SUNDAY'] as const;
export type Weekday = (typeof WEEKDAYS)[number];

/** One span on each day chosen, in the week's order. */
export function hoursOf(days: readonly string[], from: string, to: string): BusinessCalendar['hours'] {
  return WEEKDAYS.filter((day) => days.includes(day)).map((day) => ({ day, from, to }));
}

/** The days a calendar works and its first span, for the form that edits it. */
export function formOf(calendar: Pick<BusinessCalendar, 'hours'>): { days: Weekday[]; from: string; to: string } {
  const first = calendar.hours[0];
  return { days: WEEKDAYS.filter((day) => calendar.hours.some((span) => span.day === day)), from: first?.from ?? '08:00', to: first?.to ?? '17:00' };
}

/** Holidays from text, one date per line or separated by commas; blanks dropped. */
export function holidaysOf(text: string): string[] {
  return text.split(/[\s,]+/u).map((word) => word.trim()).filter((word) => word !== '');
}

/** Targets per priority from `P1=15, P2=60`; a malformed pair is undefined, so the form says so. */
export function byPriorityOf(text: string): Record<string, number> | undefined {
  const out: Record<string, number> = {};
  for (const pair of text.split(',').map((each) => each.trim()).filter((each) => each !== '')) {
    const [option, minutes] = pair.split('=').map((each) => each.trim());
    const value = Number(minutes);
    if (!option || !Number.isInteger(value) || value < 1) return undefined;
    out[option] = value;
  }
  return out;
}

/** The text a form shows for a goal's targets per priority. */
export const byPriorityText = (goal: Pick<SlaGoal, 'by_priority'> | undefined) =>
  Object.entries(goal?.by_priority ?? {}).map(([option, minutes]) => `${option}=${minutes}`).join(', ');

export interface ReportRow {
  readonly policy: string;
  readonly goal: 'FIRST_RESPONSE' | 'NEXT_RESPONSE' | 'RESOLUTION';
  readonly met: number;
  readonly breached: number;
  readonly running: number;
  readonly paused: number;
  /** The share of the decided goals that were met, in whole percent; undefined while none is decided. */
  readonly metShare: number | undefined;
}

/** The report's counts as one row per policy and goal, in the order they first appear. */
export function reportRows(report: SlaReport): ReportRow[] {
  const rows = new Map<string, { policy: string; goal: ReportRow['goal']; met: number; breached: number; running: number; paused: number }>();
  for (const cell of report.data) {
    const key = `${cell.policy_key}:${cell.goal}`;
    const row = rows.get(key) ?? { policy: cell.policy_key, goal: cell.goal, met: 0, breached: 0, running: 0, paused: 0 };
    if (cell.standing === 'MET') row.met += cell.entries;
    else if (cell.standing === 'BREACHED') row.breached += cell.entries;
    else if (cell.standing === 'PAUSED') row.paused += cell.entries;
    else row.running += cell.entries;
    rows.set(key, row);
  }
  return [...rows.values()].map((row) => {
    const decided = row.met + row.breached;
    return { ...row, metShare: decided === 0 ? undefined : Math.round((row.met / decided) * 100) };
  });
}

/** A workspace calendar beside the hub's own version of it, if the hub keeps one (UC-SVC-01 check 5). */
export interface HubCalendarRow {
  key: string;
  workspace: BusinessCalendar;
  own: BusinessCalendar | undefined;
}

/** The workspace's calendars by key, each with the hub's own version where it keeps one. */
export function hubCalendarRows(workspace: readonly BusinessCalendar[], own: readonly BusinessCalendar[]): HubCalendarRow[] {
  const byKey = new Map(own.map((row) => [row.key, row]));
  return [...workspace]
    .sort((a, b) => a.key.localeCompare(b.key))
    .map((row) => ({ key: row.key, workspace: row, own: byKey.get(row.key) }));
}

/** How many of a requester's other open requests the side panel lists (UC-SVC-03 check 7). */
export const REQUESTER_OPEN = 5;

/**
 * The requester's other open requests, as the search reads them: by creator, open, newest first -
 * answering only what the reader may see. Asked one larger, since the request itself is among them.
 */
export function requesterRequests(accountId: string) {
  return {
    filter: {
      op: 'AND',
      nodes: [
        { op: 'EQ', field: 'created_by', value: accountId },
        { op: 'EQ', field: 'is_completed', value: false },
      ],
    },
    sort: [{ field: 'created_at', dir: 'DESC' }],
    page: { size: REQUESTER_OPEN + 1 },
  };
}

/** The others of a requester's open requests, the one being read left out. */
export function otherRequests<T extends { id: string }>(rows: readonly T[], selfId: string): T[] {
  return rows.filter((row) => row.id !== selfId).slice(0, REQUESTER_OPEN);
}
