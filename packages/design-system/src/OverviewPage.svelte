<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What needs a look (page-system §7.5): the head, at most one raised focal card - the one shadow
  // a screen without movement may carry (§4.2) - and `SummaryCard`s in a grid: one column on a
  // phone's width, two on a tablet's, three above - the widths of the page's own region, not of
  // the window, so a pinned navigation beside it does not leave three cards too narrow.
  import type { Snippet } from 'svelte';

  import PageFrame from './_PageFrame.svelte';
  import type { PageHead } from './page.ts';

  interface Props {
    head: PageHead;
    /** The one focal card: *Getting started*. */
    focal?: Snippet;
    refused?: Snippet;
    /** The `SummaryCard`s. */
    children: Snippet;
  }

  const { head, focal, refused, children }: Props = $props();
</script>

<PageFrame kind="overview" {head} {refused}>
  {#if focal}<section class="focal">{@render focal()}</section>{/if}
  <div class="tiles"><div class="grid">{@render children()}</div></div>
</PageFrame>

<style>
  .focal {
    display: grid;
    gap: var(--sp-150);
    padding: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    /* The cards' own surface, so the focal card is one of them in either theme; its shadow is
       what raises it. */
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .tiles { container-type: inline-size; }

  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--sp-200);
  }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a container query cannot read a custom property. */
  @container (width >= 600px) {
    .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a container query cannot read a custom property. */
  @container (width >= 905px) {
    .grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  }
</style>
