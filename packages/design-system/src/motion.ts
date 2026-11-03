// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The product's movements (user-experience.md §6), out where they can be tested.
//
// Rule 6 (design-system.md §6) holds for every one of them: `opacity` and `transform` only, the
// pair behind each movement read from a `motion.<role>` token's custom properties at run time, and
// nothing at all under `prefers-reduced-motion` or `[data-motion="reduced"]`. Reading the tokens at
// run time rather than importing their values keeps `tokens.json` the one place a duration lives
// (rule 15): what this module holds is arithmetic, not values.
//
// Two shapes, because Svelte offers two hooks and a list is owned in two ways:
//
// * `enter`, `leave`, `reveal` and `flip` are Svelte transitions and an animation, for a call site
//   that owns its `{#each}` (`animate:flip` must sit on the each block's own child).
// * `arrive` and `arrivals` are attachments, for a component whose rows are its caller's: `Table`'s
//   body and `BucketColumn`'s cards are snippets, and `ListRow` and `TaskRow` are the row itself.
//   A Svelte intro on a component's root plays only when the component's own block is created,
//   which a row added to its caller's list never is; an attachment runs whenever the node is made.
//
// A node arrives once. A keyed list that is re-sorted moves its nodes, and a moved node must not
// replay its entrance - that is what `flip` is for.

import type { Attachment } from 'svelte/attachments';
import type { AnimationConfig } from 'svelte/animate';
import type { TransitionConfig } from 'svelte/transition';

/** A function from progress to eased progress, as Svelte's transitions take one. */
export type Easing = (t: number) => number;

/** The roles this module moves by, each a `motion.<role>` token pair. */
export type MotionRole = 'entrance' | 'exit' | 'reorder' | 'reveal';

/** What of a computed style this module reads. A subset, so a test needs no DOM. */
export interface StyleSource {
  getPropertyValue(name: string): string;
}

/**
 * How many items of one arrival are staggered (user-experience.md §6). The ninth and every later
 * one arrive with the eighth, so a long list is never waited for.
 */
export const STAGGERED = 8;

const linear: Easing = (t) => t;

/**
 * A cubic Bézier easing, as CSS defines `cubic-bezier(x1, y1, x2, y2)`: x is time, y is progress.
 * Newton's method on x first, bisection where the slope is too flat for it.
 */
export function cubicBezier(x1: number, y1: number, x2: number, y2: number): Easing {
  const cx = 3 * x1;
  const bx = 3 * (x2 - x1) - cx;
  const ax = 1 - cx - bx;
  const cy = 3 * y1;
  const by = 3 * (y2 - y1) - cy;
  const ay = 1 - cy - by;
  const xAt = (s: number) => ((ax * s + bx) * s + cx) * s;
  const yAt = (s: number) => ((ay * s + by) * s + cy) * s;
  const slopeAt = (s: number) => (3 * ax * s + 2 * bx) * s + cx;

  const solve = (x: number): number => {
    let s = x;
    for (let i = 0; i < 8; i++) {
      const error = xAt(s) - x;
      if (Math.abs(error) < 1e-6) return s;
      const slope = slopeAt(s);
      if (Math.abs(slope) < 1e-6) break;
      s -= error / slope;
    }
    let low = 0;
    let high = 1;
    s = x;
    while (high - low > 1e-6) {
      if (xAt(s) < x) low = s;
      else high = s;
      s = (low + high) / 2;
    }
    return s;
  };

  return (t) => (t <= 0 ? 0 : t >= 1 ? 1 : yAt(solve(t)));
}

/** A duration as a computed custom property spells it, in milliseconds or seconds; 0 where it is none. */
export function parseDuration(value: string): number {
  const match = /^\s*(\d*\.?\d+)(ms|s)\s*$/.exec(value);
  if (!match) return 0;
  const amount = Number(match[1] ?? '');
  return match[2] === 's' ? amount * 1000 : amount;
}

/** A length in pixels as a computed custom property spells it; 0 where it is none. */
export function parseLength(value: string): number {
  const match = /^\s*(-?\d*\.?\d+)px\s*$/.exec(value);
  return match ? Number(match[1]) : 0;
}

/** An easing as a computed custom property spells it; linear for anything this module cannot read. */
export function parseEasing(value: string): Easing {
  const match = /^\s*cubic-bezier\(([^)]*)\)\s*$/.exec(value);
  const points = (match?.[1] ?? '').split(',').map((part) => Number(part.trim()));
  const [x1, y1, x2, y2] = points;
  if (points.length !== 4 || x1 === undefined || y1 === undefined || x2 === undefined || y2 === undefined) return linear;
  if (!points.every((point) => Number.isFinite(point))) return linear;
  return cubicBezier(x1, y1, x2, y2);
}

/** The delay of the item at `position` within one arrival, the step being `step` milliseconds. */
export function staggerDelay(position: number, step: number): number {
  return Math.min(Math.max(position, 0), STAGGERED - 1) * step;
}

