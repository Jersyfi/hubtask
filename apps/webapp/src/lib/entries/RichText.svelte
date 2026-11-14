<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  /** What the formatting bar offers (E-3): the pages' editor, kept to what a description or a comment needs. */
  export const FORMATS = ['bold', 'italic', 'strike', 'code', 'link', 'list', 'checklist', 'quote'] as const;
  export type Format = (typeof FORMATS)[number];
</script>

<script lang="ts">
  // An entry's description or a comment in the pages' editor (E-3): read as formatted text, written
  // in place with the marks a page has, and still stored as Markdown. The editor is loaded on first
  // use, so a screen that never opens an entry carries none of it.
  import { flushSync, onMount, untrack } from 'svelte';
  import type { Editor, ReferenceAttrs, ReferenceDrawing } from '@hubtask/editor';
  import type { ReferenceTarget } from '@hubtask/sync-engine';

  import { Skeleton } from '@hubtask/design-system/components';

  import { engine } from '../data/engine.ts';
  import { referencesIn, resolveRequest, targetLabel } from '../data/links.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The Markdown shown and edited. */
    value: string;
    /** The text box's accessible name. */
    label: string;
    placeholder?: string;
    editable?: boolean;
    lang?: string;
    /** Each local change, as Markdown. */
    onchange?: (markdown: string) => void;
    /** The writer left the text: the moment to save. */
    oncommit?: () => void;
    /** Escape: the writer throws the change away. */
    onescape?: () => void;
    onnavigate?: (path: string) => void;
  }

  const { value, label, placeholder, editable = false, lang, onchange, oncommit, onescape, onnavigate }: Props = $props();

  let host = $state<HTMLElement | null>(null);
  let editor: Editor | undefined;
  let library: typeof import('@hubtask/editor') | undefined;
  /** What the editor holds, so a value coming back from the server is not drawn over the typing. */
  let current = untrack(() => value);
  let isFocused = $state(false);
  let isEmpty = $state(untrack(() => value.trim() === ''));
  /** The editor has drawn the text. Before, a text that is there is a skeleton - never a blank box. */
  let isReady = $state(false);

  const targets = new Map<string, ReferenceTarget>();
  const listeners = new Set<() => void>();
  const keyOf = (ref: { kind: string; id: string }) => `${ref.kind.toUpperCase()}/${ref.id.toLowerCase()}`;
  const drawing = {
    label: (ref: ReferenceAttrs) => {
      const target = targets.get(keyOf(ref));
      return target ? (targetLabel(target) ?? t('app.pages.reference_unavailable')) : undefined;
    },
    open: (ref: ReferenceAttrs) => {
      const kind = ref.kind.toUpperCase();
      if (kind === 'ENTRY') onnavigate?.(`/items/${ref.id}`);
      else if (kind === 'PAGE') onnavigate?.(`/pages/${ref.id}`);
      else if (kind === 'CI') onnavigate?.(`/cis/${ref.id}`);
    },
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  } as ReferenceDrawing;

  async function resolve(markdown: string) {
    const found = referencesIn(markdown).map((each) => each.reference);
    if (found.length === 0) return;
    const state = await engine.refresh<{ data: ReferenceTarget[] }>(resolveRequest(found));
    if (state.status !== 'ready') return;
    for (const target of state.data.data) targets.set(keyOf(target), target);
    for (const listener of listeners) listener();
  }

  onMount(() => {
    let isGone = false;
    void import('@hubtask/editor').then((lib) => {
      if (isGone || !host) return;
      library = lib;
      const made = lib.createPageEditor({
        element: host,
        body: current,
        editable,
        drawing,
        label,
        fallback: t('app.pages.reference_loading'),
        onChange: (written) => {
          // An editor being destroyed still dispatches: nothing of it is kept then.
          if (isGone) return;
          // The serializer ends a block with a line break the reader never typed; stored, it would
          // make an unchanged description differ from itself.
          const markdown = written.replace(/\n+$/, '');
          current = markdown;
          isEmpty = markdown.trim() === '';
          onchange?.(markdown);
        },
      });
      made.on('focus', () => (isFocused = true));
      made.on('blur', () => {
        if (isGone) return;
        isFocused = false;
        oncommit?.();
      });
      if (lang) made.view.dom.setAttribute('lang', lang);
      editor = made;
      isReady = true;
      void resolve(current);
    });
    return () => {
      isGone = true;
      editor?.destroy();
      editor = undefined;
    };
  });

  // A value saved elsewhere (another tab, the server's answer) is drawn while nobody types here.
  $effect(() => {
    const next = value;
    untrack(() => {
      if (!editor || !library || isFocused || next === current) return;
      current = next;
      isEmpty = next.trim() === '';
      editor.commands.setContent(library.fromMarkdown(next) as never, { emitUpdate: false });
      void resolve(next);
    });
  });

  $effect(() => {
    const can = editable;
    untrack(() => editor?.setEditable(can, false));
  });

  /** Applies a mark or a block from the formatting bar at the caret. */
  export function format(kind: Format): void {
    if (!editor) return;
    const chain = editor.chain().focus();
    if (kind === 'bold') chain.toggleBold().run();
    else if (kind === 'italic') chain.toggleItalic().run();
    else if (kind === 'strike') chain.toggleStrike().run();
    else if (kind === 'code') chain.toggleCode().run();
    else if (kind === 'list') chain.toggleBulletList().run();
    else if (kind === 'checklist') chain.toggleTaskList().run();
    else if (kind === 'quote') chain.toggleBlockquote().run();
    else if (editor.isActive('link')) chain.unsetLink().run();
    else {
      // A link from the words selected, where they are an address; Markdown's own `[words](address)` otherwise.
      const { from, to } = editor.state.selection;
      const words = editor.state.doc.textBetween(from, to).trim();
      if (/^https?:\/\/\S+$/u.test(words)) chain.setLink({ href: words }).run();
      else chain.run();
    }
  }

  /** Starts the text over - a comment sent, another one to edit - even while the focus is in it. */
  export function reset(markdown: string): void {
    current = markdown;
    isEmpty = markdown.trim() === '';
    if (editor && library) editor.commands.setContent(library.fromMarkdown(markdown) as never, { emitUpdate: false });
  }

  /** Whether the caret stands at the start of a line, where `/` opens the insert menu instead of being typed. */
  export function isAtLineStart(): boolean {
    return editor ? editor.state.selection.empty && editor.state.selection.$from.parentOffset === 0 : true;
  }

  /** Puts Markdown - a mention, a reference - in at the caret, as the writer would have typed it. */
  export function insert(markdown: string): void {
    if (!editor || !library) return;
    editor.chain().focus().insertContent(library.parseInline(markdown) as never).run();
    void resolve(markdown);
  }
