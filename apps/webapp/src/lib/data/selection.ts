// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * Which entries a reader has picked out, as arithmetic over the order they see.
 *
 * **The visible order is the only order.** A range runs from the anchor to the entry just clicked
 * *through what is on screen* — not through identifiers, not through creation time. That is what
 * makes shift-click mean what everybody expects it to mean, and it is why every function here
 * takes the visible list rather than holding one.
 *
 * **A selection never outlives what it points at.** A row that left the list — filtered away,
 * moved, trashed by somebody else — is dropped from the selection rather than kept, because a bar
 * saying "12 selected" over eleven visible rows is a bar that will send an operation about an entry
 * nobody can see.
 *
 * Pure, so the keyboard and the pointer paths are the same code and both are tested without a DOM.
 */

/** How many entries one bulk may carry, as the installation reports it. */
export function bulkCapOf(limits: Record<string, unknown> | undefined): number | undefined {
  const declared = limits?.['max_bulk_operations'];
  if (typeof declared !== 'number' || !Number.isFinite(declared) || declared <= 0) return undefined;
  return declared;
}

/** Whether one more may be picked. No cap known means yes, and the server refuses if it must. */
export function isWithinCap(count: number, cap: number | undefined): boolean {
  return cap === undefined || count <= cap;
}

/** Adds one, or takes it away. What a plain click and the space bar both do. */
export function toggle(selected: readonly string[], id: string): readonly string[] {
  return selected.includes(id) ? selected.filter((held) => held !== id) : [...selected, id];
}

/**
 * Everything from the anchor to this entry, in the order they are drawn, added to what is held.
 *
 * Added rather than replacing: a reader who picked three rows and then shift-clicked a fourth
 * meant to have four, not one range. An anchor that is no longer visible degenerates to a plain
 * toggle, which is the only honest answer when the other end of the range has gone.
 */
export function rangeTo(
  visible: readonly string[],
  selected: readonly string[],
  anchor: string | undefined,
  id: string,
): readonly string[] {
  const from = anchor === undefined ? -1 : visible.indexOf(anchor);
  const to = visible.indexOf(id);
  if (from < 0 || to < 0) return toggle(selected, id);

  const [start, end] = from <= to ? [from, to] : [to, from];
  const span = visible.slice(start, end + 1);
  return [...selected, ...span.filter((each) => !selected.includes(each))];
}

/** Everything on screen, or nothing — which is what one control that says both does. */
export function toggleAll(
  visible: readonly string[],
  selected: readonly string[],
): readonly string[] {
  const allHeld = visible.length > 0 && visible.every((id) => selected.includes(id));
  return allHeld ? selected.filter((id) => !visible.includes(id)) : [...new Set([...selected, ...visible])];
}

/** What is still on screen, in the order it is drawn. A selection is never wider than the list. */
export function stillVisible(
  visible: readonly string[],
  selected: readonly string[],
): readonly string[] {
  return visible.filter((id) => selected.includes(id));
}
