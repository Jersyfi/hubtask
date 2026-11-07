<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One whiteboard drawn full-bleed (M4, UC-CNV-01 … UC-CNV-03; user-experience.md §7.2): renamed,
  // put in the trash and brought back from here. A hub's boards are listed and made in *All
  // contents* (the navigation concept §2.4), so this view opens one board only.
  //
  // **What the reader may never do is not drawn** (P-05): the board's own `writable` says whether
  // its menu offers a write.
  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { Button, Dialog, Input, Spinner } from '@hubtask/design-system/components';
  import type { Whiteboard, WhiteboardRead } from '@hubtask/sync-engine';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { boardPath, whiteboards } from '../lib/data/whiteboards.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import BoardCanvas from '../lib/whiteboards/BoardCanvas.svelte';
  import StarToggle from '../lib/workspace/StarToggle.svelte';

  interface Props {
    boardId: string;
    onnavigate: (path: string) => void;
  }

  const { boardId, onnavigate }: Props = $props();

  const opened = untrack(() => resource<WhiteboardRead>(boardPath(boardId)));
  const current = $derived(opened.state.status === 'ready' ? opened.state.data : undefined);
  const openRefusal = $derived(opened.state.status === 'failed' ? renderProblem(opened.state.error, messages).message : undefined);
  const hubId = $derived(current?.hub_id ?? '');
  // The sidebar shows this hub's level; the route names only the board (`levelOf`).
  $effect(() => frame.locate(hubId || undefined));
  const hub = $derived(hubId ? containers.find(hubId) : undefined);
  /** Where the hub's boards are listed. */
  const boardsOf = (at: string) => `/hubs/${at}/contents?kind=whiteboards`;

  $effect(() => frame.entitle(current?.title ?? t('app.whiteboards.title')));
  // A board is a canvas, not a document: it takes the content region whole (`page.fill`).
  $effect(() => frame.fill());

  let renaming = $state(false);
  let titleDraft = $state('');

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

  async function rename() {
    const board = current;
    const title = titleDraft.trim();
    renaming = false;
    if (board && title && title !== board.title) await act(() => whiteboards.rename(board, title));
  }

  async function trashBoard(board: Whiteboard) {
    const done = await act(() => whiteboards.trash(board), t('app.whiteboards.trashed', { title: board.title }));
    if (done) onnavigate(boardsOf(board.hub_id));
  }

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
          {#if hub}<a class="hub" href={boardsOf(hub.id)}>{hub.name}</a><span class="sep" aria-hidden="true">/</span>{/if}
          <h1 class="board-title" title={current.title}>{current.title}</h1>
          <StarToggle kind="WHITEBOARD" id={current.id} name={current.title} />
        </div>
      {/snippet}
    </BoardCanvas>
  {/key}
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
</style>
