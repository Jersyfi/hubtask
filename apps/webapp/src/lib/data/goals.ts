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

/** A share as the 0-100 a progress bar takes; undefined for none, which the bar draws as unknown. */
export function percentOf(share: number | null | undefined): number | undefined {
  const text = percentText(share);
  return text === '' ? undefined : Number(text);
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
  readonly goalId?: string;
  readonly status?: string;
}

const asText = (value: unknown): string | undefined => (typeof value === 'string' ? value : undefined);

/**
 * A widget's rows as `GET /dashboards/{id}/figures` answered them, in one shape: a summary's
 * (count, group, period, value) or a GOALS widget's (goal_id, title, status, progress).
 */
export function figureRows(data: readonly Record<string, unknown>[]): FigureRow[] {
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
    const both = group !== undefined && period !== undefined;
    return {
      key: `${group ?? ''}|${period ?? ''}|${i}`,
      label: both ? group : (group ?? period ?? ''),
      ...(both ? { column: period } : {}),
      value,
      text: raw ?? String(count),
      count,
    };
  });
}

/** The total a NUMBER widget shows: the sum of its rows' values. */
export function totalOf(rows: readonly FigureRow[]): number {
  return rows.reduce((sum, row) => sum + row.value, 0);
}

/** The ten label colours of the design system, in the order series take them. */
export const SERIES = ['blue', 'teal', 'amber', 'violet', 'green', 'magenta', 'orange', 'lime', 'red', 'slate'] as const;
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
    return { key: row.key, x: i * slot + gap / 2, y: height - h, width: slot - gap, height: h, series: seriesOf(i) };
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
  for (const row of rows) {
    const column = row.column ?? '';
    if (!rowNames.includes(row.label)) rowNames.push(row.label);
    if (!columnNames.includes(column)) columnNames.push(column);
    at.set(`${row.label}\u0000${column}`, row.text);
  }
  columnNames.sort();
  return {
    rows: rowNames,
    columns: columnNames,
    cells: rowNames.map((r) => columnNames.map((c) => at.get(`${r}\u0000${c}`) ?? '')),
  };
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
