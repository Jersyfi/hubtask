// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The sprint screen's arithmetic (UC-PLN-06, UC-PLN-07): an estimate read from a field and shown,
 * the burndown laid out as two lines in a box, and the sprints a closer may carry into.
 *
 * Pure, so that "what is an estimate" and "where is a day on the chart" are checked by
 * `node --test` rather than by driving a browser. The server counts commitment, result, burndown
 * and velocity; this module only reads and draws them.
 */

import type { BurndownPoint, Estimate, Sprint } from '@hubtask/sync-engine';

export const sizes = ['XS', 'S', 'M', 'L', 'XL', 'XXL'] as const;
export type Size = (typeof sizes)[number];

/** What the estimate field holds, as the request body it becomes; undefined for a value refused. */
export type EstimateInput = { unit: 'POINTS' | 'HOURS'; value: number } | { unit: 'SIZE'; size: Size };

/**
 * Reads an estimate typed in a field: a T-shirt size (any case), or a number of points - or of
 * hours with an `h` after it - of at most 10000 with two decimals; a comma is a decimal point.
 * An empty field is `null`, the estimate cleared.
 */
export function estimateOf(text: string): EstimateInput | null | undefined {
  const raw = text.trim();
  if (raw === '') return null;
  const size = raw.toUpperCase();
  if ((sizes as readonly string[]).includes(size)) return { unit: 'SIZE', size: size as Size };
  const hours = /h$/i.test(raw);
  const number = Number((hours ? raw.slice(0, -1) : raw).trim().replace(',', '.'));
  if (!Number.isFinite(number) || number < 0 || number > 10000) return undefined;
  if (Math.abs(number * 100 - Math.round(number * 100)) > 1e-6) return undefined;
  return { unit: hours ? 'HOURS' : 'POINTS', value: number };
}

/** An estimate as the field shows it: a size, `5`, or `1.5h`; empty without one. */
export function estimateText(estimate: Estimate | null | undefined): string {
  if (!estimate) return '';
  if (estimate.unit === 'SIZE') return estimate.size ?? '';
  return estimate.unit === 'HOURS' ? `${estimate.value}h` : String(estimate.value);
}

/**
 * One line of the burndown as SVG points in a box of the given width and height, `inset` from its
 * top and bottom so a stroke on either edge is drawn whole.
 */
export function burndownLine(
  points: readonly BurndownPoint[], key: 'remaining' | 'ideal' | 'scope', days: number, top: number, width: number, height: number,
  inset = 0,
): string {
  if (points.length === 0 || days <= 0 || top <= 0) return '';
  const step = days > 1 ? width / (days - 1) : 0;
  const room = height - 2 * inset;
  // A day past the sprint's last or a value past the top stays on the frame's edge, never beyond.
  return points
    .slice(0, days)
    .map((point, i) => `${round(i * step)},${round(inset + room - (Math.min(Math.max(point[key], 0), top) / top) * room)}`)
    .join(' ');
}

/** The highest value the burndown's box must hold: the commitment, the scope or the work left. */
export function burndownTop(sprint: Sprint, points: readonly BurndownPoint[]): number {
  return Math.max(sprint.committed_value, ...points.map((p) => Math.max(p.scope, p.remaining, p.ideal)), 0);
}

/** How many calendar days a sprint spans, both ends included. */
export function sprintDays(sprint: Pick<Sprint, 'start_on' | 'end_on'>): number {
  const from = Date.parse(`${sprint.start_on}T00:00:00Z`);
  const to = Date.parse(`${sprint.end_on}T00:00:00Z`);
  return Number.isFinite(from) && Number.isFinite(to) ? Math.round((to - from) / 86_400_000) + 1 : 0;
}

/** The sprints a running one's unfinished entries may move into: the planned ones, soonest first. */
export function carryTargets(sprints: readonly Sprint[], closing: string): Sprint[] {
  return sprints
    .filter((s) => s.state === 'PLANNED' && s.id !== closing)
    .sort((a, b) => a.start_on.localeCompare(b.start_on));
}

/** The sprint a screen opens on: the running one, else the next planned, else the latest. */
export function sprintShown(sprints: readonly Sprint[]): Sprint | undefined {
  return sprints.find((s) => s.state === 'ACTIVE') ?? carryTargets(sprints, '')[0] ?? sprints[0];
}

/** What a T-shirt size counts as in a sprint of points (UC-PLN-07 check 1). */
const SIZE_POINTS: Readonly<Record<Size, number>> = { XS: 1, S: 2, M: 3, L: 5, XL: 8, XXL: 13 };

/**
 * The estimates of some entries added up in the sprint's unit, as the server counts a sprint's
 * (UC-PLN-07 check 2): points and sizes in a sprint of points, hours in one of hours; an entry
 * without an estimate or in the other unit adds nothing. A column's figure, not a commitment.
 */
