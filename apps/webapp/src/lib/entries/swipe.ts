// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a finger did on a row or a board: a swipe, a still hold, a hold that moves, or nothing yet
 * (ADR-0088, `design-system.md` §11.9). Pure, so the board and the list read one gesture the same
 * way, and so the rules can be tested without a screen.
 *
 * - A **swipe** is a horizontal move past the threshold, more sideways than down, made before the
 *   hold timer fires. Vertical movement is a scroll and is left to the browser.
 * - A **still hold** is the timer firing with the finger within the threshold: it opens the row's
 *   menu when the finger lifts.
 * - A **hold that moves** is movement after the timer fired: it drags.
 * - A touch that starts within the screen edge's inset is the platform's own back gesture, and is
 *   no gesture of ours.
 *
 * The distances are read from the design tokens (`--density-control-md-min` for the threshold,
 * `--sp-300` for the edge inset), never written here (rule 15).
 */

/** The coarse pointer's hold, the same the drag helper waits for, so one press cannot mean two things. */
export const HOLD_MS = 300;

export interface Point {
  readonly x: number;
  readonly y: number;
  /** Milliseconds, from any one clock. */
  readonly at: number;
}

export type Gesture = 'swipe-end' | 'swipe-start' | 'hold' | 'drag' | 'scroll' | 'pending';

/**
 * What the movement from `start` to `now` is, with `threshold` px for a swipe, the hold after
 * `holdMs`, and the inline end on the right unless `isRtl`.
 */
export function classify(start: Point, now: Point, threshold: number, isRtl = false, holdMs = HOLD_MS): Gesture {
  const dx = now.x - start.x;
  const dy = now.y - start.y;
  const moved = Math.max(Math.abs(dx), Math.abs(dy)) >= threshold;
  const held = now.at - start.at >= holdMs;
  if (held) return moved ? 'drag' : 'hold';
  if (!moved) return 'pending';
  if (Math.abs(dy) >= Math.abs(dx)) return 'scroll';
  const towardsEnd = isRtl ? dx < 0 : dx > 0;
  return towardsEnd ? 'swipe-end' : 'swipe-start';
}

/** Whether a horizontal move of `dx` against `dy` is a swipe at all: the board's rule, kept here. */
export function isSwipe(dx: number, dy: number, threshold: number): boolean {
  return Math.abs(dx) >= threshold && Math.abs(dx) >= Math.abs(dy);
}

/** Whether a touch at `x` starts in the edge band of a screen `width` wide, `inset` deep. */
export function startsAtEdge(x: number, width: number, inset: number): boolean {
  return x < inset || x > width - inset;
}

function lengthOf(element: Element | null, property: string): number {
  if (!element) return 0;
  const value = Number.parseFloat(getComputedStyle(element).getPropertyValue(property));
  return Number.isFinite(value) ? value : 0;
}

/** How far a swipe travels: the density's control size, read rather than written. */
export function swipeThreshold(element: Element | null): number {
  return lengthOf(element, '--density-control-md-min');
}

/** How deep the screen edge's band is, which the platform's back gesture owns. */
export function edgeInset(element: Element | null): number {
  return lengthOf(element, '--sp-300');
}
