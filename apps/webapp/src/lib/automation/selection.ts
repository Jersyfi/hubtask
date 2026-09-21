// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { canPlace, parentOf, removeAt, stepAt, type Step } from './model.ts';

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

/**
 * Whether a gap of the chain may take what is being dragged: a step, unless into its own arm and
 * unless where a stop forbids it (decision 14).
 */
export function gapTakes(drag: Drag | undefined, list: string, index: number, actions: readonly Step[]): boolean {
  if (!drag) return false;
  if (drag.src === 'action') return canPlace(actions, list, index, drag.kind);
  if (drag.src === 'step') {
    if (list === drag.path || list.startsWith(`${drag.path}/`)) return false;
    // The gap as it would be with the piece lifted out: a step moved within its own list has one
    // gap fewer below it, which is what moveStep counts as well.
    const source = parentOf(drag.path);
    const without = removeAt(actions, drag.path);
    const at = source.list === list && source.index < index ? index - 1 : index;
    return canPlace(without, list, at, stepAt(actions, drag.path)?.kind ?? '');
  }
  return false;
}

/** The drag payload as the browser carries it between dragstart and drop. */
export const DRAG_TYPE = 'application/x-hubtask-rule-piece';
