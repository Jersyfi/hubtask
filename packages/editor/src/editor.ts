// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The page editor: TipTap's MIT core with the blocks a page's Markdown carries, a reference inline
 * (a mention) and as a block (a card or a live view), an identifier on every block an inline comment
 * can stand at, and - for a writer - the live document and the other writers' carets.
 *
 * **No style of its own.** The editor's CSS injection is off (the content security policy refuses an
 * inline style element); every class it draws is styled by the application with the design tokens
 * (rule 15). A caret's colour is a class (`page-caret--0` … `--7`), not a value.
 */

import { Editor, Extension, Node, mergeAttributes, type JSONContent } from '@tiptap/core';
import Collaboration from '@tiptap/extension-collaboration';
import CollaborationCaret from '@tiptap/extension-collaboration-caret';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import StarterKit from '@tiptap/starter-kit';
import type * as Y from 'yjs';

import type { LiveDocument } from './live.ts';
import { fromMarkdown, toMarkdown, type DocNode, type ReferenceAttrs } from './markdown.ts';
import { CommentPassages, passagesKey, selectedWords, type Passage } from './passages.ts';
import { MIRROR_META, linkLine, mirrorTicks, taskLineAt, tickedEntries, type TaskLine } from './tasks.ts';

/** How the application draws a reference: its label, kept current as the resolver answers. */
export interface ReferenceDrawing {
  /** The words a reference shows, or nothing while it is unresolved or out of the reader's reach. */
  label(ref: ReferenceAttrs): string | undefined;
  /** Called when a reference is clicked, to open what it points at. */
  open?(ref: ReferenceAttrs): void;
  /** Subscribes to label changes; answers the unsubscribe. */
  subscribe(listener: () => void): () => void;
}

const referenceAttributes = {
  kind: { default: 'entry' },
  id: { default: '' },
  anchor: { default: null },
};

function drawReference(dom: HTMLElement, ref: ReferenceAttrs, drawing: ReferenceDrawing | undefined, fallback: string) {
  dom.textContent = drawing?.label(ref) ?? fallback;
  dom.dataset['kind'] = ref.kind;
  dom.dataset['id'] = ref.id;
}

function referenceView(className: string, tag: string, fallback: string) {
  return ({ node, extension }: { node: { attrs: Record<string, unknown> }; extension: { options: { drawing?: ReferenceDrawing } } }) => {
    const dom = document.createElement(tag);
    dom.className = className;
    dom.setAttribute('contenteditable', 'false');
    const ref = { kind: String(node.attrs['kind']), id: String(node.attrs['id']), anchor: (node.attrs['anchor'] as string | null) ?? null };
    const drawing = extension.options.drawing;
    drawReference(dom, ref, drawing, fallback);
    const stop = drawing?.subscribe(() => drawReference(dom, ref, drawing, fallback));
    dom.addEventListener('click', () => drawing?.open?.(ref));
    return { dom, destroy: () => stop?.() };
  };
}

/** A mention: `[](hubtask:<kind>/<id>)` inside a line. */
export const Reference = Node.create<{ drawing?: ReferenceDrawing; fallback: string }>({
  name: 'reference',
  group: 'inline',
  inline: true,
  atom: true,
  selectable: true,
  addOptions: () => ({ drawing: undefined, fallback: '…' }),
  addAttributes: () => referenceAttributes,
  parseHTML: () => [{ tag: 'span[data-reference]' }],
  renderHTML: ({ HTMLAttributes }) => ['span', mergeAttributes({ 'data-reference': '' }, HTMLAttributes)],
  addNodeView() {
    return referenceView('page-mention', 'span', this.options.fallback) as never;
  },
});

/** A card or a live view: `![](hubtask:<kind>/<id>)` on a line of its own. */
export const ReferenceBlock = Node.create<{ drawing?: ReferenceDrawing; fallback: string }>({
  name: 'referenceBlock',
  group: 'block',
  atom: true,
  selectable: true,
  draggable: true,
  addOptions: () => ({ drawing: undefined, fallback: '…' }),
  addAttributes: () => referenceAttributes,
  parseHTML: () => [{ tag: 'div[data-reference-block]' }],
  renderHTML: ({ HTMLAttributes }) => ['div', mergeAttributes({ 'data-reference-block': '' }, HTMLAttributes)],
  addNodeView() {
    return referenceView('page-card', 'div', this.options.fallback) as never;
  },
});

