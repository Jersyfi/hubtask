// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Freehand drawing: a line the pointer drew made one or more strokes (the board keeps at most
 * 1,000 points per stroke, `whiteboards.stroke_points_invalid`), the smooth path a stroke is drawn
 * along, and what the eraser touches.
 */

import type { Point } from './geometry.ts';
import type { Element, Pen } from './scene.ts';

/** The most points one stroke keeps; a longer line becomes several strokes. */
export const MAX_STROKE_POINTS = 1000;

/** How wide each pen draws, in board units. */
export const PEN_WIDTH: Record<Pen, number> = { PEN: 3, HIGHLIGHTER: 16 };

/** Drops the points that bend a line by less than a tolerance (Ramer-Douglas-Peucker). */
export function simplify(points: readonly Point[], tolerance = 1.5): Point[] {
  if (points.length < 3) return [...points];
  const keep = new Uint8Array(points.length);
  keep[0] = 1;
  keep[points.length - 1] = 1;
  const stack: [number, number][] = [[0, points.length - 1]];
  while (stack.length > 0) {
    const [first, last] = stack.pop() as [number, number];
    let farthest = -1;
    let distance = tolerance;
    for (let i = first + 1; i < last; i++) {
      const d = toSegment(points[i] as Point, points[first] as Point, points[last] as Point);
      if (d > distance) {
        distance = d;
        farthest = i;
      }
    }
    if (farthest >= 0) {
      keep[farthest] = 1;
      stack.push([first, farthest], [farthest, last]);
    }
  }
  return points.filter((_, i) => keep[i] === 1);
}

/** A drawn line as strokes of the board: simplified, cut into pieces the board keeps, each in its own box. */
export function strokesOf(line: readonly Point[], pen: Pen, newId: () => string, colour?: string): Element[] {
  const points = simplify(line);
  if (points.length === 0) return [];
  const out: Element[] = [];
  for (let start = 0; start < points.length; start += MAX_STROKE_POINTS - 1) {
    const piece = points.slice(start, start + MAX_STROKE_POINTS);
    if (piece.length === 0 || (start > 0 && piece.length < 2)) break;
    const left = Math.min(...piece.map((p) => p.x));
    const top = Math.min(...piece.map((p) => p.y));
    const right = Math.max(...piece.map((p) => p.x));
    const bottom = Math.max(...piece.map((p) => p.y));
    const round = (value: number) => Math.round(value * 10) / 10;
    const x = Math.floor(left);
    const y = Math.floor(top);
    out.push({
      id: newId(), kind: 'STROKE', pen, x, y, width: Math.ceil(right) - x, height: Math.ceil(bottom) - y,
      points: piece.flatMap((p) => [round(p.x - x), round(p.y - y)]),
      ...(colour ? { color_token: colour } : {}),
    });
  }
  return out;
}

/** A stroke's points on the board. */
export function strokePoints(e: Element): Point[] {
  const flat = e.points ?? [];
  const out: Point[] = [];
  for (let i = 0; i + 1 < flat.length; i += 2) out.push({ x: e.x + (flat[i] as number), y: e.y + (flat[i + 1] as number) });
  return out;
}

/** The smooth SVG path along a stroke: quadratic curves through the midpoints of its points. */
export function strokePath(e: Element): string {
  const points = strokePoints(e);
  const n = (value: number) => Math.round(value * 10) / 10;
  const [first] = points;
  if (!first) return '';
  if (points.length === 1) return `M ${n(first.x)} ${n(first.y)} l 0.1 0`;
  let d = `M ${n(first.x)} ${n(first.y)}`;
  for (let i = 1; i < points.length - 1; i++) {
    const p = points[i] as Point;
    const q = points[i + 1] as Point;
    d += ` Q ${n(p.x)} ${n(p.y)} ${n((p.x + q.x) / 2)} ${n((p.y + q.y) / 2)}`;
  }
  const last = points[points.length - 1] as Point;
  return `${d} L ${n(last.x)} ${n(last.y)}`;
}

/** Whether the eraser at a point, of a radius, touches a stroke. */
export function touches(e: Element, at: Point, radius: number): boolean {
  if (e.kind !== 'STROKE') return false;
  const reach = radius + PEN_WIDTH[e.pen ?? 'PEN'] / 2;
  if (at.x < e.x - reach || at.x > e.x + e.width + reach || at.y < e.y - reach || at.y > e.y + e.height + reach) return false;
  const points = strokePoints(e);
  if (points.length === 1) return Math.hypot(at.x - (points[0] as Point).x, at.y - (points[0] as Point).y) <= reach;
  for (let i = 1; i < points.length; i++) {
    if (toSegment(at, points[i - 1] as Point, points[i] as Point) <= reach) return true;
  }
  return false;
}

function toSegment(p: Point, a: Point, b: Point): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const length = dx * dx + dy * dy;
  const t = length === 0 ? 0 : Math.max(0, Math.min(1, ((p.x - a.x) * dx + (p.y - a.y) * dy) / length));
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy));
}
