// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What an unavailable control does when somebody tries it, and the one reason a group of controls
// shares (design-system.md §4, "A control that cannot be used now").
//
// An unavailable control stays focusable - `aria-disabled`, not the native attribute - so that a
// keyboard reaches it and hears why. The price is that the platform no longer stops it: a click
// still fires, `Enter` still activates, and a form still submits through its default button. The
// guard below is what stops it, and it has to stop it *first*: a caller's `onclick` is delegated
// to the document root by Svelte and a parent row listens on an ancestor, so the event is
// stopped in the capture phase on the control itself, before either can see it.
//
// `pointerdown` and `mousedown` are not stopped (unless a control changes on them, a range): a
// drag that starts on a row still starts on the control inside it, as it did before.

import { getContext, setContext } from 'svelte';

/** The keys that activate a button, a checkbox or a form's default button. */
const ACTIVATING = new Set(['Enter', ' ']);

/** The keys a control that changes on any key still lets through: leaving it, and closing. */
const PASSING = new Set(['Tab', 'Shift', 'Escape']);

export interface GuardOptions {
  /**
   * Whether every key but leaving and `Escape` is refused, for a control that changes on keys
   * without being activated - a range moves on the arrows.
   */
  readonly isEveryKeyRefused?: boolean;
  /** Whether a press is refused too, for a control that changes under the pointer - a range. */
  readonly isPressRefused?: boolean;
}

/**
 * Refuses every way of activating `targets` and calls `onTried` instead; returns the function that
 * lifts it. `targets` is the control and whatever forwards a click to it - its `label`.
 */
export function guard(
  targets: readonly (HTMLElement | null | undefined)[],
  onTried: () => void,
  { isEveryKeyRefused = false, isPressRefused = false }: GuardOptions = {},
): () => void {
  const refuse = (event: Event) => {
    event.preventDefault();
    event.stopImmediatePropagation();
  };
  const onClick = (event: Event) => {
    refuse(event);
    onTried();
  };
  const onKeydown = (event: KeyboardEvent) => {
    const passes = isEveryKeyRefused ? PASSING.has(event.key) : !ACTIVATING.has(event.key);
    if (passes) return;
    refuse(event);
    onTried();
  };

  const nodes = targets.filter((node): node is HTMLElement => node instanceof HTMLElement);
  for (const node of nodes) {
    node.addEventListener('click', onClick, { capture: true });
    node.addEventListener('keydown', onKeydown, { capture: true });
    if (isPressRefused) node.addEventListener('pointerdown', refuse, { capture: true });
  }
  return () => {
    for (const node of nodes) {
      node.removeEventListener('click', onClick, { capture: true });
      node.removeEventListener('keydown', onKeydown, { capture: true });
      node.removeEventListener('pointerdown', refuse, { capture: true });
    }
  };
}

/**
 * How long the bubble waits after the pointer left the control and the bubble before it hides:
 * long enough to cross the gap between them (WCAG 1.4.13, *hoverable*). Read from the `attach`
 * motion role at run time, so the figure stays in `tokens.json` (rule 15).
 */
export function graceOf(element: Element | null): number {
  if (!element) return 0;
  const value = getComputedStyle(element).getPropertyValue('--motion-attach-duration').trim();
  const amount = Number.parseFloat(value);
  if (Number.isNaN(amount)) return 0;
  return value.endsWith('ms') ? amount : amount * 1000;
}

/** The one reason a composite control states for all of its parts. */
export interface ReasonGroup {
  /** The id of the element the parts point their `aria-describedby` at. */
  readonly id: string;
  readonly reason: string | undefined;
}

const GROUP = Symbol('hubtask.reason-group');

/** Called by a composite while it initialises: its parts read it from their own. */
export function provideReasonGroup(group: () => ReasonGroup): void {
  setContext(GROUP, group);
}

/**
 * Called by a part while it initialises. Answers a function giving the group's id when the part is
 * unavailable for the group's own reason - then the part draws no bubble of its own and points at
 * the group's - and `undefined` otherwise, including for a part switched off for a reason of its
 * own, which it still states itself.
 */
export function sharedReason(): (reason: string | undefined) => string | undefined {
  const group = getContext<(() => ReasonGroup) | undefined>(GROUP);
  return (reason) => {
    if (!group || reason === undefined) return undefined;
    const current = group();
    return current.reason === reason ? current.id : undefined;
  };
}
