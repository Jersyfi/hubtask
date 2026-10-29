// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The passages inline comments stand at (UC-DOC-11 check 4): the words a thread was written about,
 * found again in the block it names and marked in the page, so that the margin can draw the thread
 * level with them.
 *
 * A comment keeps the block's identifier and the words, never a position: positions move with every
 * keystroke of every writer, the words and the block do not. A reader's editor has no block
 * identifiers (they live in the live document only), so a passage whose block is not found is
 * looked for in the whole page.
 */

import { Extension } from '@tiptap/core';
import type { Node as PMNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';

/** A thread's passage: the block it stands at and the words it was about. */
export interface Passage {
  readonly id: string;
  readonly anchor: string | null;
  readonly quote: string;
}

/** Where a passage stands in the document. */
export interface PassageRange {
  readonly from: number;
  readonly to: number;
}

/** The text of a node with the document position of every character. */
function indexed(node: PMNode, start: number): { text: string; at: number[] } {
  let text = '';
  const at: number[] = [];
  node.descendants((child, offset) => {
    if (!child.isText || !child.text) return true;
    for (let i = 0; i < child.text.length; i += 1) at.push(start + offset + i);
    text += child.text;
    return false;
  });
  return { text, at };
}

function within(node: PMNode, start: number, quote: string): PassageRange | undefined {
  const { text, at } = indexed(node, start);
  const index = text.indexOf(quote);
  if (index < 0) return undefined;
  const from = at[index];
  const last = at[index + quote.length - 1];
  return from === undefined || last === undefined ? undefined : { from, to: last + 1 };
}

/**
 * Finds a passage: the first occurrence of its words in the block it names, else in the whole page;
 * nothing when the words are gone.
 */
export function findPassage(doc: PMNode, passage: Passage): PassageRange | undefined {
  const quote = passage.quote.trim();
  if (quote === '') return undefined;
  let found: PassageRange | undefined;
  if (passage.anchor) {
    doc.forEach((node, offset) => {
      if (found || node.attrs['blockId'] !== passage.anchor) return;
      found = within(node, offset + 1, quote);
    });
  }
  return found ?? within(doc, 0, quote);
}

/**
 * The words selected inside one block, for a comment on exactly them; nothing for an empty
 * selection or one across blocks.
 */
export function selectedWords(doc: PMNode, from: number, to: number): string | undefined {
  if (from >= to) return undefined;
  const $from = doc.resolve(from);
  const $to = doc.resolve(to);
  if ($from.depth < 1 || $to.depth < 1 || $from.index(0) !== $to.index(0)) return undefined;
  const words = doc.textBetween(from, to, ' ').trim();
  return words === '' ? undefined : words;
}

interface PassageState {
  readonly passages: readonly Passage[];
  readonly ranges: ReadonlyMap<string, PassageRange>;
  readonly decorations: DecorationSet;
}

export const passagesKey = new PluginKey<PassageState>('commentPassages');

function locate(doc: PMNode, passages: readonly Passage[]): PassageState {
  const ranges = new Map<string, PassageRange>();
  const decorations: Decoration[] = [];
  for (const passage of passages) {
    const range = findPassage(doc, passage);
    if (!range) continue;
    ranges.set(passage.id, range);
    decorations.push(Decoration.inline(range.from, range.to, { class: 'page-passage', 'data-passage': passage.id }));
  }
  return { passages, ranges, decorations: DecorationSet.create(doc, decorations) };
}

/** Marks the passages of the page's open threads. Set them with `setPassages`. */
export const CommentPassages = Extension.create({
  name: 'commentPassages',
  addProseMirrorPlugins() {
    return [new Plugin<PassageState>({
      key: passagesKey,
      state: {
        init: (_config, state) => locate(state.doc, []),
        apply: (tr, value, _old, state) => {
          const next = tr.getMeta(passagesKey) as readonly Passage[] | undefined;
          if (next) return locate(state.doc, next);
          return tr.docChanged ? locate(state.doc, value.passages) : value;
        },
      },
      props: { decorations: (state) => passagesKey.getState(state)?.decorations },
    })];
  },
});
