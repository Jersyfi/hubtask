// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The arithmetic of a rank change, out where it can be tested.
//
// The same reasoning `focus.ts`, `layers.ts` and `structure.ts` record: "which position does
// Alt+Down mean on the fourth of six rows" and "which slot is the pointer over" are questions about
// a list, and a component that answered them inline could only be checked by driving a browser.
// Here they are functions over numbers, and `reorder.test.js` runs them under `node --test`.
//
// **The command comes first and the gesture is measured against it.** WCAG 2.2 SC 2.5.7 requires a
// single-pointer alternative to every dragging movement, and a rank change is a command before it
// is a gesture: `rankTarget` is what a menu item calls, `rankIntent` is the same answer for a key,
// and `dropIndex` is the pointer arriving at one of the positions those two already name - so
// the two paths cannot come to disagree about where the fourth row lands.

/** The four ways to say where an entry should go, as a command rather than as a gesture. */
export type RankCommand = 'up' | 'down' | 'top' | 'bottom';

/**
 * The position a command asks for, or `null` when it asks for the one the entry already holds.
 *
 * `null` rather than a clamped index, because "already first" is a different answer from "move to
 * the first position" and only one of them is worth a request. It is also what a menu shows a
 * reason for instead of a control that does nothing.
 */
export function rankTarget(command: RankCommand, index: number, count: number): number | null {
  if (index < 0 || index >= count) return null;

  const to =
    command === 'up' ? index - 1 : command === 'down' ? index + 1 : command === 'top' ? 0 : count - 1;

  if (to < 0 || to >= count || to === index) return null;
  return to;
}

/** What a key event has to say for this module to read it. A subset, so a test needs no DOM. */
export interface RankKey {
  readonly key: string;
  readonly altKey: boolean;
  readonly shiftKey: boolean;
  readonly ctrlKey?: boolean;
  readonly metaKey?: boolean;
}

/**
 * The command a key press means on a focused row, or `null` for every other press.
 *
 * **Alt and the vertical arrows**, and the choice is about what the keys already do. `Alt+Left` and
 * `Alt+Right` are the browser's back and forward, and `Alt+Home` is a home page in Gecko - so a
 * shortcut on any of them is a shortcut that sometimes navigates away from the list instead of
 * ranking in it. The vertical pair is free in every engine ADR-0044's row names, and `Shift` widens
 * one step into the whole level rather than introducing a third chord.
 *
 * `Ctrl` and `Meta` are refused rather than ignored: they are the modifiers a platform builds its
 * own shortcuts out of, and a handler that answered `Ctrl+Alt+Down` would be answering somebody
 * else's key.
 */
export function rankIntent(event: RankKey, index: number, count: number): number | null {
  if (!event.altKey || event.ctrlKey || event.metaKey) return null;

  const command: RankCommand | null =
    event.key === 'ArrowUp'
      ? event.shiftKey
        ? 'top'
        : 'up'
      : event.key === 'ArrowDown'
        ? event.shiftKey
          ? 'bottom'
          : 'down'
        : null;

  return command === null ? null : rankTarget(command, index, count);
}

/** One row's extent along the axis the list runs in. Two numbers, so a test needs no rectangles. */
export interface Extent {
  readonly start: number;
  readonly end: number;
}

/**
 * The lines between neighbouring rows: one fewer than there are rows.
 *
 * The **middle of the gap** rather than the top of the next row, because the gap belongs to
 * neither: a pointer resting in the space between two rows would otherwise already count as being
 * on the lower one, and the drop marker would jump a row early on any list with air in it.
 */
export function boundariesOf(extents: readonly Extent[]): number[] {
  const lines: number[] = [];
  for (let index = 0; index + 1 < extents.length; index += 1) {
    lines.push((extents[index]!.end + extents[index + 1]!.start) / 2);
  }
  return lines;
}

/**
 * The position a pointer is over: how many boundaries it has passed.
 *
 * The result is an index into the list **as it is drawn**, which is the same number `rankTarget`
 * answers with - so a drag that ends here and a menu item that names the same position produce one
 * call rather than two shapes of one.
 *
 * Above the first row and below the last are the ends rather than nothing: a reader dragging past
 * the edge of a list means the end of it, and a drag that answered `null` there would refuse the
 * one position that is hardest to hit exactly.
 */
export function dropIndex(along: number, boundaries: readonly number[]): number {
  let passed = 0;
  while (passed < boundaries.length && along > boundaries[passed]!) passed += 1;
  return passed;
}

/**
 * How far a pointer travels before a press becomes a drag.
 *
 * Not zero, and that is the whole reason it is a constant with a name: a row is a link, and a
 * pointer that moves two pixels between `pointerdown` and `pointerup` is somebody clicking it. A
 * drag that began at the first pixel would make a list of links unopenable with a shaky hand or a
 * trackpad.
 */
export const DRAG_THRESHOLD_PX = 6;

export function hasLeftTheHandle(
  from: { readonly x: number; readonly y: number },
  to: { readonly x: number; readonly y: number },
  threshold = DRAG_THRESHOLD_PX,
): boolean {
  return Math.hypot(to.x - from.x, to.y - from.y) >= threshold;
}
