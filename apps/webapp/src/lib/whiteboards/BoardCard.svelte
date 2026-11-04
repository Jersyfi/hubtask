<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One board as a preview card (user-experience.md §2.2, §7.2): a drawing of its objects, its
  // name and one quiet line. A list's rows carry no objects, so the drawing reads the board itself,
  // only once the card is on the screen and only for a board small enough that a thumbnail costs
  // less than the board; a larger or empty one shows its mark instead.

  import { untrack } from 'svelte';
  import { Icon } from '@hubtask/design-system/components';
  import type { Whiteboard, WhiteboardRead } from '@hubtask/sync-engine';
  import type { Element } from '@hubtask/whiteboard';

  import { engine } from '../data/engine.ts';
  import { boardPath } from '../data/whiteboards.svelte.ts';
  import BoardThumb from './BoardThumb.svelte';

  /** The most objects a card reads its board for. */
  const PREVIEW_MOST = 1500;

  interface Props {
    board: Whiteboard;
    /** One quiet line: the hub, the count, when it changed. */
    meta: string;
    onopen: (path: string) => void;
  }

  const { board, meta, onopen }: Props = $props();

  let card = $state<HTMLAnchorElement | undefined>(undefined);
  let seen = $state(false);
  let elements = $state<readonly Element[] | undefined>(undefined);
  const wanted = $derived(board.element_count > 0 && board.element_count <= PREVIEW_MOST);

  $effect(() => {
    if (!card || !wanted || seen) return;
    if (typeof IntersectionObserver === 'undefined') { seen = true; return; }
    // design-system-lint-ignore: a reading margin for the observer, not a drawn length.
    const watch = new IntersectionObserver((entries) => { if (entries.some((each) => each.isIntersecting)) seen = true; }, { rootMargin: '200px' });
    watch.observe(card);
    return () => watch.disconnect();
  });

  $effect(() => {
    if (!seen || !wanted) return;
    const path = boardPath(board.id);
    return untrack(() => engine.subscribe<WhiteboardRead>({ path }, (next) => {
      if (next.status === 'ready') elements = (next.data.elements ?? []) as Element[];
    }));
  });

  const href = $derived(`/whiteboards/${board.id}`);
</script>

<a
  class="card"
  {href}
  bind:this={card}
  data-board-card
  onclick={(event) => { if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return; event.preventDefault(); onopen(href); }}
>
  <span class="preview" aria-hidden="true">
    {#if elements && elements.length > 0}
      <BoardThumb {elements} />
    {:else}
      <span class="mark"><Icon name="layout-template" size="md" /></span>
    {/if}
  </span>
  <span class="title" title={board.title}>{board.title}</span>
  <span class="meta">{meta}</span>
</a>

<style>
  .card {
    display: grid;
    grid-template-rows: auto auto auto;
    gap: var(--sp-050);
    block-size: 100%;
    box-sizing: border-box;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-primary);
    text-decoration: none;
    box-shadow: var(--shadow-raised);
    transition: box-shadow var(--motion-state-duration) var(--motion-state-easing), border-color var(--motion-state-duration) var(--motion-state-easing);
  }
  .card:hover { border-color: var(--border-default); box-shadow: var(--shadow-overlay); }
  .card:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--bw-ring); }
  .preview {
    display: grid;
    place-items: center;
    block-size: var(--sp-1600);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    overflow: hidden;
  }
  .mark { color: var(--text-subtle); }
  .title { font-weight: var(--fw-semibold); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