</script>

{#if !isReady && !isEmpty}<div class="loading" aria-busy="true"><Skeleton lines={3} /></div>{/if}
<div
  class="rich"
  data-editable={editable ? '' : undefined}
  data-empty={isEmpty ? '' : undefined}
  data-placeholder={placeholder}
  role="presentation"
  bind:this={host}
  onkeydown={(event) => {
    if (event.key !== 'Escape' || !onescape) return;
    event.preventDefault();
    onescape();
    // The caller's value is the text as it was; drawn now, while the focus still holds the
    // drawing back (the effect above waits for nobody to be typing).
    flushSync();
    if (editor && library) {
      current = value;
      isEmpty = value.trim() === '';
      editor.commands.setContent(library.fromMarkdown(value) as never, { emitUpdate: false });
    }
    editor?.commands.blur();
  }}
></div>

<style>
  .loading { padding-block: var(--sp-050); }
  .rich { position: relative; min-inline-size: 0; overflow-wrap: anywhere; }
  .rich[data-empty]::before {
    position: absolute;
    inset-block-start: 0;
    inset-inline-start: 0;
    color: var(--text-subtle);
    content: attr(data-placeholder);
    pointer-events: none;
  }
  .rich :global(.page-body) { color: var(--text-primary); line-height: var(--lh-normal); outline: none; }
  .rich[data-editable] :global(.page-body) { min-block-size: var(--sp-300); border-radius: var(--r-sm); }
  .rich[data-editable] :global(.page-body:focus-visible) { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-050); }
  .rich :global(.page-body > :first-child) { margin-block-start: 0; }
  .rich :global(.page-body > :last-child) { margin-block-end: 0; }
  /* The column is the measure here, as in the scene: the global 60ch of a lone paragraph would wrap
     a comment or the notes at two thirds of the section. */
  .rich :global(.page-body p) { margin-block: 0 var(--sp-100); max-width: none; }
  .rich :global(.page-body h1),
  .rich :global(.page-body h2),
  .rich :global(.page-body h3) { margin-block: var(--sp-150) var(--sp-050); font-family: var(--font-display); font-size: var(--fs-200); line-height: var(--lh-tight); }
  .rich :global(.page-body ul),
  .rich :global(.page-body ol) { margin-block: 0 var(--sp-100); padding-inline-start: var(--sp-300); }
  .rich :global(.page-body pre) { padding: var(--sp-100); border-radius: var(--r-md); background: var(--bg-surface-sunken); font-family: var(--font-mono); }
  .rich :global(.page-body code) { font-family: var(--font-mono); }
  .rich :global(.page-body a) { color: var(--text-brand); }
  .rich :global(.page-body blockquote) {
    margin-inline: 0;
    padding-inline-start: var(--sp-150);
    border-inline-start: var(--bw-thick) solid var(--border-default);
    color: var(--text-secondary);
  }
  .rich :global(.page-body ul[data-type='taskList']) { list-style: none; padding-inline-start: var(--sp-050); }
  .rich :global(.page-body ul[data-type='taskList'] li) { display: flex; gap: var(--sp-100); align-items: baseline; }
  .rich :global(.page-body table) { border-collapse: collapse; }
  .rich :global(.page-body th),
  .rich :global(.page-body td) { padding: var(--sp-025) var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); text-align: start; }
  .rich :global(.page-mention) {
    padding-inline: var(--sp-025);
    border-radius: var(--r-xs);
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
    cursor: pointer;
  }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .rich :global(.page-body ul[data-type='taskList'] li > label) { position: relative; }
    .rich :global(.page-body ul[data-type='taskList'] li > label::after) { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
