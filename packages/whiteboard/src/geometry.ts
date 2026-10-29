// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Where things are on a board: an object's box, the ends of a connector on the outlines it is
 * attached to, the path it runs, which objects a viewport shows (so a board of thousands draws
 * only what is in view), which object a point hits, and the viewport's pan and zoom.
 */

import { touches } from './draw.ts';
import type { Element } from './scene.ts';

export interface Point { x: number; y: number }
export interface Box { left: number; top: number; right: number; bottom: number }

/** What the screen shows: the board point at its top-left corner and the scale. */
export interface Viewport { x: number; y: number; zoom: number; width: number; height: number }

export const MIN_ZOOM = 0.1;
export const MAX_ZOOM = 4;

export function box(e: Element): Box {
  return {
    left: Math.min(e.x, e.x + e.width), top: Math.min(e.y, e.y + e.height),
    right: Math.max(e.x, e.x + e.width), bottom: Math.max(e.y, e.y + e.height),
  };
}

export function center(e: Element): Point {
  return { x: e.x + e.width / 2, y: e.y + e.height / 2 };
}

/** Where the line from an object's centre toward a point leaves its box. */
export function outline(e: Element, toward: Point): Point {
  const c = center(e);
  const dx = toward.x - c.x;
  const dy = toward.y - c.y;
  if (dx === 0 && dy === 0) return c;
  const halfW = Math.abs(e.width) / 2;
  const halfH = Math.abs(e.height) / 2;
  const scale = Math.min(dx === 0 ? Infinity : halfW / Math.abs(dx), dy === 0 ? Infinity : halfH / Math.abs(dy));
  return { x: c.x + dx * scale, y: c.y + dy * scale };
}

/**
 * A connector's two ends: on the outline of the object each is attached to, facing the other
 * end, or the free point (x, y) and (x + width, y + height) where it is attached to nothing - or
 * to an object no longer on the board.
 */
export function ends(connector: Element, byId: ReadonlyMap<string, Element>): [Point, Point] {
  const from = connector.from ? byId.get(connector.from) : undefined;
  const to = connector.to ? byId.get(connector.to) : undefined;
  const freeStart = { x: connector.x, y: connector.y };
  const freeEnd = { x: connector.x + connector.width, y: connector.y + connector.height };
  const startAim = to ? center(to) : freeEnd;
  const endAim = from ? center(from) : freeStart;
  return [from ? outline(from, startAim) : freeStart, to ? outline(to, endAim) : freeEnd];
}

/** The SVG path a connector runs along. */
export function connectorPath(connector: Element, byId: ReadonlyMap<string, Element>): string {
  const [a, b] = ends(connector, byId);
  const n = (value: number) => Math.round(value * 10) / 10;
  switch (connector.route) {
    case 'ELBOW': {
      const midX = (a.x + b.x) / 2;
      return `M ${n(a.x)} ${n(a.y)} H ${n(midX)} V ${n(b.y)} H ${n(b.x)}`;
    }
    case 'CURVED': {
      const bend = Math.abs(b.x - a.x) / 2;
      return `M ${n(a.x)} ${n(a.y)} C ${n(a.x + bend)} ${n(a.y)} ${n(b.x - bend)} ${n(b.y)} ${n(b.x)} ${n(b.y)}`;
    }
    default:
      return `M ${n(a.x)} ${n(a.y)} L ${n(b.x)} ${n(b.y)}`;
  }
}

/** Where a connector's label stands: half way along. */
export function labelPoint(connector: Element, byId: ReadonlyMap<string, Element>): Point {
  const [a, b] = ends(connector, byId);
  return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
}

function overlaps(a: Box, b: Box): boolean {
  return a.left <= b.right && a.right >= b.left && a.top <= b.bottom && a.bottom >= b.top;
}

/** The part of the board a viewport shows, in board units. */
export function visibleBox(view: Viewport): Box {
  return { left: view.x, top: view.y, right: view.x + view.width / view.zoom, bottom: view.y + view.height / view.zoom };
}

/**
 * The objects a viewport shows, in drawing order: those whose box meets it, and the connectors
 * with an end among them or running across it. Everything else is not drawn at all.
 */
