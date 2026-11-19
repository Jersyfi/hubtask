// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An empty page's text explains itself (S-6, scene pnew 3): while the document holds only empty
// paragraphs, the first carries the words of what to do, drawn by the page's sheet from
// `data-placeholder`. A decoration, never content: nothing of it is saved or sent to co-writers.

import type { Node as PmNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';

export const placeholderKey = new PluginKey('pagePlaceholder');

/**
 * Whether a document is nothing but empty paragraphs: a live document seeded from an empty body
 * may hold more than one.
 */
export function isEmptyDocument(doc: PmNode): boolean {
  let empty = doc.childCount > 0;
  doc.forEach((block) => {
    if (block.type.name !== 'paragraph' || block.content.size > 0) empty = false;
  });
  return empty;
}

/** The plugin that marks an empty document's paragraph with the words. */
export function placeholder(words: string): Plugin {
  return new Plugin({
    key: placeholderKey,
    props: {
      decorations: (state) => {
        if (!words || !isEmptyDocument(state.doc)) return DecorationSet.empty;
        const node = state.doc.firstChild!;
        return DecorationSet.create(state.doc, [
          Decoration.node(0, node.nodeSize, { class: 'page-empty', 'data-placeholder': words }),
        ]);
      },
    },
  });
}
