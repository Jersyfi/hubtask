// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A checklist line that became a task (UC-DOC-03): the line holds a mention of the entry, so it
 * reads as the entry's title wherever the entry is renamed, and its tick mirrors the entry's
 * completion both ways - the page draws what the entry says, and ticking the line completes or
 * reopens the entry.
 *
 * The link is the mention itself, never a second record: the page's references (`page_reference`)
 * already say which entries a page names, and the entry's backlinks name the page.
 */

import type { Node as PMNode } from '@tiptap/pm/model';
import type { EditorState, Transaction } from '@tiptap/pm/state';

/** A checklist line: where it stands, its words, its tick and the entry it is linked to. */
export interface TaskLine {
  readonly pos: number;
  readonly text: string;
  readonly checked: boolean;
  readonly entryId?: string;
}

/** The meta a transaction carries when it only mirrors an entry's completion. */
export const MIRROR_META = 'hubtaskMirrorTicks';

function entryIn(item: PMNode): string | undefined {
  let found: string | undefined;
  item.forEach((child) => {
    if (found || child.type.name !== 'paragraph') return;
    child.forEach((inline) => {
      if (!found && inline.type.name === 'reference' && String(inline.attrs['kind']).toLowerCase() === 'entry') {
        found = String(inline.attrs['id']).toLowerCase();
      }
    });
  });
  return found;
}

function lineOf(item: PMNode, pos: number): TaskLine {
  const entryId = entryIn(item);
  return {
    pos, text: item.firstChild?.textContent.trim() ?? '', checked: Boolean(item.attrs['checked']),
    ...(entryId ? { entryId } : {}),
  };
}

/** The checklist line a position stands in, if any. */
export function taskLineAt(doc: PMNode, at: number): TaskLine | undefined {
  const $at = doc.resolve(at);
  for (let depth = $at.depth; depth > 0; depth -= 1) {
    const node = $at.node(depth);
    if (node.type.name === 'taskItem') return lineOf(node, $at.before(depth));
  }
  return undefined;
}

/** Every checklist line of the page. */
export function taskLines(doc: PMNode): TaskLine[] {
  const lines: TaskLine[] = [];
  doc.descendants((node, pos) => {
    if (node.type.name === 'taskItem') lines.push(lineOf(node, pos));
    return true;
  });
  return lines;
}

/**
 * Replaces a line's words by a mention of the entry they became; nothing when the position is
 * not a checklist line.
 */
export function linkLine(state: EditorState, line: TaskLine, entryId: string): Transaction | undefined {
  const item = state.doc.nodeAt(line.pos);
  const paragraph = item?.firstChild;
  const reference = state.schema.nodes['reference'];
  if (!item || item.type.name !== 'taskItem' || !paragraph || paragraph.type.name !== 'paragraph' || !reference) return undefined;
  const from = line.pos + 2;
  const to = from + paragraph.content.size;
  return state.tr.replaceWith(from, to, reference.create({ kind: 'entry', id: entryId, anchor: null }));
}

/** Ticks the lines linked to entries as the entries stand; nothing when every tick already agrees. */
export function mirrorTicks(state: EditorState, completed: ReadonlyMap<string, boolean>): Transaction | undefined {
  let tr: Transaction | undefined;
  for (const line of taskLines(state.doc)) {
    if (!line.entryId) continue;
    const done = completed.get(line.entryId);
    if (done === undefined || done === line.checked) continue;
    tr ??= state.tr;
    const item = state.doc.nodeAt(line.pos);
    if (item) tr.setNodeMarkup(line.pos, undefined, { ...item.attrs, checked: done });
  }
  return tr?.setMeta(MIRROR_META, true).setMeta('addToHistory', false);
}

/** The linked lines a change ticked or unticked, by entry - what the person asked of the entries. */
export function tickedEntries(before: PMNode, after: PMNode): { entryId: string; checked: boolean }[] {
  const was = new Map<string, boolean>();
  for (const line of taskLines(before)) if (line.entryId) was.set(line.entryId, line.checked);
  const out: { entryId: string; checked: boolean }[] = [];
  for (const line of taskLines(after)) {
    if (!line.entryId) continue;
    const previous = was.get(line.entryId);
    if (previous !== undefined && previous !== line.checked) out.push({ entryId: line.entryId, checked: line.checked });
  }
  return out;
}
