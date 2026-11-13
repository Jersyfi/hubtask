// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The goal screens' arithmetic (UC-GOL-01 … 04): a period's goals as an alignment tree, a share
 * as a percentage, a goal or a dashboard turned back into what its write takes, a widget's figures
 * as rows a chart and its table both draw, the chart's shapes, and the dashboard's grid.
 *
 * Pure, so that "which goal hangs where" and "which bar is how tall" are checked by `node --test`.
 * The server computes every number - progress, a view's total, what the reader may count (P-05,
 * P-11); this module only lays out what it answered, and never stores a figure.
 */

import type { DashboardWidget, DashboardWidgetWrite, Goal, GoalWrite, KeyResult, KeyResultWrite, Portfolio, PortfolioWrite } from '@hubtask/sync-engine';

/** A share from 0 to 1 as a whole percentage, rounded down so that 100 means everything; '' for none. */
export function percentText(share: number | null | undefined): string {
  if (share === null || share === undefined || !Number.isFinite(share)) return '';
  return String(Math.floor(Math.min(Math.max(share, 0), 1) * 100 + 1e-9));
}

/** The calendar quarter a day falls in, with its first and last day: the period offered first (§4). */
export function currentQuarter(day: string): { year: number; quarter: number; start_on: string; end_on: string } {
  const [year = 1970, month = 1] = day.split('-').map(Number);
  const quarter = Math.floor((month - 1) / 3) + 1;
  const first = (quarter - 1) * 3 + 1;
  const last = new Date(Date.UTC(year, first + 2, 0)).getUTCDate();
  const pad = (n: number) => String(n).padStart(2, '0');
  return { year, quarter, start_on: `${year}-${pad(first)}-01`, end_on: `${year}-${pad(first + 2)}-${pad(last)}` };
}

/** How many goals are at risk or off track: the head's figure of what needs a look. */
export function needingALook(goals: readonly Pick<Goal, 'status'>[]): number {
  return goals.filter((goal) => goal.status === 'AT_RISK' || goal.status === 'OFF_TRACK').length;
}

/** A share as the 0-100 a progress bar takes; undefined for none, which the bar draws as unknown. */
export function percentOf(share: number | null | undefined): number | undefined {
  const text = percentText(share);
  return text === '' ? undefined : Number(text);
}

/**
 * The hub a goal or a dashboard is in; null for the workspace's. The nil UUID names no hub - an
 * answer kept from an older server carries it for the workspace - so it reads as the workspace.
 */
export function hubOf(place: { hub_id?: string | null } | null | undefined): string | null {
  const id = place?.hub_id;
  return id && id !== '00000000-0000-0000-0000-000000000000' ? id : null;
}

// --- The alignment tree (UC-GOL-01 check 9) -----------------------------------------------------

export interface TreeRow {
  readonly goal: Goal;
  readonly depth: number;
}

/**
 * The goals as a tree in reading order: each goal under its parent, a goal whose parent the reader
 * does not see (another period's, a hub they do not read) as a root. Siblings keep the server's
 * order. A cycle cannot come from the server (`goals.parent_cycle`), and is cut here all the same.
 */
export function alignmentTree(goals: readonly Goal[]): TreeRow[] {
  const ids = new Set(goals.map((goal) => goal.id));
  const children = new Map<string, Goal[]>();
  const roots: Goal[] = [];
  for (const goal of goals) {
    if (goal.parent_id && ids.has(goal.parent_id) && goal.parent_id !== goal.id) {
      const list = children.get(goal.parent_id) ?? [];
      list.push(goal);
      children.set(goal.parent_id, list);
    } else {
      roots.push(goal);
    }
  }
  const rows: TreeRow[] = [];
  const seen = new Set<string>();
  const walk = (goal: Goal, depth: number) => {
    if (seen.has(goal.id)) return;
    seen.add(goal.id);
    rows.push({ goal, depth });
    for (const child of children.get(goal.id) ?? []) walk(child, depth + 1);
  };
  for (const root of roots) walk(root, 0);
  // What a cycle left unreached is still shown, at the top.
  for (const goal of goals) walk(goal, 0);
  return rows;
}

