<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Whiteboards* in a hub (M4, UC-CNV-01 … UC-CNV-03): the hub's boards, the last changed first,
  // and one board drawn (`BoardCanvas`). A board is made empty or from an exported native document,
  // renamed, put in the trash and brought back.
  //
  // **What the reader may never do is not drawn** (P-05): a reader without WRITE_ITEMS on the hub
  // sees the boards and is offered no write; the board's own `writable` says the same for it.

  import { untrack } from 'svelte';
  import { Banner, Button, Dialog, EmptyState, IconButton, Inline, Input, Spinner, Stack } from '@hubtask/design-system/components';
  import type { Whiteboard, WhiteboardList, WhiteboardRead } from '@hubtask/sync-engine';

  import { actor } from '../lib/data/account.svelte.ts';
  import { holds, roleOrder } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { boardPath, boardsPath, readNative, trashedBoardsPath, whiteboards } from '../lib/data/whiteboards.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import BoardCanvas from '../lib/whiteboards/BoardCanvas.svelte';

  interface Props {
    /** The hub whose boards are listed; absent when a board's address names only the board. */
    hubId?: string;
    boardId?: string;
    onnavigate: (path: string) => void;
  }

  const { hubId: hubFromRoute, boardId, onnavigate }: Props = $props();

  const opened = untrack(() => (boardId ? resource<WhiteboardRead>(boardPath(boardId)) : undefined));
  const current = $derived(opened?.state.status === 'ready' ? opened.state.data : undefined);
  const openRefusal = $derived(opened?.state.status === 'failed' ? renderProblem(opened.state.error, messages).message : undefined);
  const hubId = $derived(hubFromRoute ?? current?.hub_id ?? '');
  const hub = $derived(hubId ? containers.find(hubId) : undefined);

  $effect(() => frame.entitle(current?.title ?? t('app.whiteboards.title')));

  let list = $state<readonly Whiteboard[] | undefined>(undefined);
  let trash = $state<readonly Whiteboard[]>([]);
  let listRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    if (!at || boardId) return;
    return untrack(() => {
      const stops = [
        engine.subscribe<WhiteboardList>({ path: boardsPath(at) }, (next) => {
          if (next.status === 'ready') list = next.data.data;
          if (next.status === 'failed') listRefusal = renderProblem(next.error, messages).message;
        }),
        engine.subscribe<WhiteboardList>({ path: trashedBoardsPath(at) }, (next) => {
          if (next.status === 'ready') trash = next.data.data;
        }),
      ];
      people.open({ hubId: at });
      return () => stops.forEach((stop) => stop());
    });
  });

  const mayWrite = $derived(
    hubId ? holds(people.roleOf({ hubId }, actor.account?.id, roleOrder()) ?? undefined, 'WRITE_ITEMS').status === 'permitted' : false,
  );

  let isCreating = $state(false);
  let titleDraft = $state('');
  let imported = $state<{ title: string; elements: unknown[] } | undefined>(undefined);
  let renaming = $state(false);
  let showsTrash = $state(false);

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

  async function create() {
    const title = titleDraft.trim();
    if (!title || !hubId) return;
    const made = await act(() => whiteboards.create(hubId, { title, ...(imported ? { elements: imported.elements as never } : {}) }));
    isCreating = false;
    imported = undefined;
    titleDraft = '';
    if (made) onnavigate(`/whiteboards/${made.id}`);
  }

  async function readFile(event: Event) {
    const file = (event.currentTarget as HTMLInputElement).files?.[0];
    if (!file) return;
    const native = readNative(await file.text());
    if (!native) {
      toast.show({ text: t('app.whiteboards.import_not_native'), tone: 'danger' });
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

  const when = (at: string) => formatDateTime(at, messages.locale);
  const collections = $derived(hubId ? containers.collectionsOf(hubId).map((each) => ({ id: each.id, name: each.name })) : []);
</script>

{#if boardId}
  <div class="one">
    {#if openRefusal || !current}
      <h1 class="title">{t('app.whiteboards.title')}</h1>
    {/if}
    {#if openRefusal}
      <Banner tone="danger">{openRefusal}</Banner>
    {:else if !current}
      <Spinner label={t('app.whiteboards.loading')} />
    {:else}
      <Inline justify="between" align="center">
        <Stack gap="025">
          {#if hub}<a class="hub" href={`/hubs/${hub.id}/whiteboards`}>{hub.name} · {t('app.whiteboards.title')}</a>{/if}
          <h1 class="title">{current.title}</h1>
        </Stack>
        {#if current.writable}
          <Inline gap="050">
            <IconButton icon="pencil" label={t('app.whiteboards.rename')} onclick={() => { titleDraft = current.title; renaming = true; }} />
            <IconButton icon="trash" label={t('app.whiteboards.trash')} onclick={() => void trashBoard(current)} />
          </Inline>
        {/if}
      </Inline>
      {#key current.id}
        <BoardCanvas board={current} {collections} {onnavigate} />
      {/key}
    {/if}
  </div>
{:else}
  <div class="list">
    <Inline justify="between" align="center">
      <Stack gap="025">
        {#if hub}<a class="hub" href={`/hubs/${hub.id}`}>{hub.name}</a>{/if}
        <h1 class="title">{t('app.whiteboards.title')}</h1>
      </Stack>
      {#if mayWrite}<Button icon="plus" onclick={() => (isCreating = true)}>{t('app.whiteboards.new')}</Button>{/if}
    </Inline>
    {#if listRefusal}
      <Banner tone="danger">{listRefusal}</Banner>
    {:else if !list}
      <Spinner label={t('app.whiteboards.loading')} />
    {:else if list.length === 0}
      <EmptyState kind="unused" title={t('app.whiteboards.empty')} description={t('app.whiteboards.empty_hint')} icon="layout-template">
        {#snippet action()}
          {#if mayWrite}<Button icon="plus" onclick={() => (isCreating = true)}>{t('app.whiteboards.new')}</Button>{/if}
        {/snippet}
      </EmptyState>
    {:else}
      <ul class="boards">
        {#each list as board (board.id)}
          <li class="row">
            <a href={`/whiteboards/${board.id}`}>{board.title}</a>
            <span class="meta">{t('app.whiteboards.objects', { count: String(board.element_count) })} · {when(board.updated_at)}</span>
          </li>
        {/each}
      </ul>
    {/if}
    {#if mayWrite}
      <div>
        <Button size="sm" tone="subtle" icon="trash" onclick={() => (showsTrash = !showsTrash)}>{t('app.whiteboards.trash_list')}</Button>
        {#if showsTrash}
          <ul class="boards">
            {#each trash as board (board.id)}
              <li class="row">
                <span>{board.title}</span>
                <Button size="sm" tone="subtle" icon="rotate-ccw" onclick={() => void act(() => whiteboards.restore(board), t('app.whiteboards.restored', { title: board.title }))}>{t('app.whiteboards.restore')}</Button>
              </li>
            {:else}
              <li class="meta">{t('app.whiteboards.trash_empty')}</li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
  </div>
{/if}

{#if isCreating}
  <Dialog bind:isOpen={isCreating} title={t('app.whiteboards.new')} dismissLabel={t('app.links.cancel')} onClose={() => { imported = undefined; titleDraft = ''; }}>
    <Stack gap="150">
      <Input label={t('app.whiteboards.board_title')} bind:value={titleDraft} maxlength={200} />
      <label class="file">
        <span>{t('app.whiteboards.import')}</span>
        <input type="file" accept="application/json,.json" onchange={readFile} />
      </label>
      {#if imported}<p class="meta">{t('app.whiteboards.import_ready', { count: String(imported.elements.length) })}</p>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isCreating = false)}>{t('app.links.cancel')}</Button>
      <Button tone="primary" disabledReason={titleDraft.trim() ? undefined : t('app.whiteboards.needs_title')} onclick={() => void create()}>{t('app.whiteboards.create')}</Button>
    {/snippet}
  </Dialog>
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
  .one, .list { display: grid; gap: var(--sp-150); min-block-size: 0; }
  .one { grid-template-rows: auto minmax(0, 1fr); block-size: 100%; }
  .title { margin: 0; font-size: var(--fs-400); font-weight: var(--fw-semibold); }
  .hub { color: var(--text-secondary); font-size: var(--fs-075); }
  .boards { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .row a { color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  .row a:hover { text-decoration: underline; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .file { display: grid; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-075); }
</style>
