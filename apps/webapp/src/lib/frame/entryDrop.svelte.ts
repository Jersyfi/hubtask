// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * An entry carried from a list or a board onto a collection in the sidebar (UC-WRK-06 check 8,
 * the navigation concept's §5.4): the list's drag publishes what it carries and which sidebar row
 * is under the pointer; the sidebar outlines that row and, on the drop, takes the move. A row is a
 * target where an element of `[data-entry-target]` holds it as `[data-id]`.
 */

class Carried {
  /** The entry being carried, or null. */
  id = $state<string | null>(null);
  /** The sidebar row under the pointer, as its `data-id`. */
  over = $state<string | undefined>(undefined);
}

export const carried = new Carried();

/** The sidebar row a point is over, through whatever is drawn above it (the carried card). */
export function targetAt(x: number, y: number): string | undefined {
  if (typeof document === 'undefined') return undefined;
  for (const element of document.elementsFromPoint(x, y)) {
    const row = element.closest<HTMLElement>('[data-entry-target] [data-id]');
    if (row?.dataset['id']) return row.dataset['id'];
  }
  return undefined;
}

type Take = (rowId: string, entryId: string) => boolean;
let take: Take | undefined;

export const entryTarget = {
  /** The sidebar names what a drop on one of its rows does; answers its own removal. */
  set(fn: Take): () => void {
    take = fn;
    return () => {
      if (take === fn) take = undefined;
    };
  },
  /** A drop on a row: whether the sidebar took it, so the list does not also place the entry. */
  drop(rowId: string, entryId: string): boolean {
    return take?.(rowId, entryId) ?? false;
  },
};
