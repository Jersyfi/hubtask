<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A page's body (UC-DOC-02): the block editor, bound to the page's live document for a writer
  // and drawing the saved body for a reader. Mentions, cards and live views are resolved for the
  // reader, and drawn without a title where the reader may not see what they point at (P-19).
  //
  // **The live document is the page while writers edit** (offline-sync.md §13). A writer's editor
  // starts only once the server's state has arrived; until then the saved body is drawn, read-only,
  // so that an empty document is never seeded twice and nothing typed waits for a document that
  // is not there. A page written through the API, imported or made from a template has a body and
  // no live document yet: the first writer's editor seeds it from the body.
  //
  // **The Markdown is saved beside it,** a moment after the typing stops, without `If-Match`
  // (`pages.saveBody`): search, versions, references and export read it.
  //
  // A reader's editor is not connected: what they read is the body as last saved.

  import { mount, tick, unmount, untrack } from 'svelte';

  import './page-text.css';
  import { BlockHandle, BoxPopover, Button, FloatingToolbar, IconButton, Inline, InsertList, Menu, Toolbar, UploadField } from '@hubtask/design-system/components';
  import {
    Doc, LiveDocument, actOnLine, createPageEditor, lineAt, insertReference, insertWikiBlock, linkLineToEntry, markdownOf, mirrorCompletion, parseAddress, passageTops,
    replaceFromMarkdown, revealBlock, seedFromMarkdown, setPassages, taskLineOf, type Editor, type LiveSocket, type LiveStatus, type Passage,
    type ReferenceAttrs, type ReferenceDrawing, type SelectedBlock, type TaskLine, type WikiBlock,
  } from '@hubtask/editor';
  import type { PageRead, ReferenceTarget } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { referencesIn, resolveRequest, targetLabel } from '../data/links.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { caretOf, pages, writersHere, writtenBody } from '../data/pages.svelte.ts';
  import { unsentDocuments } from '../data/unsentdocs.ts';
  import { liveKeeper } from '../data/unsentkeeper.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { page as frame } from '../frame/page.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { isLiveUnreachable } from '../data/liveunreachable.ts';
  import MentionDialog from '../entries/MentionDialog.svelte';
  import LiveEmbed from './LiveEmbed.svelte';
  import PagePicture from './PagePicture.svelte';
  import EmojiPicker from './EmojiPicker.svelte';
  import { pictureColours } from '../data/whiteboards.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { formatDay } from '../i18n/datetime.ts';
  import { renderProblem } from '../problem.ts';
  import { messages } from '../i18n/i18n.svelte.ts';

  interface Props {
    page: PageRead;
    /** The pages of the space, offered as links to a page. */
    spacePages: readonly { id: string; title: string; parent_id?: string | null }[];
    /** The views along the hub, offered as live views. */
    views: readonly { id: string; name: string }[];
    onnavigate: (path: string) => void;
    /** The block the caret stands in and the words selected in it, for an inline comment. */
    onblock?: (block: SelectedBlock | undefined) => void;
    /** A body to put in place of the document's, once per object: a draft thrown away (UC-DOC-15). */
    replaceWith?: { body: string };
    /** A block to scroll to, set by the comment margin. */
    reveal?: string;
    /** The open threads' passages, marked in the page (UC-DOC-11 check 4). */
    passages?: readonly Passage[];
    /** Called with where each marked passage starts, from the viewport's top, whenever it moves. */
    onpassages?: (tops: Map<string, number>) => void;
    /** The hub's collections, offered to hold a checklist line made a task (UC-DOC-03). */
    collections?: readonly { id: string; name: string }[];
    /** Called with the names of the other writers in the live document, whenever they change. */
    onpresent?: (names: string[]) => void;
    /** Called with the page's headings of levels 2 and 3 whenever they change: the outline (S-3). */
    onheadings?: (headings: { id: string; label: string; level: number }[]) => void;
    /** *Comment* on the bar above selected words: the margin's writing field takes the words. */
    oncomment?: () => void;
  }

  const { page, spacePages, views, onnavigate, onblock, reveal, passages = [], onpassages, collections = [], replaceWith, onpresent, onheadings, oncomment }: Props = $props();

  const SAVE_AFTER_MS = 1_500;

  let host = $state<HTMLElement | undefined>(undefined);
  let editor = $state<Editor | undefined>(undefined);
  let status = $state<LiveStatus | 'reading'>('reading');
  // Local changes the server has not had yet, kept on the device meanwhile (unsentdocs.ts).
  let unsent = $state(0);
  let failedAttempts = $state(0);
  let isLive = $state(false);
  let saving = $state<'idle' | 'saving' | 'saved' | 'failed'>('idle');
  let saveRefusal = $state<string | undefined>(undefined);
  let isMentioning = $state(false);
  let isLinkingPage = $state(false);
  let isEmbedding = $state(false);
  let isAddingBlock = $state(false);
  let embedAddress = $state('');
  let statusLabel = $state('');
  let mathSource = $state('');
  let isPickingEmoji = $state(false);
  let isMakingTask = $state(false);
  let taskLine = $state<TaskLine | undefined>(undefined);

  // --- References, resolved for the reader ------------------------------------------------------

  const targets = new Map<string, ReferenceTarget>();
  const listeners = new Set<() => void>();
  const keyOf = (ref: { kind: string; id: string }) => `${ref.kind.toUpperCase()}/${ref.id.toLowerCase()}`;

  const drawing: ReferenceDrawing = {
    label: (ref: ReferenceAttrs) => {
      const target = targets.get(keyOf(ref));
      if (!target) return undefined;
      return targetLabel(target) ?? t('app.pages.reference_unavailable');
    },
    open: (ref: ReferenceAttrs) => {
      const target = targets.get(keyOf(ref));
      if (!target?.available) return;
      const kind = ref.kind.toUpperCase();
      if (kind === 'ENTRY') onnavigate(`/items/${ref.id}`);
      else if (kind === 'PAGE') onnavigate(`/pages/${ref.id}`);
      else if (kind === 'HUB') onnavigate(`/hubs/${ref.id}`);
      else if (kind === 'COLLECTION') onnavigate(`/collections/${ref.id}`);
      else if (kind === 'CI') onnavigate(`/cis/${ref.id}`);
      else if (kind === 'VIEW' && target.collection_id) onnavigate(`/collections/${target.collection_id}?view=${ref.id}`);
    },
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    // A whiteboard, or one frame of it, is drawn live where the page embeds it (research §4.5).
    // A picture or a file the page holds is drawn as itself (UC-DOC-14).
    mount: (ref: ReferenceAttrs, into: HTMLElement) => {
      const kind = ref.kind.toUpperCase();
      if (kind === 'MEDIA') {
        const view = mount(PagePicture, { target: into, props: { mediaId: ref.id } });
        return () => void unmount(view);
      }
      // A view and a whiteboard live in the page under one head (S-4).
      if (kind !== 'WHITEBOARD' && kind !== 'VIEW') return undefined;
      const view = mount(LiveEmbed, { target: into, props: { ref, drawing, canOpen: () => targets.get(keyOf(ref))?.available === true, onnavigate } });
      return () => void unmount(view);
    },
    // A child-pages block lists the pages under this one in the space, as the tree holds them.
    children: (into: HTMLElement) => {
      const below = spacePages.filter((each) => each.parent_id === page.id);
      if (below.length === 0) return undefined;
      const list = document.createElement('ul');
      list.className = 'page-children__list';
      for (const each of below) {
        const item = document.createElement('li');
        const link = document.createElement('button');
        link.type = 'button';
        link.className = 'page-children__link';
        link.textContent = each.title;
        link.addEventListener('click', () => onnavigate(`/pages/${each.id}`));
        item.append(link);
        list.append(item);
      }
      into.append(list);
      return () => list.remove();
    },
    day: (iso: string) => formatDay(`${iso}T12:00:00Z`, messages.locale),
    words: (key) => t(`app.pages.block_${key}`),
    // The origins the interface's policy frames; any other embed stays a link card.
    frames: () => manifest.value?.frame_sources ?? [],
    // A diagram is a picture, drawn in the colours an exported whiteboard takes.
    diagramColours: () => pictureColours(),
  };

  let resolving = 0;
  async function resolve(markdown: string) {
    const found = referencesIn(markdown).map((each) => each.reference);
    if (found.length === 0) return;
    const mine = (resolving += 1);
    const state = await engine.refresh<{ data: ReferenceTarget[] }>(resolveRequest(found));
    if (mine !== resolving || state.status !== 'ready') return;
    for (const target of state.data.data) targets.set(keyOf(target), target);
    for (const listener of listeners) listener();
    // A line made a task is ticked as its entry stands (UC-DOC-03).
    const completed = new Map<string, boolean>();
    for (const target of state.data.data) {
      if (target.kind === 'ENTRY' && target.available && target.completed !== undefined) completed.set(target.id.toLowerCase(), target.completed);
    }
    if (editor && completed.size > 0) mirrorCompletion(editor, completed);
  }

  // --- A checklist line made a task (UC-DOC-03) --------------------------------------------------

  async function makeTask(collectionId: string) {
    const line = taskLine;
    isMakingTask = false;
    if (!editor || !line || line.entryId || line.text === '') return;
    try {
      const entry = await items.create({ type: 'TASK', collection_id: collectionId, title: line.text }, crypto.randomUUID());
      if (!editor || !linkLineToEntry(editor, line, entry.id)) return;
      changed(markdownOf(editor));
      toast.show({ text: t('app.pages.task_made', { title: entry.title }), tone: 'success' });
    } catch (error) {
      toast.show({ text: t('app.pages.task_failed', { reason: renderProblem(error as never, messages).message }), tone: 'danger' });
    }
  }

  async function ticked(entryId: string, checked: boolean) {
    try {
      await items.setCompleted(entryId, checked, crypto.randomUUID());
    } catch (error) {
      toast.show({ text: t('app.pages.task_failed', { reason: renderProblem(error as never, messages).message }), tone: 'danger' });
    }
    if (editor) void resolve(markdownOf(editor));
  }

  // --- Saving the Markdown beside the live document ----------------------------------------------

  let lastSaved = untrack(() => writtenBody(page));
  let timer: ReturnType<typeof setTimeout> | undefined;

  function changed(markdown: string) {
    clearTimeout(timer);
    timer = setTimeout(() => void save(markdown), SAVE_AFTER_MS);
  }

  async function save(markdown: string) {
    void resolve(markdown);
    if (markdown === lastSaved) return;
    saving = 'saving';
    saveRefusal = undefined;
    try {
      await pages.saveBody(page, markdown);
      lastSaved = markdown;
      saving = 'saved';
    } catch (error) {
      saving = 'failed';
      saveRefusal = renderProblem(error as never, messages).message;
    }
  }

  $effect(() => {
    const next = replaceWith;
    if (!next) return;
    untrack(() => {
      if (!editor || !isLive || !replaceFromMarkdown(editor, next.body)) return;
      // The body given is what stands now, as the editor writes it out: nothing waits to be saved.
      lastSaved = markdownOf(editor);
    });
  });

  // Writing in the body hides the phone's bottom bar (the navigation concept's §9): the keyboard
  // and the editor's own toolbar take the room.
  $effect(() => {
    const element = host;
    if (!element || !page.writable) return;
    let release: (() => void) | undefined;
    const enter = () => {
      release ??= frame.write();
    };
    // Decided after the event: a focused control removed while it is drawn (a panel's choice) sends
    // its focusout in the middle of the drawing, where no state may change.
    const leave = (event: FocusEvent) => {
      if (event.relatedTarget instanceof Node && element.contains(event.relatedTarget)) return;
      queueMicrotask(() => {
        if (element.contains(document.activeElement)) return;
        release?.();
        release = undefined;
      });
    };
    element.addEventListener('focusin', enter);
    element.addEventListener('focusout', leave);
    return () => {
      element.removeEventListener('focusin', enter);
      element.removeEventListener('focusout', leave);
      release?.();
    };
  });

  // The outline writes itself from the headings (S-3); a heading without words is left out.
  $effect(() => {
    const seen = editor;
    if (!seen || !onheadings) return;
    let last = '';
    const read = () => {
      const headings: { id: string; label: string; level: number }[] = [];
      seen.state.doc.forEach((node) => {
        const level = Number(node.attrs['level'] ?? 0);
        const id = node.attrs['blockId'] as string | null;
        if (node.type.name === 'heading' && id && (level === 2 || level === 3) && node.textContent.trim() !== '') {
          headings.push({ id, label: node.textContent.trim(), level });
        }
      });
      const key = JSON.stringify(headings);
      if (key !== last) {
        last = key;
        onheadings(headings);
      }
    };
    read();
    seen.on('update', read);
    return () => void seen.off('update', read);
  });

  // Tools only while writing (S-1): the bar stands while the focus is in the body or one of its
  // panels; "/" on an empty line opens the blocks to insert, as the bar's *Block* does.
  let zone = $state<HTMLElement | undefined>(undefined);
  let isWriting = $state(false);
  $effect(() => {
    const element = zone;
    if (!element) return;
    const enter = () => (isWriting = true);
    const leave = (event: FocusEvent) => {
      if (event.relatedTarget instanceof Node && element.contains(event.relatedTarget)) return;
      queueMicrotask(() => {
        if (!element.contains(document.activeElement)) isWriting = false;
      });
    };
    element.addEventListener('focusin', enter);
    element.addEventListener('focusout', leave);
    return () => {
      element.removeEventListener('focusin', enter);
      element.removeEventListener('focusout', leave);
    };
  });
  $effect(() => {
    const element = host;
    if (!element) return;
    const slash = (event: KeyboardEvent) => {
      const seen = editor;
      if (event.key !== '/' || !seen?.isEditable || !seen.state.selection.empty) return;
      if (seen.state.selection.$from.parent.textContent !== '') return;
      openInsert(true);
      slashQuery = '';
    };
    element.addEventListener('keydown', slash, true);
    return () => element.removeEventListener('keydown', slash, true);
  });
  const hasPanel = $derived(isAddingBlock || isLinkingPage || isEmbedding || isPickingEmoji || isMakingTask);

  // --- The tools at the text (S-1): a bar above selected words, "+" and the handle at a line ------
  // A phone keeps the one bar above the keyboard (§7); from a tablet on the tools stand at the text.

  type Box = { top: number; left: number; width: number; height: number }; // design-system-lint-ignore: a viewport box, as the browser measures it
  let selectionBox = $state<Box | undefined>(undefined);
  let line = $state<{ start: number; box: Box } | undefined>(undefined);
  /** The line the insert panel was opened at, and the panel's box under it. */
  let insertBox = $state<Box | undefined>(undefined);
  let blockForm = $state<'math' | 'status' | 'embed' | undefined>(undefined);
  let isLinkingWeb = $state(false);
  let linkAddress = $state('');
  let marks = $state(0);
  const atText = $derived(page.writable && isLive && !viewport.isCompact);

  const boxOf = (element: Element): Box => {
    const r = element.getBoundingClientRect();
    return { top: r.top, left: r.left, width: r.width, height: r.height }; // design-system-lint-ignore: a viewport box, as the browser measures it
  };

  function measure() {
    const seen = editor;
    const body = host;
    if (!seen || seen.isDestroyed || !body) return;
    marks += 1;
    const { selection } = seen.state;
    // A chosen block (a node selection: a card, a picture) has no words to format.
    if (selection.empty || 'node' in selection) {
      selectionBox = undefined;
      isLinkingWeb = false;
    } else {
      const a = seen.view.coordsAtPos(selection.from);
      const b = seen.view.coordsAtPos(selection.to);
      const frame = body.getBoundingClientRect();
      const oneLine = Math.abs(a.top - b.top) < 1;
      const left = oneLine ? Math.min(a.left, b.left) : frame.left;
      const right = oneLine ? Math.max(a.right, b.right) : frame.right;
      const top = Math.min(a.top, b.top);
      selectionBox = { top, left, width: Math.max(1, right - left), height: Math.max(a.bottom, b.bottom) - top };
    }
    const at = lineAt(seen);
    if (at) line = { start: at.start, box: boxOf(at.element) };
  }

  $effect(() => {
    const seen = editor;
    if (!seen || !atText) return;
    return untrack(() => {
      measure();
      seen.on('selectionUpdate', measure);
      seen.on('update', measure);
      window.addEventListener('resize', measure);
      return () => {
        seen.off('selectionUpdate', measure);
        seen.off('update', measure);
        window.removeEventListener('resize', measure);
      };
    });
  });

  // The boxes are where the text stood when measured; the status line and the phone's bar come and
  // go above the text with writing, so the text moves and is measured again once they are drawn.
  $effect(() => {
    void isWriting;
    void hasPanel;
    if (atText) void tick().then(measure);
  });

  // The handle follows the pointer over the text, as it follows the caret.
  function hover(event: PointerEvent) {
    const seen = editor;
    const body = host;
    if (!seen || !body || !atText || !isWriting) return;
    // Only the text itself moves the handle: a pointer crossing the handle's menu or a panel laid
    // over other lines would retarget the line the menu acts on.
    if (!(event.target instanceof Node) || !body.contains(event.target)) return;
    const frame = body.getBoundingClientRect();
    if (event.clientY < frame.top || event.clientY > frame.bottom) return;
    const at = lineAt(seen, { left: Math.min(Math.max(event.clientX, frame.left + 1), frame.right - 1), top: event.clientY }); // design-system-lint-ignore: a viewport box, as the browser measures it
    if (at && at.start !== line?.start) line = { start: at.start, box: boxOf(at.element) };
  }

  // A field that appears after a choice takes the focus once it is drawn, never while it is drawn:
  // focus moving during the drawing would change the state the drawing reads.
  const focusSoon = (element: HTMLElement) => void queueMicrotask(() => element.focus());

  // A checklist line not yet a task offers *task from it* at its end (S-4, UC-DOC-03).
  const taskBox = $derived.by((): Box | undefined => {
    void marks;
    const seen = editor;
    const body = host;
    if (!seen || !body || !taskLine || taskLine.entryId || taskLine.text === '' || !isWriting || hasPanel) return undefined;
    const at = seen.view.coordsAtPos(Math.min(taskLine.pos + 2, seen.state.doc.content.size));
    const frame = body.getBoundingClientRect();
    return { top: at.top, left: frame.right - 1, width: 1, height: Math.max(1, at.bottom - at.top) }; // design-system-lint-ignore: a viewport box, as the browser measures it
  });

  /** A panel opened by a button takes the focus, as a menu does; one opened by "/" leaves it in the line. */
  const focusPanel = () => void tick().then(() => zone?.querySelector<HTMLElement>('[data-box-popover] button')?.focus());

  /** Opened by "/", the focus stays in the line and the editor hands the list its keys (`listKeys`). */
  let insertList = $state<ReturnType<typeof InsertList> | undefined>(undefined);
  let isInsertByKey = $state(false);
  /** The words typed after "/" in its line, which narrow the list; undefined when "/" did not open it. */
  let slashQuery = $state<string | undefined>(undefined);

  // The "/" stays in the line and what follows it narrows the list; the line no longer starting with
  // "/" (erased, or the caret gone to another line) closes it.
  const isSlashOpen = $derived(slashQuery !== undefined);
  $effect(() => {
    const seen = editor;
    if (!seen || !isAddingBlock || !isSlashOpen) return;
    return untrack(() => {
      const follow = () => {
        if (slashQuery === undefined) return;
        const text = seen.state.selection.$from.parent.textContent;
        if (text.startsWith('/')) slashQuery = text.slice(1);
        else closeInsert();
      };
      seen.on('transaction', follow);
      return () => void seen.off('transaction', follow);
    });
  });

  /** A choice taken from "/" replaces the "/" and the words typed after it. */
  function dropSlash() {
    const seen = editor;
    if (!seen || slashQuery === undefined) return;
    slashQuery = undefined;
    const at = seen.state.selection.$from;
    if (at.parent.textContent.startsWith('/')) seen.chain().deleteRange({ from: at.start(), to: at.end() }).run();
  }

  function openInsert(byKey = false) {
    blockForm = undefined;
    isInsertByKey = byKey;
    isAddingBlock = true;
    const at = editor ? lineAt(editor) : undefined;
    insertBox = at ? boxOf(at.element) : line?.box;
  }

  const LIST_KEYS = new Set(['ArrowDown', 'ArrowUp', 'Home', 'End', 'Enter', 'Escape']);
  $effect(() => {
    const element = host;
    if (!element) return;
    const listKeys = (event: KeyboardEvent) => {
      if (!isAddingBlock || !isInsertByKey || blockForm || !LIST_KEYS.has(event.key)) return;
      if (insertList?.keydown(event)) event.stopPropagation();
    };
    element.addEventListener('keydown', listKeys, true);
    return () => element.removeEventListener('keydown', listKeys, true);
  });

  let fileChooser = $state<ReturnType<typeof UploadField> | undefined>(undefined);

  const insertGroups = $derived([
    {
      label: t('app.pages.insert_hubtask'),
      choices: [
        { id: 'view', label: t('app.pages.tool_view'), detail: t('app.pages.insert_detail_view'), icon: 'table' as const },
        { id: 'mention', label: t('app.pages.tool_mention'), detail: t('app.pages.insert_detail_mention'), icon: 'at-sign' as const },
        { id: 'page', label: t('app.pages.tool_page'), detail: t('app.pages.insert_detail_page'), icon: 'file-text' as const },
        { id: 'emoji', label: t('app.pages.tool_emoji'), detail: t('app.pages.insert_detail_emoji'), icon: 'sparkles' as const },
        { id: 'file', label: t('app.pages.tool_file'), detail: t('app.pages.insert_detail_file'), icon: 'image' as const },
      ],
    },
    {
      label: t('app.pages.insert_blocks'),
      choices: [
        ...(['info', 'warning', 'success', 'error'] as const).map((tone) => ({
          id: `callout_${tone}`, label: t(`app.pages.block_callout_${tone}`), detail: t('app.pages.insert_detail_callout'), icon: 'info' as const,
        })),
        { id: 'details', label: t('app.pages.block_toggle'), detail: t('app.pages.insert_detail_details'), icon: 'chevron-right' as const },
        { id: 'toc', label: t('app.pages.block_toc'), detail: t('app.pages.insert_detail_toc'), icon: 'list' as const },
        { id: 'children', label: t('app.pages.block_child_pages'), detail: t('app.pages.insert_detail_children'), icon: 'folder' as const },
        { id: 'date', label: t('app.pages.block_date'), detail: t('app.pages.insert_detail_date'), icon: 'calendar' as const },
        { id: 'diagram', label: t('app.pages.block_diagram_add'), detail: t('app.pages.insert_detail_diagram'), icon: 'git-branch' as const },
        { id: 'math', label: t('app.pages.block_math_add'), detail: t('app.pages.insert_detail_math'), icon: 'type' as const },
        { id: 'status', label: t('app.pages.block_status'), detail: t('app.pages.insert_detail_status'), icon: 'tag' as const },
        { id: 'embed', label: t('app.pages.block_embed_add'), detail: t('app.pages.insert_detail_embed'), icon: 'globe' as const },
      ],
    },
  ]);

  function choose(id: string) {
    dropSlash();
    const at = line?.box;
    if (id === 'view' || id === 'page' || id === 'emoji') {
      closeInsert();
      if (id === 'view') isEmbedding = true;
      else if (id === 'page') isLinkingPage = true;
      else isPickingEmoji = true;
      insertBox = at;
      focusPanel();
    } else if (id === 'mention') {
      closeInsert();
      isMentioning = true;
    } else if (id === 'file') {
      closeInsert();
      fileChooser?.choose();
    } else if (id.startsWith('callout_')) {
      addBlock({ kind: 'callout', tone: id.slice('callout_'.length) as 'info' | 'warning' | 'success' | 'error' });
    } else if (id === 'details') addBlock({ kind: 'details', summary: '' });
    else if (id === 'toc') addBlock({ kind: 'toc' });
    else if (id === 'children') addBlock({ kind: 'children' });
    else if (id === 'date') addBlock({ kind: 'date', date: today() });
    else if (id === 'diagram') addBlock({ kind: 'diagram', source: 'graph TD\n  A --> B' });
    else if (id === 'math' || id === 'status' || id === 'embed') blockForm = id;
  }

  function closeInsert() {
    slashQuery = undefined;
    isAddingBlock = false;
    blockForm = undefined;
    insertBox = undefined;
  }

  /** The kinds of text a line can be turned into, from the bar's first choice or the handle. */
  const STYLES = ['paragraph', 'heading2', 'heading3', 'list', 'numbered', 'checklist', 'quote', 'code'] as const;
  type Style = (typeof STYLES)[number];
  const styleLabel = (style: Style) => t(`app.pages.style_${style}`);
  const currentStyle = $derived.by((): Style => {
    void marks;
    const seen = editor;
    if (!seen) return 'paragraph';
    if (seen.isActive('heading', { level: 2 })) return 'heading2';
    if (seen.isActive('heading', { level: 3 })) return 'heading3';
    if (seen.isActive('taskList')) return 'checklist';
    if (seen.isActive('bulletList')) return 'list';
    if (seen.isActive('orderedList')) return 'numbered';
    if (seen.isActive('blockquote')) return 'quote';
    if (seen.isActive('codeBlock')) return 'code';
    return 'paragraph';
  });
  const active = $derived.by(() => {
    void marks;
    const seen = editor;
    return { bold: seen?.isActive('bold') ?? false, italic: seen?.isActive('italic') ?? false, strike: seen?.isActive('strike') ?? false, code: seen?.isActive('code') ?? false };
  });
  const styleItems = $derived(STYLES.map((style) => ({ id: style, label: styleLabel(style) })));

  function turnInto(style: string, start?: number) {
    const seen = editor;
    if (!seen) return;
    let chain = seen.chain().focus();
    if (start !== undefined) chain = chain.setTextSelection(start + 1);
    if (style === 'paragraph') chain = chain.setParagraph();
    else if (style === 'heading2') chain = chain.setHeading({ level: 2 });
    else if (style === 'heading3') chain = chain.setHeading({ level: 3 });
    else if (style === 'list') chain = chain.toggleBulletList();
    else if (style === 'numbered') chain = chain.toggleOrderedList();
    else if (style === 'checklist') chain = chain.toggleTaskList();
    else if (style === 'quote') chain = chain.toggleBlockquote();
    else if (style === 'code') chain = chain.toggleCodeBlock();
    chain.run();
  }

  const lineItems = $derived([
    ...STYLES.map((style) => ({ id: `style:${style}`, label: styleLabel(style) })),
    { id: 'up', label: t('app.pages.line_up'), icon: 'arrow-up' as const, hasSeparatorBefore: true },
    { id: 'down', label: t('app.pages.line_down'), icon: 'arrow-down' as const },
    { id: 'remove', label: t('app.pages.line_remove'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true },
  ]);

  function lineAct(id: string) {
    const seen = editor;
    const at = line;
    if (!seen || !at) return;
    if (id.startsWith('style:')) turnInto(id.slice(6), at.start);
    else if (id === 'up' || id === 'down' || id === 'remove') actOnLine(seen, at.start, id);
    measure();
  }

  function addBelow() {
    const seen = editor;
    const at = line;
    if (!seen || !at) return;
    if (actOnLine(seen, at.start, 'below')) {
      measure();
      openInsert();
    }
  }

  function applyLink() {
    const address = linkAddress.trim();
    isLinkingWeb = false;
    linkAddress = '';
    if (!editor) return;
    if (address === '') editor.chain().focus().extendMarkRange('link').unsetLink().run();
    else editor.chain().focus().extendMarkRange('link').setLink({ href: address }).run();
  }
  const isQuiet = $derived(status === 'connected' && unsent === 0 && saveRefusal === undefined);

  // --- The editor and its live document ------------------------------------------------------------

  $effect(() => {
    const element = host;
    if (!element) return;
    return untrack(() => {
      // A writer of a space with drafts edits the draft waiting; a reader reads what was published.
      const body = page.writable ? writtenBody(page) : (page.body ?? '');
      void resolve(body);
      const blockSeen = (block: SelectedBlock | undefined) => {
        onblock?.(block);
        taskLine = editor ? taskLineOf(editor) : undefined;
      };
      let current = createPageEditor({
        element, body, editable: false, drawing, label: t('app.pages.body_label', { title: page.title }), fallback: t('app.pages.reference_loading'), onBlock: blockSeen,
      });
      editor = current;
      if (!page.writable) return () => current.destroy();

      const doc = new Doc();
      const account = actor.account;
      const connection: LiveDocument = new LiveDocument(doc, {
        open: () => pages.liveSocket(page.id) as unknown as LiveSocket,
        writable: true,
        onStatus: (next) => {
          status = next;
          failedAttempts = connection.failedAttempts;
        },
        keep: liveKeeper('page', page.id),
        onUnsent: (count) => {
          unsent = count;
          unsentDocuments.set(`page:${page.id}`, count);
        },
        onSynced: () => {
          if (isLive) return;
          current.destroy();
          element.replaceChildren();
          current = createPageEditor({
            element, body, editable: true, drawing, label: t('app.pages.body_label', { title: page.title }), placeholder: t('app.pages.empty_text'),
            live: { doc, connection, user: { name: account?.display_name ?? '', caret: caretOf(account?.id ?? '') } },
            fallback: t('app.pages.reference_loading'),
            onChange: changed,
            onBlock: blockSeen,
            onTick: (entryId, checked) => void ticked(entryId, checked),
          });
          editor = current;
          isLive = true;
          // A page with a body and no live document yet: the first writer seeds it. The seed is the
          // body itself, only written out again: nothing to save, or opening a page gave it a
          // version nobody wrote.
          if (seedFromMarkdown(current, body)) lastSaved = markdownOf(current);
        },
      });
      const present = () => onpresent?.(writersHere(connection.awareness.getStates(), doc.clientID));
      connection.awareness.on('change', present);
      return () => {
        connection.awareness.off('change', present);
        onpresent?.([]);
        clearTimeout(timer);
        // What was typed in the last moment is saved on the way out.
        if (isLive && current.isEditable) void save(markdownOf(current));
        current.destroy();
        connection.destroy();
        unsentDocuments.forget(`page:${page.id}`);
        doc.destroy();
      };
    });
  });

  $effect(() => {
    if (reveal && editor) revealBlock(editor, reveal);
  });

  // The passages are marked again whenever the threads or the editor change, and their places are
  // reported after every transaction - another writer's typing moves them too - and on a resize.
  $effect(() => {
    const current = editor;
    const marked = passages;
    if (!current) return;
    return untrack(() => {
      const report = () => onpassages?.(passageTops(current));
      setPassages(current, marked);
      report();
      current.on('transaction', report);
      window.addEventListener('resize', report);
      return () => {
        current.off('transaction', report);
        window.removeEventListener('resize', report);
      };
    });
  });

  // --- The toolbar ---------------------------------------------------------------------------------

  const run = (command: (chain: ReturnType<Editor['chain']>) => ReturnType<Editor['chain']>) => {
    if (!editor) return;
    command(editor.chain().focus()).run();
  };

  function insertAddressText(text: string) {
    const match = /\((hubtask:[^)\s]+)\)/u.exec(text);
    const address = match ? parseAddress(match[1] ?? '') : undefined;
    if (!editor || !address) return;
    insertReference(editor, address, text.startsWith('!'));
    changed(markdownOf(editor));
  }

  function insertPage(id: string) {
    if (!editor) return;
    insertReference(editor, { kind: 'page', id, anchor: null }, false);
    isLinkingPage = false;
    changed(markdownOf(editor));
  }

  // --- Pictures and files (UC-DOC-14) ---------------------------------------------------------------

  let isUploading = $state(false);

  /** An upload made one of the page's files, then drawn where the caret stands. */
  async function putFile(file: File) {
    if (!editor) return;
    isUploading = true;
    try {
      const object = await media.upload(file, 'ATTACHMENT', { idempotencyKey: crypto.randomUUID() });
      await pages.attachMedia(page.id, object.id);
      if (!editor) return;
      insertReference(editor, { kind: 'media', id: object.id, anchor: null }, true);
      changed(markdownOf(editor));
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isUploading = false;
    }
  }

  /** A wiki block at the caret (UC-DOC-01 check 10). */
  function addBlock(block: WikiBlock) {
    if (!editor) return;
    if (!insertWikiBlock(editor, block)) {
      toast.show({ text: t('app.pages.block_refused'), tone: 'danger' });
      return;
    }
    closeInsert();
    embedAddress = '';
    statusLabel = '';
    mathSource = '';
    changed(markdownOf(editor));
  }

  const today = () => todayIn(actor.zone);

  function embedView(id: string) {
    if (!editor) return;
    insertReference(editor, { kind: 'view', id, anchor: null }, true);
    isEmbedding = false;
    changed(markdownOf(editor));
  }

  const statusWords = $derived(
    !page.writable ? t('app.pages.status_reading')
    : status === 'connected' && isLive ? (saving === 'saving' ? t('app.pages.status_saving') : t('app.pages.status_live'))
    : status === 'refused' ? t('app.pages.status_refused')
    : unsent > 0 ? t('app.pages.status_unsent', { count: String(unsent) })
    : isLiveUnreachable(status, failedAttempts, live.state) ? t('app.pages.status_unreachable')
    : status === 'offline' ? t('app.pages.status_offline')
    : t('app.pages.status_connecting'),
  );
