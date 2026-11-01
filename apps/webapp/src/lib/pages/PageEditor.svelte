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

  import { mount, unmount, untrack } from 'svelte';
  import { Button, Inline, Toolbar } from '@hubtask/design-system/components';
  import {
    Doc, LiveDocument, createPageEditor, insertReference, insertWikiBlock, linkLineToEntry, markdownOf, mirrorCompletion, parseAddress, passageTops,
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
  import { caretOf, pages, writtenBody } from '../data/pages.svelte.ts';
  import { unsentDocuments } from '../data/unsentdocs.ts';
  import { liveKeeper } from '../data/unsentkeeper.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { isLiveUnreachable } from '../data/liveunreachable.ts';
  import MentionDialog from '../entries/MentionDialog.svelte';
  import BoardView from '../whiteboards/BoardView.svelte';
  import PagePicture from './PagePicture.svelte';
  import { t } from '../i18n/i18n.svelte.ts';
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
  }

  const { page, spacePages, views, onnavigate, onblock, reveal, passages = [], onpassages, collections = [], replaceWith }: Props = $props();

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
      if (kind !== 'WHITEBOARD') return undefined;
      const view = mount(BoardView, { target: into, props: { boardId: ref.id, frame: ref.anchor, onopen: onnavigate } });
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
      // The page as published is what stands now; nothing waits to be saved.
      lastSaved = next.body;
    });
  });

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
        element, body, editable: false, drawing, fallback: t('app.pages.reference_loading'), onBlock: blockSeen,
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
            element, body, editable: true, drawing,
            live: { doc, connection, user: { name: account?.display_name ?? '', caret: caretOf(account?.id ?? '') } },
            fallback: t('app.pages.reference_loading'),
            onChange: changed,
            onBlock: blockSeen,
            onTick: (entryId, checked) => void ticked(entryId, checked),
          });
          editor = current;
          isLive = true;
          // A page with a body and no live document yet: the first writer seeds it.
          if (seedFromMarkdown(current, body)) changed(markdownOf(current));
        },
      });
      return () => {
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
  async function putFile(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file || !editor) return;
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
    isAddingBlock = false;
    embedAddress = '';
    statusLabel = '';
    changed(markdownOf(editor));
  }

  const today = () => new Date().toISOString().slice(0, 10);

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

