<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script module lang="ts">
  /** The eight products with a colour of their own (`product.<id>` in tokens.json, WB-2). */
  export type ProductId = 'plans' | 'pages' | 'whiteboards' | 'desk' | 'assets' | 'time' | 'goals' | 'agents';
</script>

<script lang="ts">
  // A product's mark, or a place's chosen symbol, on its label tile (design-system.md §4, wave 5;
  // §11.3): the label's measured pair, its ink on its tile, so the symbol keeps the contrast the
  // label test holds it to in both modes. Never alone: a name stands beside it, so it is hidden
  // from the accessibility tree.
  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';
  import { labelTokens, type LabelToken } from '../dist/tokens.ts';

  interface Props {
    icon: IconName;
    /** One of the ten label tokens, or `brand`; anything else draws the plain mark without a tile. */
    color?: string;
    /**
     * A product's own colour through its alias token, which wins over `color`: inside a product
     * the tile names the product, so no component picks its label colour itself.
     */
    product?: ProductId;
    /**
     * `sm` is the breadcrumb's step and an object's key; `md` the sidebar's mark cell; `lg` a
     * widget's head; `xl` a product home's band.
     */
    size?: 'sm' | 'md' | 'lg' | 'xl';
  }

  const { icon, color, product, size = 'md' }: Props = $props();
  const token = $derived(color === 'brand' || (labelTokens as readonly string[]).includes(color ?? '') ? (color as LabelToken | 'brand') : undefined);
</script>

<span class="tile" data-token={product ? undefined : token} data-product={product} data-size={size} aria-hidden="true"><Icon name={icon} size="sm" /></span>

<style>
  .tile {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--layout-nav-mark);
    block-size: var(--layout-nav-mark);
    border-radius: var(--r-sm);
    background: var(--tile-bg, transparent);
    color: var(--tile-fg, var(--text-secondary));
  }

  .tile :global(.icon) {
    inline-size: var(--layout-nav-tileglyph);
    block-size: var(--layout-nav-tileglyph);
  }

  .tile[data-size='sm'] {
    inline-size: var(--sp-200);
    block-size: var(--sp-200);
    border-radius: var(--r-xs);
  }

  .tile[data-size='sm'] :global(.icon) {
    inline-size: var(--fs-050);
    block-size: var(--fs-050);
  }

  .tile[data-size='lg'] {
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border-radius: var(--r-md);
  }

  .tile[data-size='lg'] :global(.icon) {
    inline-size: var(--sp-200);
    block-size: var(--sp-200);
  }

  .tile[data-size='xl'] {
    inline-size: var(--sp-500);
    block-size: var(--sp-500);
    border-radius: var(--r-lg);
  }

  .tile[data-size='xl'] :global(.icon) {
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
  }

  .tile[data-product='plans'] { --tile-bg: var(--product-plans-surface); --tile-fg: var(--product-plans-ink); }
  .tile[data-product='pages'] { --tile-bg: var(--product-pages-surface); --tile-fg: var(--product-pages-ink); }
  .tile[data-product='whiteboards'] { --tile-bg: var(--product-whiteboards-surface); --tile-fg: var(--product-whiteboards-ink); }
  .tile[data-product='desk'] { --tile-bg: var(--product-desk-surface); --tile-fg: var(--product-desk-ink); }
  .tile[data-product='assets'] { --tile-bg: var(--product-assets-surface); --tile-fg: var(--product-assets-ink); }
  .tile[data-product='time'] { --tile-bg: var(--product-time-surface); --tile-fg: var(--product-time-ink); }
  .tile[data-product='goals'] { --tile-bg: var(--product-goals-surface); --tile-fg: var(--product-goals-ink); }
  .tile[data-product='agents'] { --tile-bg: var(--product-agents-surface); --tile-fg: var(--product-agents-ink); }

  /* The work of a collection is the reader's own, not a product's: the brand's tint. */
  .tile[data-token='brand'] { --tile-bg: var(--accent-primary-subtle); --tile-fg: var(--text-brand); }

  .tile[data-token='slate'] { --tile-bg: var(--label-slate-bg); --tile-fg: var(--label-slate-fg); }
  .tile[data-token='blue'] { --tile-bg: var(--label-blue-bg); --tile-fg: var(--label-blue-fg); }
  .tile[data-token='teal'] { --tile-bg: var(--label-teal-bg); --tile-fg: var(--label-teal-fg); }
  .tile[data-token='green'] { --tile-bg: var(--label-green-bg); --tile-fg: var(--label-green-fg); }
  .tile[data-token='lime'] { --tile-bg: var(--label-lime-bg); --tile-fg: var(--label-lime-fg); }
  .tile[data-token='amber'] { --tile-bg: var(--label-amber-bg); --tile-fg: var(--label-amber-fg); }
  .tile[data-token='orange'] { --tile-bg: var(--label-orange-bg); --tile-fg: var(--label-orange-fg); }
  .tile[data-token='red'] { --tile-bg: var(--label-red-bg); --tile-fg: var(--label-red-fg); }
  .tile[data-token='magenta'] { --tile-bg: var(--label-magenta-bg); --tile-fg: var(--label-magenta-fg); }
  .tile[data-token='violet'] { --tile-bg: var(--label-violet-bg); --tile-fg: var(--label-violet-fg); }
</style>
