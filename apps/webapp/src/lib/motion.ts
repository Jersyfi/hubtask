// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Reduced motion, as the product's own preference (F5-12, `design-system.md` §10 row 2.3.3).
 *
 * The media query has been honoured since wave 1: every component that moves has a
 * `prefers-reduced-motion` rule. What a media query cannot give is a choice the product makes on
 * its own - a reader whose operating system does not offer the setting, or who wants it here and
 * nowhere else. That choice is `data-motion="reduced"` on the document, which the same components
 * honour through `[data-motion='reduced']` beside their media query, and this module is the one
 * place that sets it (decision 9: one module, one attribute, one owner).
 *
 * It belongs to the device for the reason the theme does (ADR-0043): motion, like the theme, is a
 * property of the screen in front of the reader and not of the person. So the choice is kept the
 * way the theme's is - in the browser's storage until the persistence port arrives with the
 * shells - and shown in the profile beside the theme's switch. There is no "full" choice over a
 * device that asked for less: the media query is honoured whatever is chosen here, because a
 * product that overrode an operating-system accessibility setting would be the failure this row
 * exists to prevent.
 */

import type { ChoiceStore } from './theme.ts';

/** What a device may choose: the system's word, or reduced over it. */
export type MotionChoice = 'system' | 'reduced';

/** The key the choice is kept under on this device. */
export const MOTION_KEY = 'hubtask.motion';

/** The seam node/test code can reach: everything of the document this module touches. */
export interface MotionTarget {
  setAttribute(name: string, value: string): void;
  removeAttribute(name: string): void;
}

/** Sets the attribute for `reduced` and removes it for `system`, where the media query decides. */
export function applyMotion(root: MotionTarget, choice: MotionChoice): void {
  if (choice === 'reduced') root.setAttribute('data-motion', 'reduced');
  else root.removeAttribute('data-motion');
}

/** The choice kept on this device, or `system` where none is or the store cannot be reached. */
export function readMotionChoice(store: ChoiceStore | undefined): MotionChoice {
  try {
    return store?.getItem(MOTION_KEY) === 'reduced' ? 'reduced' : 'system';
  } catch {
    return 'system';
  }
}

/** Keeps the choice on this device; `system` removes it, because following the system is the absence of a choice. */
export function storeMotionChoice(store: ChoiceStore | undefined, choice: MotionChoice): void {
  try {
    if (choice === 'system') store?.removeItem(MOTION_KEY);
    else store?.setItem(MOTION_KEY, choice);
  } catch {
    // A store that refuses loses the choice for the next visit and nothing else.
  }
}