const BLOCK_TYPES = ['paragraph', 'heading', 'blockquote', 'codeBlock', 'bulletList', 'orderedList', 'taskList', 'referenceBlock'];

/**
 * Every top block carries an identifier an inline comment stands at (`PageCommentCreate.anchor`).
 * It lives in the live document only - the Markdown has no place for it - and a block that has
 * none is given one by the first writer who touches the document.
 */
export const BlockIds = Extension.create<{ mint: () => string }>({
  name: 'blockIds',
  addOptions: () => ({ mint: () => Math.random().toString(36).slice(2, 12) }),
  addGlobalAttributes() {
    return [{
      types: BLOCK_TYPES,
      attributes: {
        blockId: {
          default: null,
          keepOnSplit: false,
          parseHTML: (element: HTMLElement) => element.getAttribute('data-block'),
          renderHTML: (attributes: Record<string, unknown>) => (attributes['blockId'] ? { 'data-block': attributes['blockId'] } : {}),
        },
      },
    }];
  },
  addProseMirrorPlugins() {
    const mint = this.options.mint;
    return [new Plugin({
      key: new PluginKey('blockIds'),
      appendTransaction: (transactions, _old, state) => {
        if (!transactions.some((each) => each.docChanged) || !this.editor.isEditable) return null;
        const seen = new Set<string>();
        let tr = state.tr;
        let changed = false;
        state.doc.forEach((node, offset) => {
          if (!BLOCK_TYPES.includes(node.type.name)) return;
          const id = node.attrs['blockId'] as string | null;
          if (id && !seen.has(id)) {
            seen.add(id);
            return;
          }
          const fresh = mint();
          seen.add(fresh);
          tr = tr.setNodeMarkup(offset, undefined, { ...node.attrs, blockId: fresh });
          changed = true;
        });
        return changed ? tr.setMeta('addToHistory', false) : null;
      },
    })];
  },
});

/** The block the selection stands in, and the words selected inside it. */
export interface SelectedBlock {
  readonly id: string;
  readonly text: string;
  readonly selected?: string;
}

export interface PageEditorOptions {
  readonly element: HTMLElement;
  /** The page's body, the content of a read-only editor and the seed of an empty live document. */
  readonly body: string;
  readonly editable: boolean;
  /** The live document, for a writer; a reader's editor draws the body alone. */
  readonly live?: { readonly doc: Y.Doc; readonly connection: LiveDocument; readonly user: { name: string; caret: number } };
  readonly drawing?: ReferenceDrawing;
  /** Called after a local change, with the document's Markdown. */
  readonly onChange?: (markdown: string) => void;
  /**
   * Called when the selection moves, with the block it stands in and the words selected in it -
   * what an inline comment stands at and quotes.
   */
  readonly onBlock?: (block: SelectedBlock | undefined) => void;
  /** The words drawn for a reference before it is resolved. */
  readonly fallback?: string;
  /** Called when the person ticks or unticks a line linked to an entry (UC-DOC-03). */
  readonly onTick?: (entryId: string, checked: boolean) => void;
}

