<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
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
    /** One of the ten label tokens; anything else draws the plain mark without a tile. */
    color?: string;
    /** `sm` is the breadcrumb's step; `md` the sidebar's mark cell. */
    size?: 'sm' | 'md';
  }

  const { icon, color, size = 'md' }: Props = $props();
  const token = $derived((labelTokens as readonly string[]).includes(color ?? '') ? (color as LabelToken) : undefined);
</script>

<span class="tile" data-token={token} data-size={size} aria-hidden="true"><Icon name={icon} size="sm" /></span>

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
