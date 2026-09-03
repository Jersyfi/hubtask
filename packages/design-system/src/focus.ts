// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What the keyboard does in an overlay, kept out of the DOM so that it can be tested.
//
// Same reasoning as `layers.ts`, and for the same acceptance criteria. "A menu is fully operable
// from the keyboard" and "focus returns to the trigger when a dialog closes" are checked here as
// arithmetic and as one branch, rather than by opening a menu and pressing keys - which is a check
// that needs a driven browser (ADR-0037 left that to F5) and would otherwise be no check at all.
//
// The components keep the DOM half, and it stays small: read the items, call `rovingIndex`, focus
// what it names.

import { handleEscape, layers, type LayerRegister } from './layers.ts';

/** Which arrows move the selection. A menu is vertical; a toolbar, later, is not. */
export type Orientation = 'vertical' | 'horizontal';

export interface RovingOptions {
  readonly orientation?: Orientation;
  /** The document's direction. In RTL the arrow that means "next" is the one pointing left. */
  readonly dir?: 'ltr' | 'rtl';
}

/**
 * Where focus goes for a key, or `null` when the key is not one this list answers.
 *
 * Two behaviours are deliberate. The list **wraps**: `ArrowDown` on the last item returns to the
 * first, which is what the ARIA authoring practices specify for a menu and what makes a short list
 * usable without looking. And `current` of `-1` means "nothing is focused yet", so the first
 * `ArrowDown` after the menu opens lands on the first item rather than on the second.
 */
export function rovingIndex(
  key: string,
  current: number,
  count: number,
  { orientation = 'vertical', dir = 'ltr' }: RovingOptions = {},
): number | null {
  if (count <= 0) return null;

  const forward = orientation === 'vertical' ? 'ArrowDown' : dir === 'rtl' ? 'ArrowLeft' : 'ArrowRight';
  const backward = orientation === 'vertical' ? 'ArrowUp' : dir === 'rtl' ? 'ArrowRight' : 'ArrowLeft';

  if (key === forward) return current < 0 ? 0 : (current + 1) % count;
  if (key === backward) return current < 0 ? count - 1 : (current - 1 + count) % count;
  if (key === 'Home') return 0;
  if (key === 'End') return count - 1;
  return null;
}

/**
 * The first item whose label starts with the character typed. Type-ahead is not decoration: a menu
 * of fifteen items is unusable with arrows alone, and the practice is old enough that people try it
 * without being told. The search starts *after* the current item so that repeating a letter walks
 * through the items that share it.
 */
export function typeAheadIndex(character: string, current: number, labels: readonly string[]): number | null {
  const needle = character.toLowerCase();
  if (needle.length !== 1 || !/\S/.test(needle)) return null;
  for (let step = 1; step <= labels.length; step += 1) {
    const index = (Math.max(current, -1) + step) % labels.length;
    if (labels[index]?.trim().toLowerCase().startsWith(needle)) return index;
  }
  return null;
}

/**
 * The `Escape` contract, as one handler every overlay installs.
 *
 * The guard is the whole point. Two open layers means two listeners, and both see the same key; if
 * each called the register, one press would close both and the rule `layers.ts` exists to keep -
 * `Escape` closes exactly one - would be broken by the components that depend on it. The first
 * handler to act marks the event, and the rest stand down.
 */
export function escapeHandler(register: LayerRegister = layers): (event: KeyboardEvent) => void {
  return (event) => {
    if (event.defaultPrevented) return;
    if (handleEscape(event.key, register)) event.preventDefault();
  };
}

/**
 * Puts focus back where it was before an overlay took it, and says whether it could.
 *
 * The branch is the reason this is a function rather than a line: the trigger can be gone by the
 * time the overlay closes - a menu item that deleted the row it sat in is the ordinary case - and
 * focusing a detached element silently moves focus to the document body, which is how a keyboard
 * user ends up at the top of the page with no idea why. A caller that gets `false` decides where
 * focus goes instead.
 */
export function focusReturn(previous: Element | null | undefined): boolean {
  if (!previous || !previous.isConnected) return false;
  const focusable = previous as HTMLElement;
  if (typeof focusable.focus !== 'function') return false;
  focusable.focus();
  return true;
}

/**
 * What can be focused inside a container. `Dialog` needs none of this - a modal `<dialog>` is
 * trapped by the browser - but a popover is not modal, and the first thing it does on open is
 * move focus to what it holds.
 */
const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

export function focusables(root: ParentNode): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
    (element) => element.getAttribute('aria-hidden') !== 'true',
  );
}