/** Builds the editor. The caller destroys it with the screen. */
export function createPageEditor(options: PageEditorOptions): Editor {
  const fallback = options.fallback ?? '…';
  const extensions = [
    StarterKit.configure({
      // The live document keeps its own history; a second one would undo another writer's typing.
      undoRedo: options.live ? false : {},
      // Markdown has no underline; a page that drew one would lose it on the next save.
      underline: false,
      link: { openOnClick: false, autolink: true, protocols: ['hubtask'] },
    }),
    TaskList,
    TaskItem.configure({ nested: true }),
    Reference.configure({ drawing: options.drawing, fallback }),
    ReferenceBlock.configure({ drawing: options.drawing, fallback }),
    BlockIds,
    CommentPassages,
  ];
  if (options.live) {
    extensions.push(Collaboration.configure({ document: options.live.doc, field: 'default' }) as never);
    extensions.push(CollaborationCaret.configure({
      provider: options.live.connection,
      user: { name: options.live.user.name, caret: options.live.user.caret },
      render: (user: Record<string, unknown>) => {
        const caret = document.createElement('span');
        caret.className = `page-caret page-caret--${Number(user['caret'] ?? 0) % 8}`;
        const label = document.createElement('span');
        label.className = 'page-caret__label';
        label.textContent = String(user['name'] ?? '');
        caret.append(label);
        return caret;
      },
    }) as never);
  }
  const editor = new Editor({
    element: options.element,
    injectCSS: false,
    editable: options.editable,
    extensions,
    // A live document brings its own content; seeding it is `seedFromMarkdown`'s.
    ...(options.live ? {} : { content: fromMarkdown(options.body) as JSONContent }),
    editorProps: { attributes: { class: 'page-body', 'aria-multiline': 'true', role: 'textbox' } },
    onUpdate: ({ editor: current, transaction }) => {
      if (transaction.getMeta('y-sync$')?.isChangeOrigin) return;
      if (!transaction.getMeta(MIRROR_META)) {
        for (const tick of tickedEntries(transaction.before, current.state.doc)) options.onTick?.(tick.entryId, tick.checked);
      }
      options.onChange?.(toMarkdown(current.getJSON() as DocNode));
    },
    onSelectionUpdate: ({ editor: current }) => {
      const { $from, from, to } = current.state.selection;
      const top = $from.depth > 0 ? $from.node(1) : undefined;
      const id = top?.attrs['blockId'] as string | null | undefined;
      const selected = selectedWords(current.state.doc, from, to)?.slice(0, 500);
      options.onBlock?.(top && id ? { id, text: top.textContent.slice(0, 500), ...(selected ? { selected } : {}) } : undefined);
    },
  });
  return editor;
}

/**
 * Fills an empty live document from the page's body: a page written through the API, imported,
 * or made from a template has a body and no live document yet. Only a writer seeds, and only once
 * the server's state has arrived, so that a document with content is never seeded twice.
 */
export function seedFromMarkdown(editor: Editor, body: string): boolean {
  if (!editor.isEditable || body.trim() === '' || !editor.isEmpty) return false;
  editor.commands.setContent(fromMarkdown(body) as JSONContent);
  return true;
}

/** The document's Markdown, as the page saves it. */
export function markdownOf(editor: Editor): string {
  return toMarkdown(editor.getJSON() as DocNode);
}

/** Inserts a reference at the selection: a mention inline, a card or a live view as a block. */
export function insertReference(editor: Editor, ref: ReferenceAttrs, inPlace: boolean): void {
  const node = { type: inPlace ? 'referenceBlock' : 'reference', attrs: { ...ref } };
  editor.chain().focus().insertContent(node).run();
}

/** Scrolls to the block an inline comment stands at, and marks it for a moment. */
export function revealBlock(editor: Editor, blockId: string): boolean {
  const element = editor.view.dom.querySelector(`[data-block="${CSS.escape(blockId)}"]`);
  if (!element) return false;
  element.scrollIntoView({ block: 'center' });
  return true;
}

/** The checklist line the selection stands in, for turning it into a task. */
export function taskLineOf(editor: Editor): TaskLine | undefined {
  return taskLineAt(editor.state.doc, editor.state.selection.from);
}

/** Replaces a line's words by a mention of the entry they became (UC-DOC-03). */
export function linkLineToEntry(editor: Editor, line: TaskLine, entryId: string): boolean {
  const tr = linkLine(editor.state, line, entryId);
  if (!tr) return false;
  editor.view.dispatch(tr);
  return true;
}

/** Ticks the lines linked to entries as the entries stand. */
export function mirrorCompletion(editor: Editor, completed: ReadonlyMap<string, boolean>): void {
  if (editor.isDestroyed) return;
  const tr = mirrorTicks(editor.state, completed);
  if (tr) editor.view.dispatch(tr);
}

/** Marks the passages of the page's open threads in the page. */
export function setPassages(editor: Editor, passages: readonly Passage[]): void {
  if (editor.isDestroyed) return;
  editor.view.dispatch(editor.state.tr.setMeta(passagesKey, passages).setMeta('addToHistory', false));
}

/**
 * Where each marked passage starts, as a distance from the top of the viewport - the margin draws
 * its thread level with it. A passage whose words are gone has none.
 */
export function passageTops(editor: Editor): Map<string, number> {
  const tops = new Map<string, number>();
  if (editor.isDestroyed) return tops;
  const ranges = passagesKey.getState(editor.state)?.ranges;
  for (const [id, range] of ranges ?? []) tops.set(id, editor.view.coordsAtPos(range.from).top);
  return tops;
}

export type { Editor };
