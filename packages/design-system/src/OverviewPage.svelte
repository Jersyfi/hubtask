<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What needs a look (page-system §7.5): the head, at most one raised focal card - the one shadow
  // a screen without movement may carry (§4.2) - and `SummaryCard`s in a grid: one column on a
  // phone, two on a tablet, three above.
  import type { Snippet } from 'svelte';

  import Grid from './Grid.svelte';
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
  <Grid columns={{ compact: 1, medium: 2, expanded: 3 }} gap="200">{@render children()}</Grid>
</PageFrame>

<style>
  .focal {
    display: grid;
    gap: var(--sp-150);
    padding: var(--sp-250);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-raised);
  }
</style>
