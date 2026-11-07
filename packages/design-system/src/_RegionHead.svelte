<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The heading of a region, on the canvas above it (page-system §4.2 rules 2 and 3): the name,
  // a count or a state, the region's own acts at the end, a sentence under it. A part of the
  // compositions, never a box of its own.
  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    /** The heading's level; a region of a page is a second level. */
    level?: 2 | 3;
    id?: string;
    /** A count or a state after the name: *2*, *2 set here*. */
    note?: string;
    description?: string;
    acts?: Snippet;
  }

  const { title, level = 2, id, note, description, acts }: Props = $props();
</script>

<div class="region-head">
  <div class="line">
    <svelte:element this={`h${level}`} class="name" {id}>{title}</svelte:element>
    {#if note}<span class="note">{note}</span>{/if}
    {#if acts}<span class="acts">{@render acts()}</span>{/if}
  </div>
  {#if description}<p class="description">{description}</p>{/if}
</div>

<style>
  .region-head { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }
  .line { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--sp-100); }
  .name {
    margin: 0;
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }
  .note { color: var(--text-subtle); font-size: var(--fs-100); }
  .acts { display: inline-flex; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }
  .description { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); max-inline-size: 72ch; }
</style>