export function valueOf(entries: readonly { estimate?: Estimate | null }[], unit: 'POINTS' | 'HOURS'): number {
  const sum = entries.reduce((total, { estimate }) => {
    if (!estimate) return total;
    if (estimate.unit === 'SIZE') return unit === 'POINTS' && estimate.size ? total + SIZE_POINTS[estimate.size as Size] : total;
    return estimate.unit === unit ? total + (estimate.value ?? 0) : total;
  }, 0);
  return round(sum);
}

/**
 * How full a planned sprint is against the velocity of the last three (E-22): the bar's share, at
 * most the whole, and whether the plan is over it. Nothing without a velocity to plan against.
 * A planning aid, never a limit: the sprint takes what the person puts in.
 */
export function planLoad(planned: number, velocity: number): { readonly percent: number; readonly isOver: boolean } | undefined {
  if (!(velocity > 0)) return undefined;
  return { percent: Math.min(100, Math.round((planned / velocity) * 100)), isOver: planned > velocity };
}

/** A board column as the sprint needs it: its id, its name, whether it is the done column. */
export interface ColumnOf {
  readonly id: string;
  readonly name: string;
  readonly is_done_bucket?: boolean;
  /** Under a workflow, the statuses the column shows. */
  readonly statuses?: readonly string[];
}

/** The workflow's statuses with their categories, in the workflow's order. */
export interface StatusesOf {
  readonly statuses: readonly { readonly key: string; readonly category: string }[];
}

/** A sprint's column: the board's column, or the column-less one (`id` null). */
export interface SprintColumn<E> {
  readonly id: string | null;
  readonly name: string;
  readonly isDone: boolean;
  /** Drawn folded beside the done column, never wide: the column-less one. */
  readonly isAside?: boolean;
  readonly entries: readonly E[];
}

/**
 * A sprint's entries in the board's columns, which carry the workflow's statuses (§6.2): a done
 * entry in the done column, the others in the column they stand in; the rest in a
 * column-less column, drawn only when it holds something and folded beside the done one. Nothing
 * without columns: the caller falls back to to do, in progress and done.
 *
 * Under a workflow, a column of BACKLOG statuses is no column of the sprint (E-22: the backlog is
 * its own tab) and its entries stand in the first open column; a column of DONE statuses without
 * the workflow's first done one (a discard) is folded into the done column.
 */
export function sprintColumns<E extends { readonly item_id: string; readonly done: boolean }>(
  entries: readonly E[],
  columns: readonly ColumnOf[],
  columnOf: (itemId: string) => string | null | undefined,
  noColumnName: string,
  workflow?: StatusesOf,
): SprintColumn<E>[] | undefined {
  if (columns.length === 0) return undefined;
  const categoryOf = new Map((workflow?.statuses ?? []).map((status) => [status.key, status.category]));
  const firstDone = workflow?.statuses.find((status) => status.category === 'DONE')?.key;
  const only = (column: ColumnOf, category: string) =>
    (column.statuses ?? []).length > 0 && (column.statuses ?? []).every((key) => categoryOf.get(key) === category);
  const isBacklog = (column: ColumnOf) => only(column, 'BACKLOG');
  const isDiscard = (column: ColumnOf) => !column.is_done_bucket && only(column, 'DONE') && !(firstDone && column.statuses?.includes(firstDone));
  const shown = columns.filter((column) => !isBacklog(column) && !isDiscard(column));
  const doneColumn = (shown.find((column) => column.is_done_bucket) ?? shown.find((column) => firstDone && column.statuses?.includes(firstDone)))?.id;
  const firstOpen = shown.find((column) => column.id !== doneColumn)?.id;
  const byId = new Map(columns.map((column) => [column.id, column]));
  const placed = new Map<string | null, E[]>();
  for (const entry of entries) {
    const own = columnOf(entry.item_id);
    const column = own ? byId.get(own) : undefined;
    const at =
      entry.done && doneColumn ? doneColumn
      : column && isDiscard(column) ? (doneColumn ?? null)
      : column && isBacklog(column) ? (firstOpen ?? null)
      : column ? column.id
      : null;
    placed.set(at, [...(placed.get(at) ?? []), entry]);
  }
  const drawn: SprintColumn<E>[] = shown.map((column) => ({ id: column.id, name: column.name, isDone: column.id === doneColumn, entries: placed.get(column.id) ?? [] }));
  const rest = placed.get(null);
  return rest ? [...drawn, { id: null, name: noColumnName, isDone: false, isAside: true, entries: rest }] : drawn;
}

/**
 * The open entries whose column is not known from what was read: the collection's read is a page,
 * a sprint's entries are not. A done entry needs none - it stands in the done column.
 */
export function entriesToRead<E extends { readonly item_id: string; readonly done: boolean }>(
  entries: readonly E[],
  isKnown: (itemId: string) => boolean,
): string[] {
  return entries.filter((entry) => !entry.done && !isKnown(entry.item_id)).map((entry) => entry.item_id);
}

function round(n: number): number {
  return Math.round(n * 10) / 10;
}
