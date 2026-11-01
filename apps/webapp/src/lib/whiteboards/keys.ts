// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The board's keyboard decisions, pure so they can be tested without a canvas (WCAG 2.1.1,
 * 2.1.2, 2.5.7): what Tab chooses, and what a tool chosen without a pointer does.
 */

/**
 * The object Tab (or Shift+Tab) chooses next, or `undefined` when focus is to leave the board:
 * past the last object forwards, before the first backwards. With nothing chosen, Tab takes the
 * first and Shift+Tab the last. A board that cycled for ever would hold a keyboard user inside it.
 */
export function tabTarget(order: readonly string[], current: string | undefined, back: boolean): string | undefined {
  if (order.length === 0) return undefined;
  const at = current === undefined ? -1 : order.indexOf(current);
  if (at < 0) return back ? order[order.length - 1] : order[0];
  const next = at + (back ? -1 : 1);
  return next < 0 || next >= order.length ? undefined : order[next];
}

/**
 * What a tool chosen from the keyboard does at once, since no pointer will press the board:
 * an object is put at the centre of the view and edited, as N does; a link asks for its address
 * there; a connector is drawn from the chosen object's "Connect to" in the side panel; a pen and an
 * eraser follow a path and stay pointer tools (2.5.7's essential exception).
 */
export function keyboardTool(tool: string): 'place' | 'link' | 'connect' | 'pointer' | 'none' {
  switch (tool) {
    case 'STICKY':
    case 'SHAPE':
    case 'TEXT':
    case 'FRAME':
      return 'place';
    case 'LINK':
      return 'link';
    case 'CONNECTOR':
      return 'connect';
    case 'STROKE':
    case 'ERASER':
      return 'pointer';
    default:
      return 'none';
  }
}

/** A size typed into the side panel, as the board keeps it: whole, and at least the handle's 24. */
export function sizeOf(typed: string): number | undefined {
  const value = Math.round(Number(typed));
  return Number.isFinite(value) && typed.trim() !== '' ? Math.max(24, value) : undefined;
}
