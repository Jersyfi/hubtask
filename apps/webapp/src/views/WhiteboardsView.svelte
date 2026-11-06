<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Whiteboards* in a hub (M4, UC-CNV-01 … UC-CNV-03; user-experience.md §7.2): the hub's boards
  // as preview cards, the last changed first, and one board drawn full-bleed (`BoardCanvas`). A
  // board is made from the template gallery, blank or from an exported native document, renamed,
  // put in the trash and brought back.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // sees the boards and is offered no write; the board's own `writable` says the same for it.

  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import {
    Banner, Button, Dialog, Disclosure, EmptyState, Input, PageHeader, Skeleton, Spinner, Stack, UploadField,
  } from '@hubtask/design-system/components';
  import type { Whiteboard, WhiteboardList, WhiteboardRead } from '@hubtask/sync-engine';
  import { elementId } from '@hubtask/whiteboard';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { boardPath, boardsPath, placeCarried, readNative, whiteboards } from '../lib/data/whiteboards.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import BoardCanvas from '../lib/whiteboards/BoardCanvas.svelte';
  import BoardCard from '../lib/whiteboards/BoardCard.svelte';
  import BoardTemplates, { startElements, templateOf, type BoardStart, type RetroFormat } from '../lib/whiteboards/BoardTemplates.svelte';
  import PlaceStrip from '../lib/workspace/PlaceStrip.svelte';
  import CreateDialog from '../lib/product/CreateDialog.svelte';
  import { createPlaces } from '../lib/product/places.svelte.ts';

  interface Props {
    /** The hub whose boards are listed; absent when a board's address names only the board. */
    hubId?: string;
    boardId?: string;
    onnavigate: (path: string) => void;
    /** The home's create act asked for the create dialog (user-experience.md §5.2): opened once. */
    isCreateAsked?: boolean;
  }

  const { hubId: hubFromRoute, boardId, onnavigate, isCreateAsked = false }: Props = $props();

  const opened = untrack(() => (boardId ? resource<WhiteboardRead>(boardPath(boardId)) : undefined));
  const current = $derived(opened?.state.status === 'ready' ? opened.state.data : undefined);
  const openRefusal = $derived(opened?.state.status === 'failed' ? renderProblem(opened.state.error, messages).message : undefined);
  const hubId = $derived(hubFromRoute ?? current?.hub_id ?? '');
  const hub = $derived(hubId ? containers.find(hubId) : undefined);

  $effect(() => frame.entitle(current?.title ?? t('app.whiteboards.title')));
  // A board is a canvas, not a document: it takes the content region whole (`page.fill`).
  $effect(() => (boardId ? frame.fill() : undefined));

  let list = $state<readonly Whiteboard[] | undefined>(undefined);
  let listRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    if (!at || boardId) return;
    return untrack(() => {
      const stop = engine.subscribe<WhiteboardList>({ path: boardsPath(at) }, (next) => {
        if (next.status === 'ready') list = next.data.data;
        if (next.status === 'failed') listRefusal = renderProblem(next.error, messages).message;
      });
      people.open({ hubId: at });
      return stop;
    });
  });
  /** Recent first, whatever order the list arrived in (§2.2). */
  const boards = $derived([...(list ?? [])].sort((a, b) => b.updated_at.localeCompare(a.updated_at)));

  const mayWrite = $derived(
    hubId ? holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted' : false,
  );

  // Opened once, when the reader's role here has answered that they may write.
  let isCreateOpened = false;
  $effect(() => {
    if (!isCreateAsked || isCreateOpened || !mayWrite) return;
    isCreateOpened = true;
    untrack(() => startCreate());
  });

  let isCreating = $state(false);
  let titleDraft = $state('');
  /** Where the board is made (§5.2): this hub, or another where whiteboards are on. */
  let draftHub = $state('');
  const placesToMake = createPlaces('WHITEBOARD', 'HUB', () => isCreating);
  let start = $state<BoardStart>('BLANK');
  let format = $state<RetroFormat>('RETRO_START_STOP_CONTINUE');
  let imported = $state<ReturnType<typeof readNative>>(undefined);
  let isImportOpen = $state(false);
  let renaming = $state(false);

  /** The name a template suggests, kept only while the person has not typed one of their own. */
  const suggested = (choice: BoardStart) => (choice === 'BLANK' ? '' : t(`app.whiteboards.start_${choice.toLowerCase()}`));
  let lastSuggested = '';
  $effect(() => {
    const name = suggested(start);
    untrack(() => {
      if (!imported && (titleDraft.trim() === '' || titleDraft === lastSuggested)) titleDraft = name;
      lastSuggested = name;
    });
  });

  function startCreate(choice: BoardStart = 'BLANK') {
    start = choice;
    titleDraft = suggested(choice);
    lastSuggested = titleDraft;
    draftHub = hubId;
    isCreating = true;
  }

  async function act<T>(run: () => Promise<T>, done?: string): Promise<T | undefined> {
    try {
      const out = await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    }
  }

  // A refusal while the dialog is open is said inside it, above its actions, and the dialog keeps
  // what was typed and chosen (user-experience.md §5.10).
  let createFailure = $state<string | undefined>(undefined);
  let importFailure = $state<string | undefined>(undefined);
  let isMaking = $state(false);

  async function create() {
    const title = titleDraft.trim();
    const at = draftHub || hubId;
    if (!title || !at || isMaking) return;
    createFailure = undefined;
    const name = templateOf(start, format);
    const elements = imported?.elements ?? (name ? startElements(name, (key) => t(`app.whiteboards.word_${key}`), elementId) : undefined);
    isMaking = true;
    try {
      const made = await whiteboards.create(at, { title, ...(elements && elements.length > 0 ? { elements: elements as never } : {}) });
      // An export's pictures and files are placed on the new board, or its readers read none.
      const failed = imported ? await placeCarried(imported.elements, (id) => whiteboards.placeMedia(made.id, id)) : [];
      if (failed.length > 0) toast.show({ text: t('app.whiteboards.carried_unplaced', { count: String(failed.length) }), tone: 'warning' });
      isCreating = false;
      imported = undefined;
      titleDraft = '';
      onnavigate(`/whiteboards/${made.id}`);
    } catch (error) {
      createFailure = renderProblem(error as never, messages).message;
    } finally {
      isMaking = false;
    }
  }

  async function readFile(file: File) {
    const native = readNative(await file.text());
    importFailure = native ? undefined : t('app.whiteboards.import_not_native');
    if (!native) {
      imported = undefined;
      return;
    }
    imported = native;
    titleDraft = native.title || titleDraft;
  }

  async function rename() {
    const board = current;
    const title = titleDraft.trim();
    renaming = false;
    if (board && title && title !== board.title) await act(() => whiteboards.rename(board, title));
  }

  async function trashBoard(board: Whiteboard) {
    const done = await act(() => whiteboards.trash(board), t('app.whiteboards.trashed', { title: board.title }));
    if (done && boardId) onnavigate(`/hubs/${board.hub_id}/whiteboards`);
  }

  const metaOf = (board: Whiteboard) =>
    [t('app.whiteboards.objects', { count: String(board.element_count) }), formatRelative(board.updated_at, messages.locale)].join(' · ');
  const collections = $derived(hubId ? containers.collectionsOf(hubId).map((each) => ({ id: each.id, name: each.name, settings: each.structure?.settings })) : []);
  const boardMenu = $derived(
    current?.writable
      ? {
          items: [
            { id: 'rename', label: t('app.whiteboards.rename'), icon: 'pencil' as const },
            { id: 'trash', label: t('app.whiteboards.trash'), icon: 'trash' as const, isDestructive: true },
          ],
          onselect: (id: string) => {
            if (!current) return;
            if (id === 'rename') { titleDraft = current.title; renaming = true; }
            if (id === 'trash') void trashBoard(current);
          },
        }
      : undefined,
  );