/** The goals a goal may be hung under: none of its own subtree, nor itself. */
export function parentChoices(goals: readonly Goal[], goalId: string | undefined): Goal[] {
  if (!goalId) return [...goals];
  const below = new Set<string>([goalId]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const goal of goals) {
      if (goal.parent_id && below.has(goal.parent_id) && !below.has(goal.id)) {
        below.add(goal.id);
        grew = true;
      }
    }
  }
  return goals.filter((goal) => !below.has(goal.id));
}

// --- Writes from reads --------------------------------------------------------------------------

/** A key result as its write takes it: by its id, so that it keeps its values. */
export function keyResultWrite(result: KeyResult): KeyResultWrite {
  const write: KeyResultWrite = {
    id: result.id,
    title: result.title,
    kind: result.kind,
    start: result.start,
    target: result.target,
    current: result.current,
    unit: result.unit,
    source: result.source,
  };
  if (result.view_id) write.view_id = result.view_id;
  if (result.entry_id) write.entry_id = result.entry_id;
  return write;
}

/** A goal as `PUT /goals/{id}` takes it, with what the caller changes laid over it. */
export function goalWrite(goal: Goal, changes: Partial<GoalWrite> = {}): GoalWrite {
  const write: GoalWrite = {
    period_id: goal.period_id,
    title: goal.title,
    description: goal.description,
    status: goal.status,
    key_results: goal.key_results.map(keyResultWrite),
  };
  if (goal.owner_id) write.owner_id = goal.owner_id;
  if (goal.parent_id) write.parent_id = goal.parent_id;
  const merged = { ...write, ...changes };
  // An emptied parent or owner is left out, which the write reads as none.
  if (!merged.parent_id) delete merged.parent_id;
  if (!merged.owner_id) delete merged.owner_id;
  return merged;
}

/**
 * A portfolio as `PUT /portfolios/{id}` takes it. The projects are the ones the reader sees; the
 * server keeps the ones they do not (UC-GOL-03 check 3).
 */
export function portfolioWrite(portfolio: Portfolio, changes: Partial<PortfolioWrite> = {}): PortfolioWrite {
  const write: PortfolioWrite = {
    name: portfolio.name,
    description: portfolio.description,
    status: portfolio.status,
    items: portfolio.items.map((item) => {
      const row: NonNullable<PortfolioWrite['items']>[number] = { container_id: item.container_id, status: item.status };
      if (item.start_on) row.start_on = item.start_on;
      if (item.end_on) row.end_on = item.end_on;
      return row;
    }),
  };
  if (portfolio.owner_id) write.owner_id = portfolio.owner_id;
  if (portfolio.parent_id) write.parent_id = portfolio.parent_id;
  const merged = { ...write, ...changes };
  if (!merged.parent_id) delete merged.parent_id;
  if (!merged.owner_id) delete merged.owner_id;
  return merged;
}

/** A widget as `PUT /dashboards/{id}` takes it: by its id, without the nulls the read carries. */
/**
 * At most `keep` rows, the rest summed as one row named `rest` (wcat: bars keep eight, a ring four),
 * so a chart never draws more parts than the palette and the eye can tell apart.
 */
export function folded(rows: readonly FigureRow[], keep: number, rest: string): FigureRow[] {
  if (rows.length <= keep) return [...rows];
  const sorted = [...rows].sort((a, b) => b.value - a.value);
  const kept = sorted.slice(0, keep - 1);
  const value = sorted.slice(keep - 1).reduce((sum, row) => sum + row.value, 0);
  return [...kept, { key: '__rest', label: rest, value, text: String(value) }];
}

