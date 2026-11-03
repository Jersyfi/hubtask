// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The service desk screens' arithmetic (UC-SVC-01, 02, 06): working hours from the days and one
 * span, holidays from lines, a goal's targets per priority from `P1=15, P2=60`, and the report's
 * counts per policy and goal. Pure, tested beside.
 */

import type { BusinessCalendar, SlaGoal, SlaPolicyDefine, SlaReport } from '@hubtask/sync-engine';

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

/**
 * A calendar's business day in minutes: what it works on each day it works, where every such day
 * is alike; undefined otherwise, since days of different lengths have no one business day.
 */
export function dayLengthOf(calendar: Pick<BusinessCalendar, 'hours'>): number | undefined {
  const perDay = new Map<string, number>();
  const minute = (time: string) => { const [h, m] = time.split(':').map(Number); return (h ?? 0) * 60 + (m ?? 0); };
  for (const span of calendar.hours) perDay.set(span.day, (perDay.get(span.day) ?? 0) + minute(span.to) - minute(span.from));
  const lengths = new Set(perDay.values());
  const [only] = lengths;
  return lengths.size === 1 && only !== undefined && only > 0 ? only : undefined;
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

/**
 * A hub's settings: the SLA policies shared with it, read-only there, and its own (UC-SVC-02
 * check 13) - never another hub's, which measures none of this hub's entries.
 */
export function policiesAtHub<P extends { readonly hub_id?: string | null }>(policies: readonly P[], hub: string): { readonly shared: readonly P[]; readonly own: readonly P[] } {
  return { shared: policies.filter((p) => !p.hub_id), own: policies.filter((p) => p.hub_id === hub) };
}

/** A key free in the workspace for a copy of the policy: `<key>_copy`, then `_2`, `_3`, … */
export function policyCopyKey(policies: readonly { readonly key: string }[], key: string): string {
  const taken = new Set(policies.map((p) => p.key));
  const stem = `${key}_copy`.slice(0, 44);
  if (!taken.has(stem)) return stem;
  for (let n = 2; ; n++) {
    const candidate = `${stem}_${n}`;
    if (!taken.has(candidate)) return candidate;
  }
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

/**
 * An entry's goal as one chip's key: the requester's promise holds each goal once, but every team's
 * OLA holds its own (UC-SVC-02 check 12), so the goal alone repeats. A record copied offline before
 * OLAs existed carries no kind and is the requester's.
 */
export function goalKey(goal: { kind?: string; policy_key: string; goal: string }): string {
  return `${goal.kind ?? 'SLA'}:${goal.policy_key}:${goal.goal}`;
}

/** The policy form's words as the contract takes them. */
export interface PolicyForm {
  name: string; types: string; organisations: string; calendar: string; priority: string; position: string; enabled: boolean;
  /** The group whose OLA the policy is; empty for the requester's promise. */
  group: string;
}

const commaWords = (text: string) => text.split(',').map((each) => each.trim()).filter((each) => each !== '');

/**
 * A policy to define from its form. The group is sent whenever it is set, so that saving an OLA
 * keeps it one (UC-SVC-02 check 10); an OLA names no organisations, which the server refuses.
 */
export function policyDefinitionOf(form: PolicyForm, goals: SlaPolicyDefine['goals']): SlaPolicyDefine {
  const group = form.group.trim();
  return {
    name: form.name.trim(), entry_types: commaWords(form.types), organisations: group ? [] : commaWords(form.organisations), goals,
    position: Number(form.position) || 0, enabled: form.enabled,
    ...(form.calendar ? { calendar: form.calendar } : {}), ...(form.priority.trim() ? { priority_field: form.priority.trim() } : {}),
    ...(group ? { assignee_group: group } : {}),
  };
}
