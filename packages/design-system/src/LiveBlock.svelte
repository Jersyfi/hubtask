<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Work living in a page (S-4): an embedded view, board or entry under one head - the product's
  // tile, what it is, "Live" in words beside a mark (never the colour alone, UX-7) and the way to
  // the original. The content below is the caller's: rows of a view, a board's frame.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import ProductTile, { type ProductId } from './ProductTile.svelte';
  import type { IconName } from './icons/index.ts';

  interface Props {
    title: string;
    icon: IconName;
    /** The product the original lives in; its tile names it. */
    product?: ProductId;
    /** Where no product applies: one of the label tokens or `brand`. */
    color?: string;
    /** "Live", in the reader's words. */
    liveLabel: string;
    /** The way to the original, in words ("Open in tasks"); absent where the reader may not open it. */
    openLabel?: string;
    onopen?: () => void;
    children?: Snippet;
  }

  const { title, icon, product, color = 'brand', liveLabel, openLabel, onopen, children }: Props = $props();
</script>

<section class="live" aria-label={title} data-live-block>
  <div class="head">
    <ProductTile {icon} {product} {color} size="sm" />
    <span class="title">{title}</span>
    <span class="state"><span class="dot" aria-hidden="true"></span>{liveLabel}</span>
    {#if openLabel && onopen}
      <button type="button" class="open" onclick={onopen} data-live-open>{openLabel}<Icon name="chevron-right" size="sm" /></button>
    {/if}
  </div>
  {#if children}<div class="content">{@render children()}</div>{/if}
</section>

<style>
  .live {
    margin-block: var(--sp-150);
    overflow: hidden;
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
    align-items: center;
    padding: var(--sp-100) var(--sp-150);
    background: var(--bg-surface-sunken);
    font-size: var(--fs-100);
  }
  .title { font-weight: var(--fw-semibold); color: var(--text-primary); }
  .state { display: inline-flex; gap: var(--sp-050); align-items: center; color: var(--text-success); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .dot { inline-size: var(--sp-100); block-size: var(--sp-100); border-radius: var(--r-full); background: var(--status-success-accent); }
  .open {
    display: inline-flex;
    gap: var(--sp-025);
    align-items: center;
    margin-inline-start: auto;
    padding: var(--sp-025) var(--sp-050);
    border: none;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-brand);
    font: inherit;
    font-weight: var(--fw-medium);
    cursor: pointer;
  }
  .open:hover { background: var(--bg-surface-hover); }
  .open:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .content { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .open { position: relative; }
    .open::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: min(0%, calc((100% - var(--layout-target)) / 2)); }
  }
</style>
