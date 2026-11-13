<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One anatomy for Today and the hub overview (work-surface concept §5, §6.1): a tile, a title,
  // a figure, the content, and one act - a button, never an underlined link. A card of the strip
  // on Today is a widget with a figure and a line; a tile of the overview one with content. A
  // widget whose module is off is not drawn at all (P-17); one still loading keeps its place.
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import StateLozenge from './StateLozenge.svelte';
  import ProductTile, { type ProductId } from './ProductTile.svelte';
  import Skeleton from './Skeleton.svelte';
  import type { IconName } from './icons/index.ts';

  interface Props {
    title: string;
    /** The tile before the title: a product's colour, or a label token's for a tool. */
    tile?: { icon: IconName; product?: ProductId; color?: string };
    /** The one figure: *3:10*, *4*, *50 %*. */
    figure?: string;
    /**
     * The figure says something is wrong: overdue, breached. A lozenge in the tone stands beside
     * it with its glyph and word, so it never rests on colour alone (UX-7): *overdue*.
     */
    figureTone?: { tone: 'danger' | 'warning'; word: string };
    /** A quiet figure at the end of the head: *6*, *5 today · 1 overdue*. */
    count?: string;
    /** One line under the figure: *of 7:30 h · timer running*. */
    line?: string;
    /** The one act: *Show*, *Open*, *Stop*. */
    act?: { label: string; onclick: () => void };
    /** The content is on its way: its place is held by lines, so nothing below moves. */
    isBusy?: boolean;
    busyLabel?: string;
    children?: Snippet;
  }

  const { title, tile, figure, figureTone, count, line, act, isBusy = false, busyLabel, children }: Props = $props();
  const uid = $props.id();
</script>

<section class="widget" data-figure={figure !== undefined ? '' : undefined} aria-labelledby={`widget-${uid}`} aria-busy={isBusy ? 'true' : undefined}>
  <header class="head">
    {#if tile}<ProductTile icon={tile.icon} product={tile.product} color={tile.color} size="md" />{/if}
    <h2 class="title" id={`widget-${uid}`}>{title}</h2>
    {#if count}<span class="count">{count}</span>{/if}
    {#if act}<span class="act"><Button tone="subtle" size="sm" onclick={act.onclick}>{act.label}</Button></span>{/if}
  </header>
  {#if isBusy}
    <div class="busy" role="status" aria-label={busyLabel}><Skeleton lines={figure !== undefined ? 2 : 3} shape="text" /></div>
  {:else}
    {#if figure !== undefined}
      <span class="figure-line" data-tone={figureTone?.tone}>
        <span class="figure">{figure}</span>
        {#if figureTone}<StateLozenge label={figureTone.word} tone={figureTone.tone} icon="triangle-alert" />{/if}
      </span>
    {/if}
    {#if line}<span class="line">{line}</span>{/if}
    {#if children}<div class="content">{@render children()}</div>{/if}
  {/if}
</section>

<style>
  .widget {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    min-inline-size: 0;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  /* A card of Today's strip: tighter, the figure under the head. */
  .widget[data-figure] { gap: var(--sp-050); padding: var(--sp-150); }

  .head { display: flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  .title {
    flex: 1;
    min-inline-size: 0;
    margin: 0;
    font-size: var(--fs-090);
    font-weight: var(--fw-semibold);
    /* Whole words only: a title that does not fit wraps between them, never inside one. */
    overflow-wrap: normal;
    word-break: normal;
    text-wrap: balance;
  }

  .widget[data-figure] .title { color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-regular); }

  .count { color: var(--text-subtle); font-size: var(--fs-075); white-space: nowrap; }
  .act { flex: none; }

  .figure {
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    font-variant-numeric: tabular-nums;
    line-height: var(--lh-tight);
  }

  .figure-line { display: flex; align-items: center; gap: var(--sp-100); flex-wrap: wrap; }

  .line { color: var(--text-secondary); font-size: var(--fs-075); }
  .content { display: flex; flex-direction: column; gap: var(--sp-100); min-inline-size: 0; }
</style>
