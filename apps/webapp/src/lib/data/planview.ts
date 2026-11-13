// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The plan's view (work-surface concept §6.3, E-16, E-20): which presentation *Zeitplan* draws -
 * the light timeline or the Gantt -, what is seen, how rows are grouped and ordered, what the bars
 * show, the baseline compared against and the outline columns. Saved as a view like every
 * collection's (`SavedView`, layout `TIMELINE`, the plan's part under `grouping.plan`, the columns
 * as `visible_fields`), so no second store. Pure, so each step is tested.
 */

export type PlanPresentation = 'timeline' | 'gantt';
export const PLAN_GROUPINGS = ['outline', 'collection', 'assignee', 'none'] as const;
export const PLAN_SORTS = ['start', 'finish', 'manual'] as const;
export const PLAN_COLOURS = ['status', 'collection', 'assignee'] as const;
export const PLAN_BARS = ['dependencies', 'critical', 'slack', 'progress', 'milestones', 'weekends'] as const;
export const PLAN_COLUMNS = ['start', 'finish', 'duration', 'assignee', 'float', 'slip'] as const;

export type PlanGrouping = (typeof PLAN_GROUPINGS)[number];
export type PlanSort = (typeof PLAN_SORTS)[number];
export type PlanColour = (typeof PLAN_COLOURS)[number];
export type PlanBar = (typeof PLAN_BARS)[number];

export interface PlanViewSetup {
  readonly presentation: PlanPresentation;
  /** What is seen, as the Hubtask query line (§6.11). */
  readonly line: string;
  readonly done: 'show' | 'hide';
  readonly groupBy: PlanGrouping;
  readonly sortBy: PlanSort;
  readonly colourBy: PlanColour;
  readonly bars: Readonly<Record<PlanBar, boolean>>;
  /** The baseline compared against; empty for none. */
  readonly baselineId: string;
  /** The outline columns left of the axis, in their order. */
  readonly columns: readonly string[];
}

/** The plan as it opens before anybody saved a view of it: the Gantt, everything drawn. */
export function planViewDefault(presentation: PlanPresentation = 'gantt'): PlanViewSetup {
  return {
    presentation,
    line: '',
    done: 'show',
    groupBy: 'outline',
    sortBy: 'manual',
    colourBy: 'status',
    bars: { dependencies: true, critical: true, slack: true, progress: true, milestones: true, weekends: true },
    baselineId: '',
    columns: ['start', 'finish', 'duration', 'assignee'],
  };
}

const oneOf = <T extends string>(value: unknown, allowed: readonly T[], fallback: T): T =>
  typeof value === 'string' && (allowed as readonly string[]).includes(value) ? (value as T) : fallback;

/** Whether a saved view is one of the plan's: it carries the plan's part. */
export function isPlanView(view: { readonly layout?: string; readonly grouping?: unknown }): boolean {
  const grouping = view.grouping as Record<string, unknown> | undefined;
  return view.layout === 'TIMELINE' && typeof grouping?.['plan'] === 'object' && grouping['plan'] !== null;
}

/** A saved view read back as the plan's setup; what it does not say stays as the default. */
export function planSetupOf(view: { readonly grouping?: unknown; readonly visible_fields?: readonly string[] }, line = ''): PlanViewSetup {
  const base = planViewDefault();
  const plan = ((view.grouping as Record<string, unknown> | undefined)?.['plan'] ?? {}) as Record<string, unknown>;
  const bars = (plan['bars'] ?? {}) as Record<string, unknown>;
  const columns = (view.visible_fields ?? []).filter((each) => (PLAN_COLUMNS as readonly string[]).includes(each));
  return {
    presentation: oneOf(plan['presentation'], ['timeline', 'gantt'] as const, base.presentation),
    line,
    done: oneOf(plan['done'], ['show', 'hide'] as const, base.done),
    groupBy: oneOf(plan['group_by'], PLAN_GROUPINGS, base.groupBy),
    sortBy: oneOf(plan['sort_by'], PLAN_SORTS, base.sortBy),
    colourBy: oneOf(plan['colour_by'], PLAN_COLOURS, base.colourBy),
    bars: Object.fromEntries(PLAN_BARS.map((bar) => [bar, typeof bars[bar] === 'boolean' ? bars[bar] : base.bars[bar]])) as Record<PlanBar, boolean>,
    baselineId: typeof plan['baseline_id'] === 'string' ? plan['baseline_id'] : '',
    columns: view.visible_fields ? columns : base.columns,
  };
}