{#if page.writable && isLive}
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
      <Button size="sm" tone="subtle" icon="plus" onclick={() => (isAddingBlock = !isAddingBlock)}>{t('app.pages.tool_block')}</Button>
      <label class="upload">
        <span>{isUploading ? t('app.pages.uploading') : t('app.pages.tool_file')}</span>
        <input type="file" disabled={isUploading} onchange={(event) => void putFile(event)} />
      </label>
      {#if taskLine && !taskLine.entryId && taskLine.text !== ''}
        <Button size="sm" tone="subtle" icon="square-check" onclick={() => (isMakingTask = !isMakingTask)}>{t('app.pages.tool_task')}</Button>
      {/if}
    </Inline>
  </Toolbar>
{/if}

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
    <ul class="choices" aria-label={t('app.pages.tool_block')}>
      {#each ['info', 'warning', 'success', 'error'] as const as tone (tone)}
        <li><button type="button" class="choice" onclick={() => addBlock({ kind: 'callout', tone })}>{t(`app.pages.block_callout_${tone}`)}</button></li>
      {/each}
      <li><button type="button" class="choice" onclick={() => addBlock({ kind: 'details', summary: '' })}>{t('app.pages.block_toggle')}</button></li>
      <li><button type="button" class="choice" onclick={() => addBlock({ kind: 'toc' })}>{t('app.pages.block_toc')}</button></li>
      <li><button type="button" class="choice" onclick={() => addBlock({ kind: 'children' })}>{t('app.pages.block_child_pages')}</button></li>
      <li><button type="button" class="choice" onclick={() => addBlock({ kind: 'date', date: today() })}>{t('app.pages.block_date')}</button></li>
    </ul>
    <form class="block-form" onsubmit={(event) => { event.preventDefault(); addBlock({ kind: 'status', label: statusLabel, tone: 'info' }); }}>
      <input type="text" bind:value={statusLabel} maxlength="40" placeholder={t('app.pages.block_status_label')} aria-label={t('app.pages.block_status_label')} />
      <Button size="sm" tone="subtle" type="submit" disabledReason={statusLabel.trim() === '' ? t('app.pages.block_status_label') : undefined}>{t('app.pages.block_status')}</Button>
    </form>
    <form class="block-form" onsubmit={(event) => { event.preventDefault(); addBlock({ kind: 'embed', src: embedAddress }); }}>
      <input type="url" bind:value={embedAddress} placeholder="https://" aria-label={t('app.pages.block_embed_address')} />
      <Button size="sm" tone="subtle" type="submit" disabledReason={embedAddress.trim() === '' ? t('app.pages.block_embed_address') : undefined}>{t('app.pages.block_embed_add')}</Button>
    </form>
  </div>
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

<p class="status" aria-live="polite">{statusWords}</p>
{#if saveRefusal}<p class="refusal" role="alert">{saveRefusal}</p>{/if}

<div class="editor" bind:this={host}></div>

<MentionDialog bind:isOpen={isMentioning} selfId="" oninsert={insertAddressText} />

<style>
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
  .upload {
    display: inline-flex;
    align-items: center;
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm);
    color: var(--text-primary);
    font-size: var(--fs-075);
    cursor: pointer;
  }
  .upload:hover { background: var(--bg-surface-hover); }
  .upload:focus-within { outline: var(--bw-ring) solid var(--focus-ring); }
  .upload input { inline-size: 0; block-size: 0; opacity: 0; }

  .editor :global(.page-body) {
    min-block-size: var(--sp-1000);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-primary);
    line-height: var(--lh-normal);
    outline: none;
  }
  .editor :global(.page-body:focus-visible) { outline: var(--bw-ring) solid var(--focus-ring); }
  .editor :global(.page-body h1),
  .editor :global(.page-body h2),
  .editor :global(.page-body h3) { font-family: var(--font-display); line-height: var(--lh-tight); }
  .editor :global(.page-body pre) {
    padding: var(--sp-100);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    font-family: var(--font-mono);
  }
  .editor :global(.page-body code) { font-family: var(--font-mono); }
  .editor :global(.page-body blockquote) {
    margin-inline: 0;
    padding-inline-start: var(--sp-150);
    border-inline-start: var(--bw-thick) solid var(--border-default);
    color: var(--text-secondary);
  }
  .editor :global(.page-body ul[data-type='taskList']) { list-style: none; padding-inline-start: var(--sp-050); }
  .editor :global(.page-body ul[data-type='taskList'] li) { display: flex; gap: var(--sp-100); align-items: baseline; }
  .editor :global(.page-mention) {
    padding-inline: var(--sp-025);
    border-radius: var(--r-xs);
    background: var(--accent-primary-subtle);
    color: var(--text-brand);
    cursor: pointer;
  }
  .editor :global(.page-card) {
    margin-block: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    font-weight: var(--fw-medium);
    cursor: pointer;
  }
  .editor :global(.page-card[data-kind='view']),
  .editor :global(.page-card[data-kind='dashboard']),
  .editor :global(.page-card[data-kind='whiteboard']) { min-block-size: var(--sp-1000); }
  .editor :global(.page-card--live) { padding: 0; border: none; background: none; cursor: default; }
  .blocks { display: grid; gap: var(--sp-100); }
  .block-form { display: flex; gap: var(--sp-100); align-items: center; }
  .block-form input {
    flex: 1;
    min-inline-size: 0;
    padding: var(--sp-050) var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
  }
  .editor :global(.page-callout) {
    margin-block: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    border-inline-start: var(--bw-thick) solid var(--status-info-accent);
    border-radius: var(--r-sm);
    background: var(--status-info-surface);
  }
  .editor :global(.page-callout--warning) { border-inline-start-color: var(--status-warning-accent); background: var(--status-warning-surface); }
  .editor :global(.page-callout--success) { border-inline-start-color: var(--status-success-accent); background: var(--status-success-surface); }
  .editor :global(.page-callout--error) { border-inline-start-color: var(--status-danger-accent); background: var(--status-danger-surface); }
  .editor :global(.page-details) {
    margin-block: var(--sp-100);
    padding: var(--sp-050) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
  }
  .editor :global(.page-details__summary) { font-weight: var(--fw-medium); cursor: pointer; }
  .editor :global(.page-details__input) {
    inline-size: calc(100% - var(--sp-300));
    border: none;
    background: none;
    color: var(--text-primary);
    font: inherit;
  }
  .editor :global(.page-toc),
  .editor :global(.page-children) {
    margin-block: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
  }
  .editor :global(.page-toc__list),
  .editor :global(.page-children__list) { margin: 0; padding-inline-start: var(--sp-200); }
  .editor :global(.page-toc__item--3),
  .editor :global(.page-toc__item--4),
  .editor :global(.page-toc__item--5),
  .editor :global(.page-toc__item--6) { margin-inline-start: var(--sp-200); }
  .editor :global(.page-toc__link),
  .editor :global(.page-children__link) {
    padding: 0;
    border: none;
    background: none;
    color: var(--text-brand);
    font: inherit;
    cursor: pointer;
  }
  .editor :global(.page-embed__link) {
    display: flex;
    gap: var(--sp-100);
    margin-block: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    text-decoration: none;
  }
  .editor :global(.page-embed__host) { color: var(--text-secondary); }
  .editor :global(.page-embed__frame) {
    display: block;
    inline-size: 100%;
    aspect-ratio: 16 / 9;
    margin-block: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
  }
  .editor :global(.page-date),
  .editor :global(.page-status) {
    padding-inline: var(--sp-050);
    border-radius: var(--r-xs);
    background: var(--bg-surface-sunken);
    font-size: var(--fs-075);
  }
  .editor :global(.page-status) { font-weight: var(--fw-medium); text-transform: uppercase; }
  .editor :global(.page-status--info) { background: var(--status-info-surface); color: var(--status-info-text); }
  .editor :global(.page-status--success) { background: var(--status-success-surface); color: var(--status-success-text); }
  .editor :global(.page-status--warning) { background: var(--status-warning-surface); color: var(--status-warning-text); }
  .editor :global(.page-status--error) { background: var(--status-danger-surface); color: var(--status-danger-text); }
  .editor :global(.page-passage) {
    background: var(--label-amber-bg);
    border-block-end: var(--bw-thick) solid var(--label-amber-fg);
  }
  .editor :global(.page-caret) {
    position: relative;
    margin-inline: calc(-1 * var(--bw-hairline));
    border-inline-start: var(--bw-thick) solid currentColor;
    pointer-events: none;
  }
  .editor :global(.page-caret__label) {
    position: absolute;
    inset-block-end: 100%;
    inset-inline-start: calc(-1 * var(--bw-thick));
    padding-inline: var(--sp-025);
    border-radius: var(--r-xs);
    background: currentColor;
    font-size: var(--fs-050);
    white-space: nowrap;
  }
  .editor :global(.page-caret__label)::first-line { color: var(--text-inverse); }
  .editor :global(.page-caret--0) { color: var(--label-blue-fg); }
  .editor :global(.page-caret--1) { color: var(--label-green-fg); }
  .editor :global(.page-caret--2) { color: var(--label-violet-fg); }
  .editor :global(.page-caret--3) { color: var(--label-orange-fg); }
  .editor :global(.page-caret--4) { color: var(--label-teal-fg); }
  .editor :global(.page-caret--5) { color: var(--label-magenta-fg); }
  .editor :global(.page-caret--6) { color: var(--label-amber-fg); }
  .editor :global(.page-caret--7) { color: var(--label-red-fg); }
</style>