export function widgetWrite(widget: DashboardWidget): DashboardWidgetWrite {
  const write: DashboardWidgetWrite = {
    id: widget.id,
    title: widget.title,
    chart: widget.chart,
    source: widget.source,
    x: widget.x,
    y: widget.y,
    w: widget.w,
    h: widget.h,
  };
  if (widget.source === 'VIEW' && widget.view_id) write.view_id = widget.view_id;
  if (widget.source === 'QUERY' && widget.query) write.query = widget.query;
  if (widget.source === 'GOALS') write.goal_ids = [...widget.goal_ids];
  // The replica hands the column on as stored, without the keys a widget does not use.
  if (widget.source !== 'GOALS' && widget.compare) write.compare = { ...widget.compare };
  if (widget.source !== 'GOALS' && widget.cumulative) write.cumulative = true;
  if (widget.source === 'TEXT') write.text = widget.text ?? '';
  if (widget.chart === 'LIST' && widget.column) write.column = widget.column;
  if (widget.source === 'QUERY' && widget.released?.length) write.released = [...widget.released];
  if ((widget.chart === 'NUMBER' || widget.chart === 'LINE') && typeof widget.target === 'number') {
    write.target = widget.target;
    if (typeof widget.warn_at === 'number') write.warn_at = widget.warn_at;
  }
  return write;
}

// --- A widget's figures as rows (UC-GOL-04 check 8) ---------------------------------------------

/** One row a chart draws and its table lists. */
export interface FigureRow {
  readonly key: string;
  /** The group's value, the period's first day, or a goal's title; '' where the server had none. */
  readonly label: string;
  /** A second dimension, for a matrix: the period where both a group and a period are named. */
  readonly column?: string;
  /** What is drawn: the function's value, else the count, else a goal's progress in percent. */
  readonly value: number;
  /** The value as the server wrote it, so that money keeps its digits in the table. */
  readonly text: string;
  readonly count?: number;
  /** The server's running total up to this period, for a cumulative widget. */
  readonly running?: number;
  readonly runningText?: string;
  readonly goalId?: string;
  readonly status?: string;
  /** The group and the period as the server wrote them, which is the order they sort in. */
  readonly order?: string;
  readonly period?: string;
}

/**
 * A row's group and period in words (user-experience.md §5.6, §5.10): a type's name for its key,
 * a person's for their id, the reader's date for a period's first day - never a key or ISO text.
 */
export interface FigureNaming {
  readonly group: (value: string) => string;
  readonly period: (day: string) => string;
}
const asIs: FigureNaming = { group: (value) => value, period: (day) => day };

const asText = (value: unknown): string | undefined => (typeof value === 'string' ? value : undefined);

/**
 * A widget's rows as `GET /dashboards/{id}/figures` answered them, in one shape: a summary's
 * (count, group, period, value) or a GOALS widget's (goal_id, title, status, progress).
 */
export function figureRows(data: readonly Record<string, unknown>[], naming: FigureNaming = asIs): FigureRow[] {
  return data.map((row, i) => {
    const goalId = asText(row['goal_id']);
    if (goalId) {
      const progress = typeof row['progress'] === 'number' ? (row['progress'] as number) : null;
      const percent = percentText(progress);
      return {
        key: goalId,
        label: asText(row['title']) ?? '',
        value: percent === '' ? 0 : Number(percent),
        text: percent,
        goalId,
        ...(asText(row['status']) ? { status: asText(row['status']) } : {}),
      };
    }
    const group = asText(row['group']);
    const period = asText(row['period']);
    const count = typeof row['count'] === 'number' ? (row['count'] as number) : 0;
    const raw = asText(row['value']);
    const parsed = raw === undefined ? Number.NaN : Number(raw);
    const value = Number.isFinite(parsed) ? parsed : count;
    const running = asText(row['running']);
    return {
      key: `${group ?? ''}|${period ?? ''}|${i}`,
      ...namedPlace(group, period, naming),
      value,
      text: raw ?? String(count),
      count,
      ...(running !== undefined && Number.isFinite(Number(running)) ? { running: Number(running), runningText: running } : {}),
    };
  });
}

/** A row's label and column in words, with the server's values kept to sort by. */
function namedPlace(group: string | undefined, period: string | undefined, naming: FigureNaming) {
  const groupName = group === undefined || group === '' ? group : naming.group(group);
  const periodName = period === undefined || period === '' ? period : naming.period(period);
  const both = group !== undefined && period !== undefined;
  return {
    label: both ? groupName! : (groupName ?? periodName ?? ''),
    ...(both ? { column: periodName } : {}),
    order: `${group ?? ''}\u0000${period ?? ''}`,
    ...(period !== undefined ? { period } : {}),
  };
}

