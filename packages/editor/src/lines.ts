// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a line's handle does (S-1): a line is a block of the page's top level - a paragraph, a
 * heading, a whole list or table. It is moved one place up or down, removed, or given an empty
 * line below it for "+". Each is a transaction on the state, so the live document carries it as
 * any typing; the handle's menu is the way that needs no dragging (WCAG 2.5.7).
 */

import { TextSelection, type EditorState, type Transaction } from '@tiptap/pm/state';

/** The start of the top-level block a position stands in, or nothing outside every block. */
export function lineStart(state: EditorState, pos: number): number | undefined {
  const at = Math.max(0, Math.min(pos, state.doc.content.size));
  const $pos = state.doc.resolve(at);
  if ($pos.depth >= 1) return $pos.before(1);
  // Between two blocks (an atom's edge): the block after, else the one before.
  const after = $pos.nodeAfter;
  if (after) return at;
  const before = $pos.nodeBefore;
  return before ? at - before.nodeSize : undefined;
}

/** Moves the line starting at `start` one place up (-1) or down (1); nothing at the page's edge. */
export function moveLine(state: EditorState, start: number, direction: -1 | 1): Transaction | null {
  const $start = state.doc.resolve(start);
  const index = $start.index(0);
  const node = state.doc.maybeChild(index);
  const other = state.doc.maybeChild(index + direction);
  if (!node || !other) return null;
  const tr = state.tr.delete(start, start + node.nodeSize);
  const to = direction < 0 ? start - other.nodeSize : start + other.nodeSize;
  tr.insert(to, node);
  return tr.setSelection(TextSelection.near(tr.doc.resolve(to + 1)));
}

/** Removes the line starting at `start`; the page keeps one empty line rather than none. */
export function removeLine(state: EditorState, start: number): Transaction | null {
  const node = state.doc.nodeAt(start);
  if (!node) return null;
  const tr = state.tr;
  if (state.doc.childCount === 1) {
    const empty = state.schema.nodes['paragraph']?.create();
    if (!empty) return null;
    tr.replaceWith(start, start + node.nodeSize, empty);
  } else {
    tr.delete(start, start + node.nodeSize);
  }
  return tr.setSelection(TextSelection.near(tr.doc.resolve(Math.min(start, tr.doc.content.size))));
}

/** An empty line below the line starting at `start`, the caret in it: "+" at the handle. */
export function lineBelow(state: EditorState, start: number): Transaction | null {
  const node = state.doc.nodeAt(start);
  const paragraph = state.schema.nodes['paragraph'];
  if (!node || !paragraph) return null;
  const at = start + node.nodeSize;
  const tr = state.tr.insert(at, paragraph.create());
  return tr.setSelection(TextSelection.create(tr.doc, at + 1));
}
