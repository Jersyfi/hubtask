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

/** One line of the burndown as SVG points in a box of the given width and height. */
export function burndownLine(
  points: readonly BurndownPoint[], key: 'remaining' | 'ideal' | 'scope', days: number, top: number, width: number, height: number,
): string {
  if (points.length === 0 || days <= 0 || top <= 0) return '';
  const step = days > 1 ? width / (days - 1) : 0;
  return points
    .map((point, i) => `${round(i * step)},${round(height - (point[key] / top) * height)}`)
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

function round(n: number): number {
  return Math.round(n * 10) / 10;
}