</script>

<!-- The handle follows the pointer; a hover only moves it, nothing is acted on. -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="writing" bind:this={zone} onpointermove={hover}>
{#if page.writable && isLive && viewport.isCompact && (isWriting || hasPanel)}
  <div class="tools" data-page-tools>
  <Toolbar label={t('app.pages.toolbar')}>
    <Inline gap="050" isWrapping>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleHeading({ level: 2 }))}>{t('app.pages.tool_heading')}</Button>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleBold())}>{t('app.pages.tool_bold')}</Button>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleItalic())}>{t('app.pages.tool_italic')}</Button>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleBulletList())}>{t('app.pages.tool_list')}</Button>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleOrderedList())}>{t('app.pages.tool_numbered')}</Button>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleTaskList())}>{t('app.pages.tool_checklist')}</Button>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleBlockquote())}>{t('app.pages.tool_quote')}</Button>
      <Button size="sm" tone="subtle" onclick={() => run((c) => c.toggleCodeBlock())}>{t('app.pages.tool_code')}</Button>
      <Button size="sm" tone="subtle" icon="link" onclick={() => (isMentioning = true)}>{t('app.pages.tool_mention')}</Button>
      <Button size="sm" tone="subtle" icon="file-text" onclick={() => (isLinkingPage = !isLinkingPage)}>{t('app.pages.tool_page')}</Button>
      <Button size="sm" tone="subtle" icon="layout-template" onclick={() => (isEmbedding = !isEmbedding)}>{t('app.pages.tool_view')}</Button>
      <Button size="sm" tone="subtle" icon="plus" onclick={() => (isAddingBlock ? closeInsert() : openInsert())}>{t('app.pages.tool_block')}</Button>
      <Button size="sm" tone="subtle" onclick={() => (isPickingEmoji = !isPickingEmoji)}>{t('app.pages.tool_emoji')}</Button>
      <UploadField
          variant="button"
          size="sm"
          label={t('app.pages.tool_file')}
          chooseLabel={isUploading ? t('app.pages.uploading') : t('app.pages.tool_file')}
          disabledReason={isUploading ? t('app.pages.uploading') : undefined}
          onChoose={(file) => void putFile(file)}
        />
      {#if taskLine && !taskLine.entryId && taskLine.text !== ''}
        <Button size="sm" tone="subtle" icon="square-check" onclick={() => (isMakingTask = !isMakingTask)}>{t('app.pages.tool_task')}</Button>
      {/if}
    </Inline>
  </Toolbar>
  </div>
{/if}

{#snippet panelsHere()}
{#if isLinkingPage}
  <ul class="choices" aria-label={t('app.pages.tool_page')}>
    {#each spacePages.filter((each) => each.id !== page.id) as each (each.id)}
      <li><button type="button" class="choice" onclick={() => insertPage(each.id)}>{each.title}</button></li>
    {:else}
      <li class="quiet">{t('app.pages.no_other_pages')}</li>
    {/each}
  </ul>
{/if}
{#if isAddingBlock}
  <div class="blocks" data-page-blocks>
    {#if blockForm === 'math'}
      <form class="block-form" onsubmit={(event) => { event.preventDefault(); addBlock({ kind: 'math', latex: mathSource }); }}>
        <input type="text" bind:value={mathSource} spellcheck="false" use:focusSoon placeholder={t('app.pages.block_math_source')} aria-label={t('app.pages.block_math_source')} />
        <Button size="sm" tone="subtle" type="submit" disabledReason={mathSource.trim() === '' ? t('app.pages.block_math_source') : undefined}>{t('app.pages.block_math_add')}</Button>
      </form>
    {:else if blockForm === 'status'}
      <form class="block-form" onsubmit={(event) => { event.preventDefault(); addBlock({ kind: 'status', label: statusLabel, tone: 'info' }); }}>
        <input type="text" bind:value={statusLabel} maxlength="40" use:focusSoon placeholder={t('app.pages.block_status_label')} aria-label={t('app.pages.block_status_label')} />
        <Button size="sm" tone="subtle" type="submit" disabledReason={statusLabel.trim() === '' ? t('app.pages.block_status_label') : undefined}>{t('app.pages.block_status')}</Button>
      </form>
    {:else if blockForm === 'embed'}
      <form class="block-form" onsubmit={(event) => { event.preventDefault(); addBlock({ kind: 'embed', src: embedAddress }); }}>
        <input type="url" bind:value={embedAddress} use:focusSoon placeholder="https://" aria-label={t('app.pages.block_embed_address')} />
        <Button size="sm" tone="subtle" type="submit" disabledReason={embedAddress.trim() === '' ? t('app.pages.block_embed_address') : undefined}>{t('app.pages.block_embed_add')}</Button>
      </form>
    {:else}
      <!-- What "/" and "+" offer: Hubtask's own first, then the blocks; a block's fields only after it is chosen. -->
      <InsertList
        bind:this={insertList}
        label={t('app.pages.insert')}
        groups={insertGroups}
        takesFocus={!isInsertByKey}
        query={slashQuery ?? ''}
        emptyText={t('app.pages.insert_none')}
        keys={{ move: t('app.pages.insert_keys_move'), take: t('app.pages.insert_keys_take'), close: t('app.pages.insert_keys_close') }}
        onchoose={choose}
        onclose={() => { closeInsert(); editor?.commands.focus(); }}
      />
    {/if}
  </div>
{/if}
{#if isPickingEmoji}
  <EmojiPicker onpick={(emoji) => { if (editor && insertWikiBlock(editor, { kind: 'emoji', emoji })) changed(markdownOf(editor)); isPickingEmoji = false; }} />
{/if}
{#if isMakingTask}
  <ul class="choices" aria-label={t('app.pages.task_in')}>
    {#each collections as each (each.id)}
      <li><button type="button" class="choice" onclick={() => makeTask(each.id)}>{each.name}</button></li>
    {:else}
      <li class="quiet">{t('app.pages.no_collections')}</li>
    {/each}
  </ul>
{/if}
{#if isEmbedding}
  <ul class="choices" aria-label={t('app.pages.tool_view')}>
    {#each views as each (each.id)}
      <li><button type="button" class="choice" onclick={() => embedView(each.id)}>{each.name}</button></li>
    {:else}
      <li class="quiet">{t('app.pages.no_views')}</li>
    {/each}
  </ul>
{/if}

{/snippet}

{#if atText}
  <!-- The bar starts where the words start, as the scene draws it: centred, a word near the line's start would carry it over the sidebar. -->
  <FloatingToolbar label={t('app.pages.format_bar')} container={zone} box={isWriting && !isLinkingWeb ? selectionBox : undefined} placement={{ side: 'block-start', align: 'start' }}>
    <Menu label={t('app.pages.style')} items={styleItems} onselect={(id) => turnInto(id)}>
      {#snippet trigger(props)}
        <Button size="sm" tone="subtle" data-page-style {...props}>{styleLabel(currentStyle)}</Button>
      {/snippet}
    </Menu>
    <IconButton icon="bold" size="sm" tone="subtle" label={t('app.pages.tool_bold')} aria-pressed={active.bold} onclick={() => run((c) => c.toggleBold())} />
    <IconButton icon="italic" size="sm" tone="subtle" label={t('app.pages.tool_italic')} aria-pressed={active.italic} onclick={() => run((c) => c.toggleItalic())} />
    <IconButton icon="strikethrough" size="sm" tone="subtle" label={t('app.pages.tool_strike')} aria-pressed={active.strike} onclick={() => run((c) => c.toggleStrike())} />
    <IconButton icon="code" size="sm" tone="subtle" label={t('app.pages.tool_inline_code')} aria-pressed={active.code} onclick={() => run((c) => c.toggleCode())} />
    <IconButton icon="link" size="sm" tone="subtle" label={t('app.pages.tool_link')} onclick={() => { linkAddress = String(editor?.getAttributes('link')['href'] ?? ''); isLinkingWeb = true; }} />
    {#if oncomment}
      <Button size="sm" tone="subtle" icon="message-square" data-page-comment-selection onclick={oncomment}>{t('app.pages.tool_comment')}</Button>
    {/if}
  </FloatingToolbar>
  <FloatingToolbar label={t('app.pages.tool_link')} container={zone} box={isLinkingWeb ? selectionBox : undefined}>
    <form class="block-form" onsubmit={(event) => { event.preventDefault(); applyLink(); }}>
      <input type="url" bind:value={linkAddress} use:focusSoon placeholder="https://" aria-label={t('app.pages.link_address')} onkeydown={(event) => { if (event.key === 'Escape') { isLinkingWeb = false; editor?.commands.focus(); } }} />
      <Button size="sm" tone="subtle" type="submit">{linkAddress.trim() === '' ? t('app.pages.link_remove') : t('app.pages.tool_link')}</Button>
    </form>
  </FloatingToolbar>
  <FloatingToolbar label={t('app.pages.tool_task')} container={zone} box={selectionBox ? undefined : taskBox} placement={{ side: 'inline-start', align: 'center' }}>
    <Button size="sm" tone="subtle" icon="square-check" data-page-task-from onclick={() => { insertBox = taskBox; isMakingTask = true; focusPanel(); }}>{t('app.pages.task_from')}</Button>
  </FloatingToolbar>
  <BlockHandle
    container={zone}
    box={isWriting && !hasPanel ? line?.box : undefined}
    addLabel={t('app.pages.line_add')}
    handleLabel={t('app.pages.line_handle')}
    onadd={addBelow}
    items={lineItems}
    onselect={lineAct}
  />
  <BoxPopover
    label={t('app.pages.insert')}
    container={zone}
    box={hasPanel ? (insertBox ?? line?.box) : undefined}
    onclose={() => { closeInsert(); isLinkingPage = false; isEmbedding = false; isPickingEmoji = false; isMakingTask = false; }}
  >
    {@render panelsHere()}
  </BoxPopover>
{:else}
  {@render panelsHere()}
{/if}

{#if page.writable && (isWriting || !isQuiet)}<p class="status" aria-live="polite">{statusWords}</p>{/if}
{#if saveRefusal}<p class="refusal" role="alert">{saveRefusal}</p>{/if}

<div class="editor page-text" bind:this={host}></div>
<div class="chooser"><UploadField bind:this={fileChooser} variant="icon" size="sm" label={t('app.pages.tool_file')} chooseLabel={t('app.pages.tool_file')} onChoose={(file) => void putFile(file)} /></div>
</div>

<MentionDialog bind:isOpen={isMentioning} selfId="" oninsert={insertAddressText} />

<style>
  .writing { display: grid; gap: var(--sp-100); }
  .chooser { display: none; }
  /* The bar stays at hand above the text while writing; on a phone it sits above the keyboard (§7). */
  .tools {
    position: sticky;
    inset-block-start: var(--sp-100);
    z-index: var(--z-sticky);
    padding: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .tools { inset-block-start: auto; inset-block-end: 0; overflow-x: auto; }
  }
  .status { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .refusal { margin: 0; color: var(--text-danger); }
  .choices {
    margin: 0;
    padding: var(--sp-100);
    list-style: none;
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .choice {
    padding: var(--sp-025) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    cursor: pointer;
  }
  .choice:hover { background: var(--bg-surface-hover); }
  .quiet { color: var(--text-secondary); }

</style>
