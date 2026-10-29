// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Help to place things: alignment guides while objects move (an edge or a centre that comes
 * close to another object's snaps to it, and the line is drawn), and the minimap - the whole
 * board in a small box, the viewport marked on it.
 */

import { bounds, box, type Box, type Point, type Viewport } from './geometry.ts';
import type { Element } from './scene.ts';

/** A guide drawn while moving: a vertical line at `x` or a horizontal line at `y`, between two ends. */
export type Guide = { axis: 'x'; at: number; from: number; to: number } | { axis: 'y'; at: number; from: number; to: number };

export interface Alignment {
  /** How far the moving box is to move further to stand aligned. */
  dx: number;
  dy: number;
  guides: Guide[];
}

/**
 * Aligns a moving box to the others: of its left, centre and right the one nearest another
 * object's left, centre or right within a tolerance snaps to it, the same for top, middle and
 * bottom. Only the objects near the viewport are worth handing in.
 */
export function align(moving: Box, others: readonly Element[], tolerance: number): Alignment {
  const xs = [moving.left, (moving.left + moving.right) / 2, moving.right];
  const ys = [moving.top, (moving.top + moving.bottom) / 2, moving.bottom];
  let bestX: { delta: number; at: number; other: Box } | undefined;
  let bestY: { delta: number; at: number; other: Box } | undefined;
  for (const e of others) {
    if (e.kind === 'CONNECTOR') continue;
    const b = box(e);
    for (const target of [b.left, (b.left + b.right) / 2, b.right]) {
      for (const x of xs) {
        const delta = target - x;
        if (Math.abs(delta) <= tolerance && (!bestX || Math.abs(delta) < Math.abs(bestX.delta))) bestX = { delta, at: target, other: b };
      }
    }
    for (const target of [b.top, (b.top + b.bottom) / 2, b.bottom]) {
      for (const y of ys) {
        const delta = target - y;
        if (Math.abs(delta) <= tolerance && (!bestY || Math.abs(delta) < Math.abs(bestY.delta))) bestY = { delta, at: target, other: b };
      }
    }
  }
  const guides: Guide[] = [];
  if (bestX) {
    guides.push({
      axis: 'x', at: bestX.at,
      from: Math.min(moving.top, bestX.other.top) + (bestY?.delta ?? 0), to: Math.max(moving.bottom, bestX.other.bottom) + (bestY?.delta ?? 0),
    });
  }
  if (bestY) {
    guides.push({
      axis: 'y', at: bestY.at,
      from: Math.min(moving.left, bestY.other.left) + (bestX?.delta ?? 0), to: Math.max(moving.right, bestY.other.right) + (bestX?.delta ?? 0),
    });
  }
  return { dx: bestX?.delta ?? 0, dy: bestY?.delta ?? 0, guides };
}

/** The minimap: the board's objects and the viewport, scaled into a box of a size. */
export interface Minimap {
  scale: number;
  /** The board point at the minimap's top-left corner. */
  origin: Point;
  /** Each object's box on the minimap, connectors and strokes aside. */
  boxes: { id: string; kind: Element['kind']; left: number; top: number; width: number; height: number }[];
  /** The viewport on the minimap. */
  view: { left: number; top: number; width: number; height: number };
}

export function minimap(elements: readonly Element[], view: Viewport, size: { width: number; height: number }): Minimap {
  const seen: Box = { left: view.x, top: view.y, right: view.x + view.width / view.zoom, bottom: view.y + view.height / view.zoom };
  const all = bounds(elements);
  const area: Box = all
    ? { left: Math.min(all.left, seen.left), top: Math.min(all.top, seen.top), right: Math.max(all.right, seen.right), bottom: Math.max(all.bottom, seen.bottom) }
    : seen;
  const scale = Math.min(size.width / Math.max(1, area.right - area.left), size.height / Math.max(1, area.bottom - area.top));
  const origin = { x: area.left, y: area.top };
  const place = (b: Box) => ({
    left: (b.left - origin.x) * scale, top: (b.top - origin.y) * scale,
    width: Math.max(1, (b.right - b.left) * scale), height: Math.max(1, (b.bottom - b.top) * scale),
  });
  return {
    scale, origin,
    boxes: elements.filter((e) => e.kind !== 'CONNECTOR').map((e) => ({ id: e.id, kind: e.kind, ...place(box(e)) })),
    view: place(seen),
  };
}

/** The viewport centred on a point of the minimap. */
export function fromMinimap(map: Minimap, view: Viewport, at: Point): Viewport {
  const board = { x: map.origin.x + at.x / map.scale, y: map.origin.y + at.y / map.scale };
  return { ...view, x: board.x - view.width / view.zoom / 2, y: board.y - view.height / view.zoom / 2 };
}
