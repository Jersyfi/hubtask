// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The Gantt screen's arithmetic (UC-PLN-02 … 05): a schedule turned into rows in the order of the
 * tree, a baseline laid beside them, the moves a planner may accept, and a lag read from a field.
 *
 * Pure, so that "which row is under which" and "what is a lag" are checked by `node --test` rather
 * than by driving a browser - the reason `schedule.ts` gives.
 *
 * **The server schedules; this module only orders and labels.** Every date drawn is the one
 * `GET /plans/{id}/schedule` answered: a client that re-scheduled would be a second scheduler,
 * and two schedulers disagree.
 */

import type { BaselineComparison, PlanImport, PlanSchedule, PlanScheduleEntry } from '@hubtask/sync-engine';

import type { PlanWords } from './imports.ts';
import type { Path } from './people.ts';
import type { Scale } from './schedule.ts';
import { xlsxOf } from './xlsx.ts';

/**
 * The path whose roles decide what the plan's reader may do: the hub's own for a hub's plan, the
 * collection under its hub for a collection's. None while the place is not read, since a guess
 * would ask a hub's memberships as a collection's.
 */
export function planRolePath(planId: string, place: { type: string; parent_id?: string | null } | undefined): Path | undefined {
  if (!place) return undefined;
  return place.type === 'HUB' ? { hubId: planId } : { hubId: place.parent_id ?? undefined, collectionId: planId };
}

/** One row of the chart and of the table beside it. */
export interface GanttRow {
  readonly id: string;
  readonly title: string;
  readonly start: string;
  readonly due: string;
  readonly kind?: 'milestone' | 'summary';
  readonly isCritical: boolean;
  readonly depth: number;
  readonly baseline?: { readonly start?: string; readonly due?: string };
  readonly entry: PlanScheduleEntry;
}

/**
 * The plan's entries in the order of the tree: each parent, then its children, depth first, the
 * server's order kept among siblings. An entry whose parent is not in the answer - outside the
 * plan, or out of the reader's reach - is drawn at the top rather than dropped.
 */
export function inTreeOrder(entries: readonly PlanScheduleEntry[]): { entry: PlanScheduleEntry; depth: number }[] {
  const ids = new Set(entries.map((entry) => entry.item_id));
  const children = new Map<string, PlanScheduleEntry[]>();
  const roots: PlanScheduleEntry[] = [];
  for (const entry of entries) {
    const parent = entry.parent_id ?? undefined;
    if (parent && ids.has(parent) && parent !== entry.item_id) {
      children.set(parent, [...(children.get(parent) ?? []), entry]);
    } else roots.push(entry);
  }
  const out: { entry: PlanScheduleEntry; depth: number }[] = [];
  const seen = new Set<string>();
  const walk = (entry: PlanScheduleEntry, depth: number) => {
    if (seen.has(entry.item_id)) return;
    seen.add(entry.item_id);
    out.push({ entry, depth });
    for (const child of children.get(entry.item_id) ?? []) walk(child, depth + 1);
  };
  for (const root of roots) walk(root, 0);
  // A parent loop the server would never store still draws every entry once.
  for (const entry of entries) walk(entry, 0);
  return out;
}

/**
 * The rows the chart draws: the schedule's dates, the kind of bar, and the baseline's dates where
 * one is laid beside the plan. `toDay` turns a baseline's instant into the reader's day.
 */
export function ganttRows(
  schedule: PlanSchedule,
  comparison: BaselineComparison | undefined,
  toDay: (instant: string) => string | undefined,
): GanttRow[] {
  const kept = new Map((comparison?.entries ?? []).filter((each) => each.in_baseline).map((each) => [each.item_id, each]));
  return inTreeOrder(schedule.entries).map(({ entry, depth }) => {
    const was = kept.get(entry.item_id);
    const baseline = was
      ? {
          start: was.baseline_start_at ? toDay(was.baseline_start_at) : undefined,
          due: was.baseline_due_at ? toDay(was.baseline_due_at) : undefined,
        }
      : undefined;
    return {
      id: entry.item_id,
      title: entry.title,
      start: entry.start,
      due: entry.finish,
      kind: entry.milestone || entry.due_only === true ? 'milestone' : entry.summary ? 'summary' : undefined,
      isCritical: entry.critical,
      depth,
      baseline: baseline && (baseline.start || baseline.due) ? baseline : undefined,
      entry,
    };
  });
}

/**
 * How many days each scale draws in the chart's usual width, at its column of the space scale:
 * a day column is three steps, a week's day one, a month's half, a quarter's a quarter.
 */
const FITS: readonly (readonly [Scale, number])[] = [['day', 35], ['week', 110], ['month', 220], ['quarter', 440]];

/**
 * The window that shows the whole plan (user-experience.md §7.5): from its first date to its last,
 * a day or a twentieth of its length to spare on each side, at the finest scale that holds it.
 * Undefined for a plan with no dates, which keeps the window where the reader is.
 */
export function fitWindow(spans: readonly { readonly start?: string; readonly due?: string }[]): { from: string; to: string; scale: Scale } | undefined {
  const days = spans.flatMap((span) => [span.start, span.due]).filter((day): day is string => typeof day === 'string' && day !== '').sort();
  const first = days[0];
  const last = days.at(-1);
  if (first === undefined || last === undefined) return undefined;
  const length = Math.round((Date.parse(`${last}T00:00:00Z`) - Date.parse(`${first}T00:00:00Z`)) / DAY) + 1;
  const room = Math.max(1, Math.ceil(length / 20));
  const shown = length + 2 * room;
  const scale = FITS.find(([, holds]) => shown <= holds)?.[0] ?? 'year';
  return { scale, from: addDays(first, -room), to: addDays(last, room) };
}