/** Rows as a cumulative widget draws them: each period's running total in place of its own value. */
export function runningRows(rows: readonly FigureRow[]): FigureRow[] {
  return rows.map((row) => (row.running === undefined ? row : { ...row, value: row.running, text: row.runningText ?? String(row.running) }));
}

/** `remaining` as `GET /dashboards/{id}/figures` answers it, as rows. */
export function leftRows(data: readonly Record<string, unknown>[] | null | undefined, naming: FigureNaming = asIs): FigureRow[] {
  return (data ?? []).map((row, i) => {
    const group = asText(row['group']);
    const period = asText(row['period']);
    const text = asText(row['value']) ?? '';
    return {
      key: `left|${group ?? ''}|${period ?? ''}|${i}`,
      ...namedPlace(group, period, naming),
      value: Number.isFinite(Number(text)) ? Number(text) : 0,
      text,
    };
  });
}

/** The fields whose groups are people, named by their account. */
const PERSON_FIELDS: ReadonlySet<unknown> = new Set(['assignee_id', 'created_by']);

/**
 * The accounts a dashboard's groups name, once each: the groups of the widgets grouped by a person
 * only - one value that is no account (a "true", a status key) would refuse the whole batch read.
 */
export function groupedPeople(
  figures: readonly { id: string; data: readonly Record<string, unknown>[]; compare?: { data?: readonly Record<string, unknown>[] | null } | null }[],
  fieldOf: (widgetId: string) => unknown,
): string[] {
  const ids = new Set<string>();
  for (const figure of figures) {
    if (!PERSON_FIELDS.has(fieldOf(figure.id))) continue;
    for (const row of [...figure.data, ...(figure.compare?.data ?? [])]) {
      const group = row['group'];
      if (typeof group === 'string' && group !== '') ids.add(group);
    }
  }
  return [...ids];
}

/** Several series of one widget side by side: one line per label, one value per series. */
export interface SeriesTable {
  readonly labels: readonly string[];
  readonly series: readonly { readonly name: string; readonly values: readonly (number | undefined)[]; readonly texts: readonly string[] }[];
}

/**
 * Series aligned by their labels - each row's label and column, sorted as periods sort. A
 * cumulative series carries its last running total over a label it holds no row for, which is
 * what the server's running total is there; any other leaves the cell empty.
 */
export function seriesTable(series: readonly { name: string; rows: readonly FigureRow[]; isRunning?: boolean }[]): SeriesTable {
  const nameOf = (row: FigureRow) => (row.column === undefined ? row.label : `${row.label} ${row.column}`);
  const orderOf = new Map<string, string>();
  for (const row of series.flatMap((each) => each.rows)) if (!orderOf.has(nameOf(row))) orderOf.set(nameOf(row), row.order ?? nameOf(row));
  const labels = [...orderOf.keys()].sort((a, b) => compare(orderOf.get(a)!, orderOf.get(b)!));
  return {
    labels,
    series: series.map((each) => {
      const at = new Map(each.rows.map((row) => [nameOf(row), row] as const));
      let last: FigureRow | undefined;
      const cells = labels.map((label) => {
        const row = at.get(label);
        if (row) last = row;
        return row ?? (each.isRunning ? last : undefined);
      });
      return { name: each.name, values: cells.map((row) => row?.value), texts: cells.map((row) => row?.text ?? '') };
    }),
  };
}

/** One SVG `points` attribute per series on a shared scale; a missing value breaks nothing, it is skipped. */
export function seriesLines(table: SeriesTable, width: number, height: number): string[] {
  const top = Math.max(0, ...table.series.flatMap((each) => each.values.filter((v): v is number => v !== undefined)));
  const n = table.labels.length;
  const step = n > 1 ? width / (n - 1) : 0;
  return table.series.map((each) => each.values
    .map((v, i) => (v === undefined ? undefined : `${round(n > 1 ? i * step : width / 2)},${round(top > 0 ? height - (Math.max(v, 0) / top) * height : height)}`))
    .filter((point) => point !== undefined)
    .join(' '));
}