</script>

{#if boardId}
  {#if openRefusal || !current}
    <div class="waiting">
      <h1 class="title">{t('app.whiteboards.title')}</h1>
      {#if openRefusal}
        <FailureNote problem={openRefusal} />
      {:else}
        <Spinner label={t('app.whiteboards.loading')} />
      {/if}
    </div>
  {:else}
    {#key current.id}
      <BoardCanvas board={current} {collections} {onnavigate} {boardMenu}>
        {#snippet heading()}
          <div class="trail">
            {#if hub}<a class="hub" href={`/hubs/${hub.id}/whiteboards`}>{hub.name}</a><span class="sep" aria-hidden="true">/</span>{/if}
            <h1 class="board-title" title={current.title}>{current.title}</h1>
          </div>
        {/snippet}
      </BoardCanvas>
    {/key}
  {/if}
{:else}
  <Stack gap="300">
    <PageHeader
      title={t('app.whiteboards.title')}
      isTitleInBar={viewport.isCompact}
      breadcrumb={hub
        ? {
            trail: [{ id: 'place', label: hub.name, href: `/hubs/${hub.id}` }, { id: 'whiteboards', label: t('app.whiteboards.title') }],
            label: t('app.whiteboards.trail'),
            expandLabel: t('app.admin.expand_trail'),
            onnavigate: () => onnavigate(`/hubs/${hubId}`),
          }
        : undefined}
      primary={mayWrite ? { label: t('app.whiteboards.new'), icon: 'plus', onclick: () => startCreate() } : undefined}
      menu={mayWrite
        ? {
            label: t('app.whiteboards.list_menu'),
            // The one trash, narrowed to boards (user-experience.md §5.9): a hub keeps no list of its own.
            items: [{ id: 'trash', label: t('app.whiteboards.trash_list'), icon: 'trash' }],
            onselect: () => onnavigate('/trash?kind=WHITEBOARD'),
          }
        : undefined}
    />
    {#if hubFromRoute}<PlaceStrip placeId={hubFromRoute} current="whiteboards" {onnavigate} />{/if}

    {#if listRefusal}
      <FailureNote problem={listRefusal} />
    {:else if !list}
      <div aria-busy="true"><Skeleton lines={3} /></div>
    {:else if boards.length === 0}
      <!-- An empty hub starts a board from a template in one press (user-experience.md §4, §5.5). -->
      <div class="start" data-boards-start>
        {#if mayWrite}
          <EmptyState kind="unused" title={t('app.whiteboards.empty')} description={t('app.whiteboards.empty_hint')} icon="layout-template" />
          <BoardTemplates
            chosen={'BLANK'}
            {format}
            onpick={(choice) => startCreate(choice)}
          />
        {:else}
          <EmptyState kind="unused" title={t('app.whiteboards.empty')} description={t('app.whiteboards.empty_reader')} icon="layout-template" />
        {/if}
      </div>
    {:else}
      <ul class="boards" data-boards>
        {#each boards as board (board.id)}
          <li><BoardCard {board} meta={metaOf(board)} onopen={onnavigate} /></li>
        {/each}
      </ul>
    {/if}

  </Stack>
{/if}

{#if isCreating}
  <CreateDialog
    bind:isOpen={isCreating}
    title={t('app.whiteboards.new')}
    cancelLabel={t('app.links.cancel')}
    nameLabel={t('app.whiteboards.board_title')}
    bind:name={titleDraft}
    maxlength={200}
    isWide
    place={{ current: draftHub, fallback: hub?.name, places: placesToMake.list, onchoose: (id) => (draftHub = id) }}
    failure={createFailure}
    primary={{ label: t('app.whiteboards.create'), busyLabel: isMaking ? t('app.whiteboards.making') : undefined, disabledReason: titleDraft.trim() ? undefined : t('app.whiteboards.needs_title'), onclick: () => void create() }}
    onClose={() => { imported = undefined; titleDraft = ''; createFailure = undefined; importFailure = undefined; isImportOpen = false; }}
  >
    {#snippet gallery()}
      {#if imported}
        <Banner tone="info">{t('app.whiteboards.import_ready', { count: String(imported.elements.length) })}</Banner>
      {:else}
        <BoardTemplates bind:chosen={start} bind:format />
      {/if}
    {/snippet}
    <Disclosure label={t('app.whiteboards.import_fold')} description={t('app.whiteboards.import_fold_hint')} tone="quiet" bind:isOpen={isImportOpen}>
      <UploadField
        label={t('app.whiteboards.import')}
        chooseLabel={t('app.import.choose')}
        dropLabel={t('app.media.drop')}
        accept="application/json,.json"
        error={importFailure}
        onChoose={(file) => void readFile(file)}
      />
      {#if imported}<Button size="sm" tone="subtle" icon="x" onclick={() => (imported = undefined)}>{t('app.whiteboards.import_drop')}</Button>{/if}
    </Disclosure>
  </CreateDialog>
{/if}

{#if renaming}
  <Dialog bind:isOpen={renaming} title={t('app.whiteboards.rename')} dismissLabel={t('app.links.cancel')}>
    <Input label={t('app.whiteboards.board_title')} bind:value={titleDraft} maxlength={200} />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (renaming = false)}>{t('app.links.cancel')}</Button>
      <Button tone="primary" disabledReason={titleDraft.trim() ? undefined : t('app.whiteboards.needs_title')} onclick={() => void rename()}>{t('app.whiteboards.rename')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .waiting { display: grid; gap: var(--sp-150); padding: var(--sp-300); }
  .title { margin: 0; font-size: var(--fs-400); font-weight: var(--fw-semibold); }
  .trail { display: flex; align-items: baseline; gap: var(--sp-050); min-inline-size: 0; }
  .hub { flex: 0 1 auto; overflow: hidden; color: var(--text-secondary); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }
  .sep { color: var(--text-subtle); font-size: var(--fs-075); }
  .board-title { margin: 0; overflow: hidden; font-size: var(--fs-200); font-weight: var(--fw-semibold); text-overflow: ellipsis; white-space: nowrap; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .hub, .sep { display: none; }
  }
  .start { display: grid; gap: var(--sp-200); }
  .boards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr)); gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }
</style>
