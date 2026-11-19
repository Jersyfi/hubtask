<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A view or a whiteboard living in a page (S-4): the `LiveBlock` head - tile, what it is, *Live*
  // and the way to the original - over the board drawn live. Its title follows the references'
  // resolver, so a renamed view reads as it is called now; a reader who may not see the original
  // gets no way to it (P-19).

  import { untrack } from 'svelte';
  import { LiveBlock, StateLozenge } from '@hubtask/design-system/components';
  import type { ReferenceAttrs, ReferenceDrawing } from '@hubtask/editor';
  import type { ItemQueryResult, SavedView, WorkItem } from '@hubtask/sync-engine';

  import BoardView from '../whiteboards/BoardView.svelte';
  import { engine } from '../data/engine.ts';
  import { liveViewReadOf } from '../data/views.ts';
  import { workflows } from '../data/workflows.svelte.ts';
  import { statusNameOf } from '../workspace/happened.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    ref: ReferenceAttrs;
    drawing: ReferenceDrawing;
    /** Whether the reader may open the original. */
    canOpen: () => boolean;
    onnavigate: (path: string) => void;
  }

  const { ref, drawing, canOpen, onnavigate }: Props = $props();

  const isBoard = $derived(ref.kind.toUpperCase() === 'WHITEBOARD');
  let title = $state<string | undefined>(undefined);
  let mayOpen = $state(false);
  $effect(() => {
    const read = () => {
      title = drawing.label(ref);
      mayOpen = canOpen();
    };
    read();
    return drawing.subscribe(read);
  });

  // A view's rows: its stored question read as the reader, so only what they may see (S-4).
  const ROWS = 5;
  let stored = $state<Record<string, unknown> | undefined>(undefined);
  let rows = $state<readonly WorkItem[] | undefined>(undefined);
  let total = $state<number | undefined>(undefined);
  $effect(() => {
    if (isBoard) return;
    const id = ref.id;
    return untrack(() => engine.subscribe<SavedView>({ path: `/views/${id}` }, (next) => {
      if (next.status === 'ready') stored = next.data.query as Record<string, unknown>;
      else if (next.status === 'failed') rows = [];
    }));
  });
  $effect(() => {
    const body = liveViewReadOf(stored, ROWS);
    if (!body) return;
    return untrack(() => engine.subscribe<ItemQueryResult>({ path: '/items:query', body }, (next) => {
      if (next.status === 'ready') {
        rows = next.data.data;
        total = next.data.total ?? undefined;
      } else if (next.status === 'failed') rows = [];
    }));
  });
  $effect(() => untrack(() => workflows.open()));
  const statusOf = (item: WorkItem) =>
    item.status ? statusNameOf({ statusKey: item.status, collectionId: item.collection_id, entryType: item.entry_type }, '', workflows.list, t) : undefined;
</script>

<LiveBlock
  title={title ?? t('app.pages.reference_loading')}
  icon={isBoard ? 'layout-template' : 'square-check'}
  product={isBoard ? 'whiteboards' : undefined}
  liveLabel={t('app.pages.live')}
  openLabel={mayOpen ? t(isBoard ? 'app.pages.live_open_board' : 'app.pages.live_open_view') : undefined}
  onopen={() => drawing.open?.(ref)}
>
  {#if isBoard}
    <BoardView boardId={ref.id} frame={ref.anchor} onopen={onnavigate} isUnderHead />
  {:else if rows && rows.length > 0}
    <ul class="rows" data-live-rows>
      {#each rows as item (item.id)}
        {@const status = statusOf(item)}
        <li>
          <a class="row" href={`/items/${item.id}`} onclick={(event) => { event.preventDefault(); onnavigate(`/items/${item.id}`); }}>
            <span class="title">{item.title}</span>
            {#if status}<StateLozenge label={status} size="sm" />{/if}
          </a>
        </li>
      {/each}
    </ul>
    {#if total !== undefined && total > rows.length}<p class="more">{t('app.pages.live_more', { count: total - rows.length })}</p>{/if}
  {:else if rows}
    <p class="more">{t('app.pages.live_none')}</p>
  {/if}
</LiveBlock>

<style>
  .rows { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); }
  .rows li + li { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-decoration: none;
  }
  .row:hover { background: var(--bg-surface-hover); }
  .title { flex: 1; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .more { margin: 0; padding: var(--sp-100) var(--sp-150); color: var(--text-subtle); font-size: var(--fs-075); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .row { min-block-size: var(--layout-target); }
  }
</style>