/** Bars of several series side by side per label, on a shared scale; series n takes colour n. */
export function seriesBars(table: SeriesTable, width: number, height: number): Bar[] {
  const n = table.labels.length;
  if (n === 0 || table.series.length === 0) return [];
  const top = Math.max(0, ...table.series.flatMap((each) => each.values.filter((v): v is number => v !== undefined)));
  const slot = width / n;
  const gap = slot * 0.2;
  const bar = (slot - gap) / table.series.length;
  return table.series.flatMap((each, s) => each.values.map((v, i) => {
    const h = top > 0 && v !== undefined ? (Math.max(v, 0) / top) * height : 0;
    return { key: `${s}|${i}`, x: i * slot + gap / 2 + s * bar, y: height - h, width: bar, height: h, series: seriesOf(s) };
  }));
}

/** The total a NUMBER widget shows: the sum of its rows' values. */
export function totalOf(rows: readonly FigureRow[]): number {
  return rows.reduce((sum, row) => sum + row.value, 0);
}

/**
 * The chart palette (E-7, WB-2): the seven `chart.categorical` tokens in their fixed order - never
 * a product's or a label's colour. A chart beyond seven parts groups the rest (`folded`).
 */
export const SERIES = ['1', '2', '3', '4', '5', '6', '7'] as const;
export type Series = (typeof SERIES)[number];

/** The colour token of the n-th series, cycling. */
export function seriesOf(index: number): Series {
  return SERIES[((index % SERIES.length) + SERIES.length) % SERIES.length] as Series;
}

export interface Bar {
  readonly key: string;
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
  readonly series: Series;
}

/** Bars on a `width` × `height` canvas from zero up; a negative value is drawn as zero. */
export function barLayout(rows: readonly FigureRow[], width: number, height: number): Bar[] {
  if (rows.length === 0) return [];
  const top = Math.max(...rows.map((row) => row.value), 0);
  const slot = width / rows.length;
  const gap = slot * 0.2;
  return rows.map((row, i) => {
    const h = top > 0 ? (Math.max(row.value, 0) / top) * height : 0;
    return { key: row.key, x: i * slot + gap / 2, y: height - h, width: slot - gap, height: h, series: seriesOf(0) };
  });
}

/** A line's points on a `width` × `height` canvas, as an SVG `points` attribute. */
export function linePoints(rows: readonly FigureRow[], width: number, height: number): string {
  if (rows.length === 0) return '';
  const top = Math.max(...rows.map((row) => row.value), 0);
  const step = rows.length > 1 ? width / (rows.length - 1) : 0;
  return rows
    .map((row, i) => {
      const x = rows.length > 1 ? i * step : width / 2;
      const y = top > 0 ? height - (Math.max(row.value, 0) / top) * height : height;
      return `${round(x)},${round(y)}`;
    })
    .join(' ');
}

export interface Slice {
  readonly key: string;
  /** An SVG path on a circle of radius 1 around the origin. */
  readonly path: string;
  readonly share: number;
  readonly series: Series;
}

/** A pie's slices on the unit circle, starting at twelve o'clock; rows of zero or less are left out. */
export function pieSlices(rows: readonly FigureRow[]): Slice[] {
  const kept = rows.map((row, i) => ({ row, series: seriesOf(i) })).filter(({ row }) => row.value > 0);
  const total = kept.reduce((sum, { row }) => sum + row.value, 0);
  if (total <= 0) return [];
  if (kept.length === 1) {
    const only = kept[0]!;
    return [{ key: only.row.key, path: 'M 0 -1 A 1 1 0 1 1 0 1 A 1 1 0 1 1 0 -1 Z', share: 1, series: only.series }];
  }
  let angle = -Math.PI / 2;
  return kept.map(({ row, series }) => {
    const share = row.value / total;
    const from = angle;
    angle += share * 2 * Math.PI;
    const large = share > 0.5 ? 1 : 0;
    const path = `M 0 0 L ${round(Math.cos(from))} ${round(Math.sin(from))} A 1 1 0 ${large} 1 ${round(Math.cos(angle))} ${round(Math.sin(angle))} Z`;
    return { key: row.key, path, share, series };
  });
}

export interface Matrix {
  readonly rows: readonly string[];
  readonly columns: readonly string[];
  /** `cells[r][c]`: the value's text, '' where the pair holds nothing. */
  readonly cells: readonly (readonly string[])[];
}

