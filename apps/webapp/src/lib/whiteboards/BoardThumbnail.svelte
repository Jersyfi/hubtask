<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A board's thumbnail drawn from its objects (W-7): a home's card, a search hit. A list's rows
  // carry no objects, so it reads the board itself, only once it is on the screen and only for a
  // board small enough that a thumbnail costs less than the board; a larger, empty or unread one
  // shows the product's mark instead. Decoration: the row or card around it says the name.

  import { untrack } from 'svelte';
  import { Icon } from '@hubtask/design-system/components';
  import type { WhiteboardRead } from '@hubtask/sync-engine';
  import type { Element } from '@hubtask/whiteboard';

  import { engine } from '../data/engine.ts';
  import { boardPath } from '../data/whiteboards.svelte.ts';
  import BoardThumb from './BoardThumb.svelte';

  /** The most objects a thumbnail reads its board for. */
  const PREVIEW_MOST = 1500;

  interface Props {
    boardId: string;
    /** How many objects the board holds, where the list says; unknown, the board is read. */
    count?: number;
    size?: 'card' | 'row';
  }

  const { boardId, count, size = 'card' }: Props = $props();

  let at = $state<HTMLElement | undefined>(undefined);
  let seen = $state(false);
  let elements = $state<readonly Element[] | undefined>(undefined);
  const wanted = $derived(count === undefined || (count > 0 && count <= PREVIEW_MOST));

  $effect(() => {
    if (!at || !wanted || seen) return;
    if (typeof IntersectionObserver === 'undefined') { seen = true; return; }
    // design-system-lint-ignore: a reading margin for the observer, not a drawn length.
    const watch = new IntersectionObserver((entries) => { if (entries.some((each) => each.isIntersecting)) seen = true; }, { rootMargin: '200px' });
    watch.observe(at);
    return () => watch.disconnect();
  });

  $effect(() => {
    if (!seen || !wanted) return;
    const path = boardPath(boardId);
    return untrack(() => engine.subscribe<WhiteboardRead>({ path }, (next) => {
      if (next.status === 'ready') elements = ((next.data.elements ?? []) as Element[]).length <= PREVIEW_MOST ? (next.data.elements ?? []) as Element[] : [];
    }));
  });
</script>

<span class="thumbnail" data-size={size} bind:this={at} aria-hidden="true" data-board-thumbnail={elements && elements.length > 0 ? 'drawn' : 'mark'}>
  {#if elements && elements.length > 0}
    <BoardThumb {elements} />
  {:else}
    <span class="mark"><Icon name="layout-template" size={size === 'row' ? 'sm' : 'md'} /></span>
  {/if}
</span>

<style>
  .thumbnail {
    display: grid;
    place-items: center;
    box-sizing: border-box;
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    overflow: hidden;
  }
  .thumbnail[data-size='card'] { block-size: var(--sp-1600); padding: var(--sp-050); }
  .thumbnail[data-size='row'] { flex: none; inline-size: var(--sp-1000); block-size: var(--sp-600); padding: var(--sp-025); border: var(--bw-hairline) solid var(--border-subtle); }
  .mark { color: var(--text-subtle); }
</style>