/** A role's duration in milliseconds and its easing, both as the CSS spells them. */
export function roleOf(style: StyleSource, role: MotionRole): { duration: number; easing: string } {
  return {
    duration: parseDuration(style.getPropertyValue(`--motion-${role}-duration`)),
    easing: style.getPropertyValue(`--motion-${role}-easing`).trim(),
  };
}

/** Whether this node is somewhere motion was asked to stop: the system's setting, or the product's. */
export function isMotionReduced(node: Element): boolean {
  if (node.closest('[data-motion="reduced"]')) return true;
  return typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
}

// One arrival is everything made in the same task: a list rendered, a page of results appended.
// Positions count up within it and start again at the next, so an entry captured on its own
// arrives at once rather than after the rows that arrived before it. A node inside another that
// arrives with it (a `ListRow` in a table's row) does not move on its own: the outer one carries
// it, whichever of the two is attached first.
let arriving = new Map<Element, Animation>();
function nextPosition(): number {
  if (arriving.size === 0) queueMicrotask(() => (arriving = new Map()));
  return arriving.size;
}

const played = new WeakSet<Element>();

function carriedByAncestor(node: Element): boolean {
  for (let up = node.parentElement; up; up = up.parentElement) if (arriving.has(up)) return true;
  return false;
}

/**
 * Plays the entrance on `node` once: opacity and a rise of `--sp-100`, staggered within its
 * arrival. Nothing where motion is reduced, where the tokens are not loaded (a zero duration), or
 * where the node already arrived.
 */
export function playArrival(node: Element): void {
  if (played.has(node)) return;
  played.add(node);
  if (typeof node.animate !== 'function' || isMotionReduced(node) || carriedByAncestor(node)) return;
  const style = getComputedStyle(node);
  const { duration, easing } = roleOf(style, 'entrance');
  if (duration === 0) return;
  for (const [inner, animation] of arriving) {
    if (node.contains(inner)) {
      animation.cancel();
      arriving.delete(inner);
    }
  }
  const rise = parseLength(style.getPropertyValue('--sp-100'));
  const delay = staggerDelay(nextPosition(), parseDuration(style.getPropertyValue('--dur-stagger')));
  const animation = node.animate(
    [
      { opacity: 0, transform: `translateY(${rise}px)` },
      { opacity: 1, transform: 'none' },
    ],
    { duration, easing: easing || 'linear', delay, fill: 'backwards' },
  );
  arriving.set(node, animation);
}

/** The node itself arrives: for a component that is the row, such as `ListRow`. */
export const arrive: Attachment<Element> = (node) => {
  playArrival(node);
};

/**
 * Every element child of the node arrives, those it holds now and those added later: for a
 * component whose rows are its caller's snippet, such as `Table`'s body.
 */
export const arrivals: Attachment<Element> = (node) => {
  for (const child of Array.from(node.children)) playArrival(child);
  if (typeof MutationObserver !== 'function') return;
  const observer = new MutationObserver((records) => {
    for (const record of records) {
      for (const added of Array.from(record.addedNodes)) {
        if (added instanceof Element) playArrival(added);
      }
    }
  });
  observer.observe(node, { childList: true });
  return () => observer.disconnect();
};

let enterPosition = 0;

function rising(node: Element, role: MotionRole, stagger: boolean): TransitionConfig {
  if (isMotionReduced(node)) return { duration: 0 };
  const style = getComputedStyle(node);
  const { duration, easing } = roleOf(style, role);
  const rise = parseLength(style.getPropertyValue('--sp-100'));
  const delay = stagger ? staggerDelay(enterPosition++, parseDuration(style.getPropertyValue('--dur-stagger'))) : 0;
  if (stagger && enterPosition === 1) queueMicrotask(() => (enterPosition = 0));
  return {
    delay,
    duration,
    easing: parseEasing(easing),
    css: (t, u) => `opacity: ${t}; transform: translateY(${u * rise}px)`,
  };
}

/** An item arriving in a list its call site owns (`in:enter`), staggered within its arrival. */
export function enter(node: Element): TransitionConfig {
  return rising(node, 'entrance', true);
}

/** An item leaving its list (`out:leave`): it fades and falls the step it rose by. */
export function leave(node: Element): TransitionConfig {
  return rising(node, 'exit', false);
}

/** Content arriving in a section that opened (`in:reveal`); the section's size changes at once. */
export function reveal(node: Element): TransitionConfig {
  return rising(node, 'reveal', false);
}

/** A keyed item travelling to its new place (`animate:flip`), by transform alone. */
export function flip(node: Element, { from, to }: { from: DOMRect; to: DOMRect }): AnimationConfig {
  const dx = from.left - to.left;
  const dy = from.top - to.top;
  if ((dx === 0 && dy === 0) || isMotionReduced(node)) return { duration: 0 };
  const { duration, easing } = roleOf(getComputedStyle(node), 'reorder');
  return {
    duration,
    easing: parseEasing(easing),
    css: (_t, u) => `transform: translate(${u * dx}px, ${u * dy}px)`,
  };
}