/** Rows grouped by a value and bucketed by a period, as a two-dimensional table. */
export function matrixOf(rows: readonly FigureRow[]): Matrix {
  const rowNames: string[] = [];
  const columnNames: string[] = [];
  const at = new Map<string, string>();
  const periodOf = new Map<string, string>();
  for (const row of rows) {
    const column = row.column ?? '';
    if (!rowNames.includes(row.label)) rowNames.push(row.label);
    if (!columnNames.includes(column)) columnNames.push(column);
    if (!periodOf.has(column)) periodOf.set(column, row.period ?? column);
    at.set(`${row.label}\u0000${column}`, row.text);
  }
  columnNames.sort((a, b) => compare(periodOf.get(a)!, periodOf.get(b)!));
  return {
    rows: rowNames,
    columns: columnNames,
    cells: rowNames.map((r) => columnNames.map((c) => at.get(`${r}\u0000${c}`) ?? '')),
  };
}

/** Code-point order, as the server's group and period values sort. */
function compare(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

/**
 * A dot at each point of a line, as an SVG path: a line of one period is a dot, never nothing.
 * Each dot is a segment of no length, drawn with a round cap.
 */
export function lineDots(points: string): string {
  return points.split(' ').filter(Boolean).map((point) => `M${point}h0`).join('');
}

/** The highest value a chart's scale reaches, zero for none: the top of its value axis. */
export function scaleTop(values: readonly (number | undefined)[]): number {
  return Math.max(0, ...values.filter((v): v is number => v !== undefined && Number.isFinite(v)));
}

// --- The nine kinds' shapes (wcat) -------------------------------------------------------------

/**
 * A NUMBER widget's figure: over periods the last period with its change against the one before
 * and the last eight periods as a line; without periods the sum of its rows.
 */
export interface NumberFigure {
  readonly value: number;
  readonly text: string;
  /** Last period minus the one before, where there are two. */
  readonly change?: number;
  /** The last eight periods as SVG points on a 100 × 24 canvas; '' for fewer than two. */
  readonly spark: string;
}

export function numberFigure(rows: readonly FigureRow[]): NumberFigure {
  // Entries without the stamp (one not resolved yet) belong to no period, and so to no figure of one.
  const periods = rows.filter((row) => row.period !== undefined && row.column === undefined);
  if (periods.length === 0 || rows.some((row) => row.column !== undefined)) {
    const total = totalOf(rows);
    const texts = rows.length === 1 ? rows[0]!.text : String(round(total));
    return { value: total, text: texts, spark: '' };
  }
  const ordered = [...periods].sort((a, b) => compare(a.period!, b.period!));
  const last = ordered[ordered.length - 1]!;
  const before = ordered[ordered.length - 2];
  const recent = ordered.slice(-8);
  return {
    value: last.value,
    text: last.text,
    ...(before ? { change: round(last.value - before.value) } : {}),
    spark: recent.length > 1 ? linePoints(recent, 100, 24) : '',
  };
}

/**
 * Where a figure stands against its target and warning line (wcat "Ziel ≤ 30"): a warning line
 * above the target means lower is better, below it higher is. `met` reaches the target, `warn`
 * crosses the warning line, `between` is neither. Never drawn by colour alone: a word goes with it.
 */
export type TargetState = 'met' | 'between' | 'warn';
export function targetState(value: number, target: number | null | undefined, warnAt: number | null | undefined): TargetState | undefined {
  if (target === null || target === undefined) return undefined;
  const lowerIsBetter = warnAt !== null && warnAt !== undefined ? warnAt >= target : false;
  if (lowerIsBetter ? value <= target : value >= target) return 'met';
  if (warnAt !== null && warnAt !== undefined && (lowerIsBetter ? value >= warnAt : value <= warnAt)) return 'warn';
  return 'between';
}

/** A horizontal bar: its row and its length in percent of the longest. */
export interface HBar {
  readonly row: FigureRow;
  readonly share: number;
}

/** Bars lying down, the longest full (wcat): at most eight, the rest as one row named `rest`. */
export function hbars(rows: readonly FigureRow[], rest: string): HBar[] {
  const kept = folded(rows, 8, rest);
  const top = scaleTop(kept.map((row) => row.value));
  return kept.map((row) => ({ row, share: top > 0 ? round((Math.max(row.value, 0) / top) * 100) : 0 }));
}

/** Two series as pairs of bars lying down: each label's parts in percent of the largest pair's sum. */
export interface HPair {
  readonly label: string;
  readonly shares: readonly number[];
  readonly texts: readonly string[];
}
export function hpairs(table: SeriesTable): HPair[] {
  const sums = table.labels.map((_, i) => table.series.reduce((sum, each) => sum + Math.max(each.values[i] ?? 0, 0), 0));
  const top = Math.max(0, ...sums);
  return table.labels.slice(0, 8).map((label, i) => ({
    label,
    shares: table.series.map((each) => (top > 0 ? round((Math.max(each.values[i] ?? 0, 0) / top) * 100) : 0)),
    texts: table.series.map((each) => each.texts[i] ?? ''),
  }));
}

/** A ring's part: its row, its share of the whole and where it starts, both as fractions of one turn. */
export interface RingPart {
  readonly row: FigureRow;
  readonly share: number;
  readonly start: number;
  readonly series: Series;
}

/** A ring of at most four parts (wcat), the rest as one named `rest`; parts of zero or less are left out. */
export function ringParts(rows: readonly FigureRow[], rest: string): RingPart[] {
  const kept = folded(rows.filter((row) => row.value > 0), 4, rest);
  const total = kept.reduce((sum, row) => sum + row.value, 0);
  if (total <= 0) return [];
  let start = 0;
  return kept.map((row, i) => {
    const share = round(row.value / total);
    const part = { row, share, start: round(start), series: seriesOf(i) };
    start += row.value / total;
    return part;
  });
}

/** A matrix cell's step from 1 (least) to 5 (most), 0 where it holds nothing, over the whole matrix. */
export function matrixSteps(matrix: Matrix): number[][] {
  const values = matrix.cells.flat().map(Number).filter((v) => Number.isFinite(v));
  const top = Math.max(0, ...values);
  return matrix.cells.map((row) => row.map((text) => {
    const v = Number(text);
    if (text === '' || !Number.isFinite(v)) return 0;
    return top > 0 ? Math.max(1, Math.ceil((Math.max(v, 0) / top) * 5)) : 1;
  }));
}

/** A piece of a TEXT widget: plain, bold, or a link to a path inside the app. */
export type TextPiece = { readonly kind: 'plain' | 'bold'; readonly text: string } | { readonly kind: 'link'; readonly text: string; readonly href: string };

/**
 * A TEXT widget's words in pieces - `**bold**` and `[words](/path)` - drawn as elements, never as
 * HTML. A link goes only to a path of the app; any other target stays its words.
 */
export function textPieces(text: string): TextPiece[] {
  const pieces: TextPiece[] = [];
  const pattern = /\*\*([^*]+)\*\*|\[([^\]]+)\]\(([^)\s]+)\)/gu;
  let at = 0;
  for (const match of text.matchAll(pattern)) {
    if (match.index > at) pieces.push({ kind: 'plain', text: text.slice(at, match.index) });
    if (match[1] !== undefined) pieces.push({ kind: 'bold', text: match[1] });
    else if (/^\/(?!\/)/u.test(match[3]!)) pieces.push({ kind: 'link', text: match[2]!, href: match[3]! });
    else pieces.push({ kind: 'plain', text: match[2]! });
    at = match.index + match[0].length;
  }
  if (at < text.length) pieces.push({ kind: 'plain', text: text.slice(at) });
  return pieces;
}

