<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one floating surface (work-surface concept §6.6, §6.7): a page's writing bar, a board's
  // islands and its selection bar are the same raised strip - one radius, one surface, one shadow.
  // It draws only the surface; where it stands is its user's, by `class` and `style`.
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  interface Props extends HTMLAttributes<HTMLDivElement> {
    /** The bar's element, for whoever places or measures it. */
    element?: HTMLElement | null;
    /** The bar's drawn width, for whoever keeps it inside its room. */
    width?: number;
    /**
     * Where a bar that follows something stands in its positioned room, set through the CSSOM so
     * no style attribute meets the policy: a board's selection bar over the selection.
     */
    at?: { inlineStart: string; blockStart: string };
    children?: Snippet;
  }

  let { element = $bindable(null), width = $bindable(0), at, class: placed, children, ...rest }: Props = $props();
</script>

<div bind:this={element} bind:offsetWidth={width} class={['floating-bar', placed]} style:inset-inline-start={at?.inlineStart} style:inset-block-start={at?.blockStart} {...rest}>{@render children?.()}</div>

<style>
  .floating-bar {
    display: flex;
    align-items: center;
    gap: var(--sp-025);
    box-sizing: border-box;
    padding: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    color: var(--text-primary);
    box-shadow: var(--shadow-overlay);
  }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .floating-bar > :global(*) { min-inline-size: var(--layout-target); }
  }
</style>
