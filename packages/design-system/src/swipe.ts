// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A row swiped on a phone (work-surface concept §7): right completes, left opens the row's menu.
// Never the only way - the same acts stand in the row's menu (WCAG 2.5.7) - and never in the way of
// scrolling: a gesture that starts more upright than level is the page's, not the row's.

/** What a finished gesture asks for. */
export type SwipeOutcome = 'right' | 'left' | 'none';

/** The share of the row's width a swipe has to travel before letting go acts. */
export const SWIPE_SHARE = 0.3;
/** The shortest travel that acts on a narrow row, in CSS pixels of the gesture. */
export const SWIPE_MIN = 56;

/**
 * Whether a gesture is a swipe at all, decided once it has moved: more level than upright, and
 * past the slop a resting thumb wanders. Undecided gestures return `undefined`.
 */
export function swipeIntent(dx: number, dy: number, slop = 8): 'swipe' | 'scroll' | undefined {
  if (Math.abs(dx) < slop && Math.abs(dy) < slop) return undefined;
  return Math.abs(dx) > Math.abs(dy) ? 'swipe' : 'scroll';
}

/** What letting go after `dx` across a row `width` wide does. */
export function swipeOutcome(dx: number, width: number): SwipeOutcome {
  const threshold = Math.max(SWIPE_MIN, width * SWIPE_SHARE);
  if (dx >= threshold) return 'right';
  if (dx <= -threshold) return 'left';
  return 'none';
}

/**
 * How far the row follows the thumb: one to one up to the threshold, then a third as far, so the
 * row says "this far is enough" without a number. A side with no act does not move at all.
 */
export function swipeOffset(dx: number, width: number, sides: { right: boolean; left: boolean }): number {
  if ((dx > 0 && !sides.right) || (dx < 0 && !sides.left)) return 0;
  const threshold = Math.max(SWIPE_MIN, width * SWIPE_SHARE);
  const magnitude = Math.abs(dx);
  const travelled = magnitude <= threshold ? magnitude : threshold + (magnitude - threshold) / 3;
  return Math.sign(dx) * travelled;
}
