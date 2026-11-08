<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The work surface (page-system §7.8): the head, the place's tabs where it has several, one
  // toolbar - the view choice as a segmented control in it, never a second strip (PS-2) - and the
  // content: one view at a time. A screen whose head carries more than `PageHead` (a running timer)
  // draws its own `PageHeader` as `top`.
  import type { Snippet } from 'svelte';

  import PageFrame from './_PageFrame.svelte';
  import type { PageHead } from './page.ts';

  interface Props {
    head?: PageHead;
    /** A head of the screen's own instead of `head`. */
    top?: Snippet;
    /** The place's `PlaceTabs`: layer 2 of §4.3. */
    tabs?: Snippet;
    /** The `Toolbar`: the view's segmented control and its acts. */
    toolbar?: Snippet;
    refused?: Snippet;
    /** The one view. */
    children: Snippet;
  }

  const { head, top, tabs, toolbar, refused, children }: Props = $props();
</script>

<PageFrame kind="work" {head} {top} {refused}>
  {#if tabs}{@render tabs()}{/if}
  {#if toolbar}<div class="toolbar">{@render toolbar()}</div>{/if}
  {@render children()}
</PageFrame>

<style>
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100) var(--sp-200);
    min-width: 0;
  }
</style>
