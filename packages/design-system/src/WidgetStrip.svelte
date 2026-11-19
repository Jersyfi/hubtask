<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Today's cards in one row (scene today): however many there are, they share the row equally;
  // on a phone the row becomes a strip the thumb swipes through, each card snapping into place,
  // the next one showing at the edge so the strip says that it goes on.
  import type { Snippet } from 'svelte';

  interface Props {
    /** The region's name, announced: *Your day in figures*. */
    label: string;
    children: Snippet;
  }

  const { label, children }: Props = $props();
</script>

<section class="strip" aria-label={label}>{@render children()}</section>

<style>
  .strip {
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: minmax(0, 1fr);
    gap: var(--sp-150);
  }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .strip {
      grid-auto-columns: minmax(var(--layout-gallery-card), 72%);
      overflow-x: auto;
      overscroll-behavior-inline: contain;
      scroll-snap-type: inline mandatory;
      scroll-padding-inline: var(--sp-200);
      padding-block-end: var(--sp-100);
      scrollbar-width: none;
    }
    .strip > :global(*) { scroll-snap-align: start; }
  }
</style>