const DAY = 86_400_000;
const addDays = (day: string, count: number) => new Date(Date.parse(`${day}T00:00:00Z`) + count * DAY).toISOString().slice(0, 10);

/** The entries the plan proposes to move that a planner may accept: inside the plan, not done. */
export function proposedMoves(schedule: PlanSchedule): string[] {
  return schedule.entries.filter((entry) => entry.moved && !entry.external && !entry.done).map((entry) => entry.item_id);
}

/** The bound a lag or a lead keeps (UC-PLN-02 check 1). */
export const MAX_LAG = 999;

/**
 * A lag typed into a field, in working days: a whole number between the bounds, a minus sign for a
 * lead. Anything else is `undefined`, which the field reports rather than sending.
 */
export function lagOf(text: string): number | undefined {
  const trimmed = text.trim();
  if (!/^[+-]?\d{1,3}$/.test(trimmed)) return undefined;
  const value = Number(trimmed);
  return Math.abs(value) <= MAX_LAG ? value : undefined;
}

/**
 * The link a dependency drawn from one bar onto another becomes: the waiting row depends on the
 * row the drag began on, which is the arrow's direction on the chart.
 */
export function dependencyDrawn(from: string, to: string): { sourceId: string; targetId: string; type: string } | undefined {
  if (from === to) return undefined;
  return { sourceId: to, targetId: from, type: 'depends_on' };
}

/**
 * The plan as a table to save (UC-PLN-09 check 6): one row per entry in the tree's order, its depth
 * as the outline level, its dates, duration, float and marks, and what it waits on in the
 * notation a project tool reads back (`3SS+2`), keyed by the row's number - so the file comes
 * back in through the import.
 */
export function planCsvRows(rows: readonly GanttRow[], schedule: PlanSchedule): (string | number)[][] {
  const number = new Map(rows.map((row, at) => [row.id, at + 1]));
  const waits = new Map<string, string[]>();
  for (const each of schedule.dependencies) {
    const from = number.get(each.predecessor_id);
    if (from === undefined) continue;
    const lag = each.lag_days === 0 ? '' : each.lag_days > 0 ? `+${each.lag_days}d` : `${each.lag_days}d`;
    const kind = each.dependency_kind === 'FS' && lag === '' ? '' : each.dependency_kind;
    waits.set(each.successor_id, [...(waits.get(each.successor_id) ?? []), `${from}${kind}${lag}`]);
  }
  return rows.map((row, at) => [
    at + 1,
    row.title,
    row.kind === 'milestone' ? '' : row.start,
    row.due,
    row.depth + 1,
    row.kind === 'milestone' ? 'yes' : '',
    (waits.get(row.id) ?? []).join(';'),
    row.entry.duration_days,
    row.entry.total_float_days,
    row.isCritical ? 'yes' : '',
  ]);
}

/** The columns `planCsvRows` fills, in its order; the first seven are the ones the import reads. */
export const PLAN_CSV_HEADS = ['ID', 'Name', 'Start', 'Finish', 'Outline Level', 'Milestone', 'Predecessors', 'Duration', 'Total Float', 'Critical'] as const;

/** The plan as a workbook with the CSV export's columns and rows (UC-PLN-09 check 6). */
export function planXlsxOf(rows: readonly GanttRow[], schedule: PlanSchedule): Uint8Array {
  return xlsxOf('Plan', PLAN_CSV_HEADS, planCsvRows(rows, schedule));
}

/**
 * A project file's dry run, or its import, in words (user-experience.md §7.11): what it creates by
 * the things a planner counts, and the rows hung beside their parent. `isDry` chooses the tense.
 */
export function projectPlanOf(
  answer: Pick<PlanImport, 'tasks' | 'work_packages' | 'activities' | 'milestones' | 'dependencies' | 'flattened'>,
  isDry: boolean,
  words: PlanWords,
): { readonly tone: 'add' | 'change'; readonly text: string }[] {
  const tense = isDry ? 'dry' : 'done';
  const counted = (
    [['tasks', answer.tasks], ['packages', answer.work_packages], ['activities', answer.activities], ['milestones', answer.milestones], ['dependencies', answer.dependencies]] as const
  ).filter(([, count]) => count > 0);
  const lines: { tone: 'add' | 'change'; text: string }[] = [];
  if (counted.length > 0) {
    const things = words.list(counted.map(([word, count]) => words.t(`app.import.count_${word}`, { count: String(count) })));
    lines.push({ tone: 'add', text: words.t(`app.import.plan_creates_${tense}`, { things }) });
  }
  if (answer.flattened > 0) lines.push({ tone: 'change', text: words.t(`app.planning.import_flattened_${tense}`, { count: String(answer.flattened) }) });
  return lines;
}

/** A project file's format from its name, else from its first character. */
export function projectFormatOf(fileName: string, content: string): 'MSPROJECT_XML' | 'CSV' {
  if (/\.xml$/iu.test(fileName)) return 'MSPROJECT_XML';
  if (/\.csv$/iu.test(fileName)) return 'CSV';
  return content.trimStart().startsWith('<') ? 'MSPROJECT_XML' : 'CSV';
}
