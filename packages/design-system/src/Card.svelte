<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one surface a region or an item of the overview stands on (design-system.md §4, the
  // surfaces and levels, 1 and 1′): `bg.surface`, a `border.subtle` hairline, `r.lg`, no shadow; a
  // card pressed whole answers the pointer with a step of surface and a firmer border. `Widget`
  // stands on it, and so does every tile beside one - a collection's card, *What happened* - so a
  // page never mixes two surfaces. What stands inside, and how, is the caller's.
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class' | 'style'> {
    element?: 'section' | 'article' | 'li' | 'div';
    /** Whether the whole card is pressed - a link stretched over it: the hover of an item (1′). */
    isPressable?: boolean;
    /** The space inside: `md` a card of a grid or a strip, `lg` a tile, `none` rows to the edge. */
    padding?: 'none' | 'md' | 'lg';
    children?: Snippet;
  }

  const { element = 'div', isPressable = false, padding = 'lg', children, ...rest }: Props = $props();
</script>

<svelte:element this={element} {...rest} class="card" data-pressable={isPressable ? '' : undefined} data-padding={padding}>
  {@render children?.()}
</svelte:element>

<style>
  .card {
    position: relative;
    display: flex;
    flex-direction: column;
    min-inline-size: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .card[data-padding='md'] { padding: var(--sp-150); }
  .card[data-padding='lg'] { padding: var(--sp-200); }

  /* A pointer is told before it presses; a finger has no hover to be told by. */
  @media (hover: hover) {
    .card[data-pressable]:hover { border-color: var(--border-default); background: var(--bg-surface-hover); cursor: pointer; }
  }
</style>
