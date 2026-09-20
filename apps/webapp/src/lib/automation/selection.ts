// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/** What is selected on the canvas, and therefore what the inspector shows (F8-04). */
export type Selection =
  | { kind: 'rule' }
  | { kind: 'trigger' }
  | { kind: 'scope' }
  | { kind: 'runas' }
  | { kind: 'gate' }
  | { kind: 'condition'; index: number }
  | { kind: 'step'; path: string }
  | { kind: 'guardrails' };

/**
 * What is being dragged, while it is (F8-05, decision 7): a piece from the palette - a trigger,
 * a condition, or an action kind - or a card of the chain by its path. Only what can take it is a
 * target while it is lifted; everything else is inert and refuses the drop.
 */
export type Drag = { src: 'trigger'; kind: string } | { src: 'condition' } | { src: 'action'; kind: string } | { src: 'step'; path: string };

/** Whether a gap of the chain may take what is being dragged: a step, unless into its own arm. */
export function gapTakes(drag: Drag | undefined, list: string): boolean {
  if (!drag) return false;
  if (drag.src === 'action') return true;
  if (drag.src === 'step') return list !== drag.path && !list.startsWith(`${drag.path}/`);
  return false;
}

/** The drag payload as the browser carries it between dragstart and drop. */
export const DRAG_TYPE = 'application/x-hubtask-rule-piece';
