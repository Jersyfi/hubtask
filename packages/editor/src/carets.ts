// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A co-writer's name beside their caret never covers words. It stands above the caret where the
 * space above the line is free (a block's first line), below where the space below is free (its
 * last line), and is tucked away where neither is - the coloured caret stays. Near the reader's own
 * caret it fades, so it never stands over what the reader is writing.
 */

import { Plugin, PluginKey } from '@tiptap/pm/state';
import type { EditorView } from '@tiptap/pm/view';

export type CaretPlace = 'above' | 'below' | 'tucked';

interface Box { top: number; bottom: number; left: number; right: number }

/** Where the label goes: the free space it fits in, above first. */
export function caretLabelPlace(freeAbove: number, freeBelow: number, labelHeight: number): CaretPlace {
  if (freeAbove >= labelHeight) return 'above';
  if (freeBelow >= labelHeight) return 'below';
  return 'tucked';
}

/** Whether the label, standing on its side of the caret, would come within a line of the reader's caret. */
export function isNearOwnCaret(caret: Box, place: CaretPlace, labelWidth: number, lineHeight: number, own: Box | undefined): boolean {
  if (!own || place === 'tucked') return false;
  const top = place === 'above' ? caret.top - lineHeight : caret.top;
  const bottom = place === 'below' ? caret.bottom + lineHeight : caret.bottom;
  const overlapsLines = own.bottom > top && own.top < bottom;
  const overlapsWidth = own.right >= caret.left - lineHeight && own.left <= caret.left + labelWidth + lineHeight;
  return overlapsLines && overlapsWidth;
}

/** The free space between a block's line and its neighbours: the margin above its first line, below its last. */
function freeSpace(caret: DOMRect, block: Element): { above: number; below: number } {
  const own = block.getBoundingClientRect();
  const lineHeight = Math.max(1, caret.height);
  const isFirstLine = caret.top - own.top < lineHeight / 2;
  const isLastLine = own.bottom - caret.bottom < lineHeight / 2;
  const before = block.previousElementSibling?.getBoundingClientRect();
  const after = block.nextElementSibling?.getBoundingClientRect();
  return {
    above: isFirstLine ? caret.top - (before ? before.bottom : own.top - lineHeight) : 0,
    below: isLastLine ? (after ? after.top : own.bottom + lineHeight) - caret.bottom : 0,
  };
}

function place(view: EditorView) {
  const carets = view.dom.querySelectorAll<HTMLElement>('.page-caret');
  if (carets.length === 0) return;
  const { from } = view.state.selection;
  let own: Box | undefined;
  try {
    own = view.hasFocus() ? view.coordsAtPos(from) : undefined;
  } catch {
    own = undefined;
  }
  for (const caret of carets) {
    const label = caret.querySelector<HTMLElement>('.page-caret__label');
    const block = caret.closest('.page-body > *, li, td, th');
    if (!label || !block) continue;
    const box = caret.getBoundingClientRect();
    const size = label.getBoundingClientRect();
    const free = freeSpace(box, block);
    const where = caretLabelPlace(free.above, free.below, size.height);
    caret.dataset['place'] = where;
    if (isNearOwnCaret(box, where, size.width, box.height, own)) caret.dataset['near'] = '';
    else delete caret.dataset['near'];
  }
}

/** Places each co-writer's label after every change of the document, a selection or a caret. */
export const caretLabels = new Plugin({
  key: new PluginKey('caretLabels'),
  view: (view) => {
    place(view);
    return { update: (current) => place(current) };
  },
});
