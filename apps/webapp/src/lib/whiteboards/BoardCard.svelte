<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One board as a preview card (user-experience.md §2.2, §7.2): a drawing of its objects
  // (`BoardThumbnail`, W-7), its name, one quiet line and who made and last changed it.

  import { AvatarGroup } from '@hubtask/design-system/components';
  import type { Whiteboard } from '@hubtask/sync-engine';

  import BoardThumbnail from './BoardThumbnail.svelte';

  interface Props {
    board: Whiteboard;
    /** One quiet line: the hub, the count, when it changed. */
    meta: string;
    /** Who made and last changed it, by name (scene whome 2). */
    people?: readonly string[];
    onopen: (path: string) => void;
  }

  const { board, meta, people = [], onopen }: Props = $props();

  const href = $derived(`/whiteboards/${board.id}`);
</script>

<a
  class="card"
  {href}
  data-board-card
  onclick={(event) => { if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return; event.preventDefault(); onopen(href); }}
>
  <BoardThumbnail boardId={board.id} count={board.element_count} />
  <span class="title" title={board.title}>{board.title}</span>
  <span class="meta">{meta}</span>
  {#if people.length > 0}<span class="people"><AvatarGroup people={people.map((name) => ({ name }))} max={3} size="sm" /></span>{/if}
</a>

<style>
  .card {
    display: grid;
    grid-template-rows: auto auto auto auto;
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
  .title { font-weight: var(--fw-semibold); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { color: var(--text-secondary); font-size: var(--fs-075); }
  .people { display: flex; }
</style>