function round(value: number): number {
  return Math.round(value * 1000) / 1000;
}

// --- The grid (UC-GOL-04 check 8) ---------------------------------------------------------------

export const COLUMNS = 12;
export const MAX_WIDGETS = 24;

export interface Box {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

/** A box held to the twelve-column grid the server holds it to: inside the columns, at least 1 × 1. */
export function clampBox(box: Box): Box {
  const w = Math.min(Math.max(Math.round(box.w), 1), COLUMNS);
  const h = Math.min(Math.max(Math.round(box.h), 1), 12);
  const x = Math.min(Math.max(Math.round(box.x), 0), COLUMNS - w);
  const y = Math.max(Math.round(box.y), 0);
  return { x, y, w, h };
}

/** The box moved by whole cells, kept on the grid. */
export function moved(box: Box, dx: number, dy: number): Box {
  return clampBox({ ...box, x: box.x + dx, y: box.y + dy });
}

/** The box grown or shrunk by whole cells, kept on the grid. */
export function resized(box: Box, dw: number, dh: number): Box {
  return clampBox({ ...box, w: box.w + dw, h: box.h + dh });
}

/** Where a new widget of `w` × `h` goes: the first row under every widget there is. */
export function nextBox(widgets: readonly Box[], w = 6, h = 4): Box {
  const bottom = widgets.reduce((most, box) => Math.max(most, box.y + box.h), 0);
  return clampBox({ x: 0, y: bottom, w, h });
}

/** Widgets in reading order - row by row, left to right - which is also the order a phone stacks them. */
export function readingOrder<T extends Box>(widgets: readonly T[]): T[] {
  return [...widgets].sort((a, b) => a.y - b.y || a.x - b.x);
}

// --- A table as a file --------------------------------------------------------------------------

/**
 * Rows as RFC 4180 CSV under the given heads: a field quoted where it holds a comma, a quote or a
 * line break, and a text a spreadsheet would read as a formula prefixed with a quote mark, so that
 * a group's name never runs.
 */
export function csvOf(heads: readonly string[], rows: readonly (readonly (string | number)[])[]): string {
  const cell = (value: string | number): string => {
    let text = String(value);
    if (typeof value === 'string' && /^[=+\-@\t\r]/u.test(text)) text = `'${text}`;
    return /[",\r\n]/u.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
  };
  return `${[heads, ...rows].map((row) => row.map(cell).join(',')).join('\r\n')}\r\n`;
}

// --- The dashboard's filter as a widget inherits it (§6.5) --------------------------------------

type Node = Record<string, unknown>;
const isLeaf = (node: Node): boolean => typeof node['field'] === 'string';

/** The filter's top-level conditions, one per chip: a group ORed together stays one condition. */
export function conditionsOf(filter: Node | undefined): Node[] {
  if (!filter) return [];
  if (filter['op'] === 'AND' && Array.isArray(filter['nodes'])) return filter['nodes'] as Node[];
  return [filter];
}

/** What a widget takes of the dashboard's filter: its conditions but those on released fields - the server's rule. */
export function inheritedFilter(filter: Node | undefined, released: readonly string[] = []): Node | undefined {
  const kept = conditionsOf(filter).filter((node) => !(isLeaf(node) && released.includes(String(node['field']))));
  if (kept.length === 0) return undefined;
  return kept.length === 1 ? kept[0] : { op: 'AND', nodes: kept };
}

/**
 * The tree *Als Liste öffnen* hands to a collection's list: what the dashboard passes on AND the
 * widget's own filter, without a hub condition the collection already answers (its own hub).
 */
export function listFilter(filter: Node | undefined, released: readonly string[], own: Node | undefined, hubId: string | null | undefined): Node | undefined {
  const shared = conditionsOf(inheritedFilter(filter, released)).filter((node) => !(node['field'] === 'hub_id' && node['op'] === 'EQ' && node['value'] === hubId));
  const nodes = [...shared, ...conditionsOf(own)];
  if (nodes.length === 0) return undefined;
  return nodes.length === 1 ? nodes[0] : { op: 'AND', nodes };
}

/**
 * The tree *Als Liste öffnen* hands to the search for a widget counting a hub or the whole
 * workspace: the hub it counts in, once, AND what the dashboard passes on AND the widget's own.
 */
export function searchFilter(filter: Node | undefined, released: readonly string[], own: Node | undefined, hubId: string | undefined): Node | undefined {
  const rest = conditionsOf(listFilter(filter, released, own, hubId));
  const nodes = hubId ? [{ field: 'hub_id', op: 'EQ', value: hubId }, ...rest] : rest;
  if (nodes.length === 0) return undefined;
  return nodes.length === 1 ? nodes[0] : { op: 'AND', nodes };
}