/** The grouping a plan view stores (`SavedView.grouping`): the plan's part, whole. */
export function planGroupingOf(setup: PlanViewSetup): Record<string, unknown> {
  return {
    plan: {
      presentation: setup.presentation,
      done: setup.done,
      group_by: setup.groupBy,
      sort_by: setup.sortBy,
      colour_by: setup.colourBy,
      bars: { ...setup.bars },
      ...(setup.baselineId ? { baseline_id: setup.baselineId } : {}),
    },
  };
}

/** How many parts differ from the view the plan opened with: *Ansicht · 2*. */
export function planChanges(now: PlanViewSetup, base: PlanViewSetup): number {
  const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
  const parts: [unknown, unknown][] = [
    [now.presentation, base.presentation],
    [now.line.trim(), base.line.trim()],
    [now.done, base.done],
    [now.groupBy, base.groupBy],
    [now.sortBy, base.sortBy],
    [now.colourBy, base.colourBy],
    [now.bars, base.bars],
    [now.baselineId, base.baselineId],
    [now.columns, base.columns],
  ];
  return parts.filter(([a, b]) => !same(a, b)).length;
}

/** One piece of the head sentence: a message code with its parameters, and whether it warns. */
export interface HeadPart {
  readonly code: string;
  readonly params: Readonly<Record<string, string>>;
  readonly isWarning?: boolean;
}

/**
 * The head sentence of `plan` and `gantt` (§6.3): when the plan ends, how much is on the critical
 * path, the slip against the baseline compared, the tolerance kept, what has no dates. Each part
 * only where it has something to say.
 */
export function headParts(facts: {
  readonly finish?: string | null;
  readonly critical: number;
  readonly slipDays?: number | null;
  readonly baselineName?: string;
  readonly toleranceDays?: number | null;
  readonly undated: number;
}): readonly HeadPart[] {
  const parts: HeadPart[] = [];
  parts.push(facts.finish ? { code: 'app.planning.figure_ends', params: { date: facts.finish } } : { code: 'app.planning.no_dates', params: {} });
  if (facts.critical > 0) parts.push({ code: 'app.planning.figure_critical', params: { count: String(facts.critical) } });
  if (facts.baselineName && typeof facts.slipDays === 'number' && facts.slipDays > 0) {
    parts.push({ code: 'app.planning.figure_slip', params: { days: String(facts.slipDays), name: facts.baselineName }, isWarning: true });
  }
  if (typeof facts.toleranceDays === 'number') parts.push({ code: 'app.planning.figure_tolerance', params: { days: String(facts.toleranceDays) } });
  if (facts.undated > 0) parts.push({ code: 'app.planning.figure_undated', params: { count: String(facts.undated) } });
  return parts;
}

/** The rows a line and the done switch keep: those found, and every summary above one of them. */
export function keptRows<T extends { readonly id: string; readonly parentId?: string; readonly isDone?: boolean }>(
  rows: readonly T[],
  found: ReadonlySet<string> | undefined,
  done: 'show' | 'hide',
): readonly T[] {
  const byId = new Map(rows.map((row) => [row.id, row]));
  const keep = new Set<string>();
  for (const row of rows) {
    if (done === 'hide' && row.isDone) continue;
    if (found && !found.has(row.id)) continue;
    keep.add(row.id);
    let parent = row.parentId ? byId.get(row.parentId) : undefined;
    while (parent && !keep.has(parent.id)) {
      keep.add(parent.id);
      parent = parent.parentId ? byId.get(parent.parentId) : undefined;
    }
  }
  return rows.filter((row) => keep.has(row.id));
}

/** The last day an entry may end without moving the plan's end: its finish and its slack in working days (Monday to Friday). */
export function slackEnd(finish: string | undefined, floatDays: number): string | undefined {
  if (!finish || floatDays <= 0) return undefined;
  let at = Date.parse(`${finish}T00:00:00Z`);
  if (Number.isNaN(at)) return undefined;
  for (let left = floatDays; left > 0;) {
    at += 86_400_000;
    const day = new Date(at).getUTCDay();
    if (day !== 0 && day !== 6) left -= 1;
  }
  return new Date(at).toISOString().slice(0, 10);
}

/** A colour of the categorical palette (1-7) per value, in the order the values first come. */
export function tonesOf(values: readonly (string | undefined)[]): ReadonlyMap<string, number> {
  const tones = new Map<string, number>();
  for (const value of values) if (value && !tones.has(value)) tones.set(value, (tones.size % 7) + 1);
  return tones;
}