export function culled(elements: readonly Element[], view: Viewport, margin = 0): Element[] {
  const seen = visibleBox(view);
  const area = { left: seen.left - margin, top: seen.top - margin, right: seen.right + margin, bottom: seen.bottom + margin };
  const byId = new Map(elements.map((e) => [e.id, e]));
  return elements.filter((e) => {
    if (e.kind !== 'CONNECTOR') return overlaps(box(e), area);
    const [a, b] = ends(e, byId);
    return overlaps({ left: Math.min(a.x, b.x), top: Math.min(a.y, b.y), right: Math.max(a.x, b.x), bottom: Math.max(a.y, b.y) }, area);
  });
}

/** The top-most object under a point, connectors aside; a frame only where nothing else is. */
export function hit(elements: readonly Element[], point: Point): Element | undefined {
  let frame: Element | undefined;
  for (let i = elements.length - 1; i >= 0; i--) {
    const e = elements[i];
    if (!e || e.kind === 'CONNECTOR') continue;
    if (e.kind === 'STROKE') {
      if (touches(e, point, 6)) return e;
      continue;
    }
    const b = box(e);
    if (point.x < b.left || point.x > b.right || point.y < b.top || point.y > b.bottom) continue;
    if (e.kind !== 'FRAME') return e;
    frame ??= e;
  }
  return frame;
}

/** The objects wholly inside a box - a frame's, or a selection's rectangle. */
export function inside(elements: readonly Element[], area: Box): Element[] {
  return elements.filter((e) => {
    if (e.kind === 'CONNECTOR') return false;
    const b = box(e);
    return b.left >= area.left && b.top >= area.top && b.right <= area.right && b.bottom <= area.bottom;
  });
}

/** The objects a frame holds - itself and what lies wholly in it - and their connectors. */
export function withinFrame(elements: readonly Element[], frame: Element): Element[] {
  const ids = new Set([frame.id, ...inside(elements, box(frame)).map((e) => e.id)]);
  return elements.filter((e) => (e.kind === 'CONNECTOR' ? ids.has(e.from ?? '') && ids.has(e.to ?? '') : ids.has(e.id)));
}

/** The box around objects, connectors' ends included; undefined for none. */
export function bounds(elements: readonly Element[]): Box | undefined {
  const byId = new Map(elements.map((e) => [e.id, e]));
  let out: Box | undefined;
  for (const e of elements) {
    let b = box(e);
    if (e.kind === 'CONNECTOR') {
      const [p, q] = ends(e, byId);
      b = { left: Math.min(p.x, q.x), top: Math.min(p.y, q.y), right: Math.max(p.x, q.x), bottom: Math.max(p.y, q.y) };
    }
    out = out
      ? { left: Math.min(out.left, b.left), top: Math.min(out.top, b.top), right: Math.max(out.right, b.right), bottom: Math.max(out.bottom, b.bottom) }
      : b;
  }
  return out;
}

export function toBoard(view: Viewport, screen: Point): Point {
  return { x: view.x + screen.x / view.zoom, y: view.y + screen.y / view.zoom };
}

/** The viewport zoomed by a factor around a screen point, which stays where it is. */
export function zoomAt(view: Viewport, screen: Point, factor: number): Viewport {
  const zoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, view.zoom * factor));
  const at = toBoard(view, screen);
  return { ...view, zoom, x: at.x - screen.x / zoom, y: at.y - screen.y / zoom };
}

/** The viewport that shows a box whole, centred, with a margin of screen units. */
export function fit(view: Viewport, area: Box, margin = 48): Viewport {
  const w = Math.max(1, area.right - area.left);
  const h = Math.max(1, area.bottom - area.top);
  const zoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, Math.min((view.width - 2 * margin) / w, (view.height - 2 * margin) / h)));
  return {
    ...view, zoom,
    x: area.left - (view.width / zoom - w) / 2,
    y: area.top - (view.height / zoom - h) / 2,
  };
}

/** A value snapped to the grid. */
export function snap(value: number, grid: number): number {
  return grid > 0 ? Math.round(value / grid) * grid : value;
}
