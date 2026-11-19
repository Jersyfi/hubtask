// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The board's dock (W-2, W-3): its groups, every tool's shortcut, and what a tool does after one
 * use. Pure, so the decisions are tested without a canvas.
 */

import type { Element } from '@hubtask/whiteboard';

/** A tool of the dock: the hand and the comment are the dock's own, the others an object's kind. */
export type DockTool =
  | 'select' | 'HAND' | 'STICKY' | 'SHAPE' | 'TEXT' | 'CONNECTOR' | 'FRAME' | 'STROKE' | 'HIGHLIGHTER' | 'ERASER' | 'COMMENT';

export type DockGroup = 'move' | 'create' | 'draw' | 'conversation';

/** The dock in its order: move · create · draw · conversation; *insert* closes it as a menu. */
export const DOCK: readonly { group: DockGroup; tools: readonly { tool: DockTool; key: string }[] }[] = [
  { group: 'move', tools: [{ tool: 'select', key: 'V' }, { tool: 'HAND', key: 'H' }] },
  {
    group: 'create',
    tools: [
      { tool: 'STICKY', key: 'N' }, { tool: 'SHAPE', key: 'R' }, { tool: 'TEXT', key: 'T' },
      { tool: 'CONNECTOR', key: 'L' }, { tool: 'FRAME', key: 'F' },
    ],
  },
  { group: 'draw', tools: [{ tool: 'STROKE', key: 'P' }, { tool: 'HIGHLIGHTER', key: 'M' }, { tool: 'ERASER', key: 'E' }] },
  { group: 'conversation', tools: [{ tool: 'COMMENT', key: 'C' }] },
];

/** On a phone the dock holds these; the rest wait under ⋯ (§7 Whiteboard). */
export const PHONE_DOCK: readonly DockTool[] = ['select', 'STICKY', 'TEXT', 'COMMENT'];

/** The tool a single key chooses, or `undefined`: a key with a modifier is never a tool. */
export function toolOfKey(key: string, modified: boolean): DockTool | undefined {
  if (modified || key.length !== 1) return undefined;
  const upper = key.toUpperCase();
  for (const group of DOCK) for (const each of group.tools) if (each.key === upper) return each.tool;
  return undefined;
}

export function shortcutOf(tool: DockTool): string {
  for (const group of DOCK) for (const each of group.tools) if (each.tool === tool) return each.key;
  return '';
}

/**
 * The tool after one use (W-3): a placing tool springs back to select unless it is held (a double
 * click held it); the hand, the pens and the eraser stay, as one stroke is rarely all.
 */
export function afterUse(tool: DockTool, held: boolean): DockTool {
  if (held) return tool;
  return tool === 'HAND' || tool === 'STROKE' || tool === 'HIGHLIGHTER' || tool === 'ERASER' ? tool : 'select';
}

/** Where "+" or Tab puts the neighbour of a note: beside it to the right, its own size, a gap between. */
export function neighbourOf(of: Pick<Element, 'x' | 'y' | 'width' | 'height'>, side: 'right' | 'below' = 'right', gap = 20): { x: number; y: number } {
  return side === 'right'
    ? { x: of.x + of.width + gap + of.width / 2, y: of.y + of.height / 2 }
    : { x: of.x + of.width / 2, y: of.y + of.height + gap + of.height / 2 };
}

/** The share of the surface the controls cover (W-1: under 8 %). */
export function coverOf(surface: { width: number; height: number }, controls: readonly { width: number; height: number }[]): number {
  const area = surface.width * surface.height;
  if (area <= 0) return 0;
  return controls.reduce((sum, each) => sum + each.width * each.height, 0) / area;
}
