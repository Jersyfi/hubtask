<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A grid of `ResourceCard`s standing directly on the canvas, its filter bar above it - never
  // inside a section or another card (page-system §4.2 rule 4). One column on a phone, then two,
  // three, four and five (§4.4).
  import type { Snippet } from 'svelte';

  import Grid from './Grid.svelte';

  interface Props {
    /** What the items are: *Templates*. Announced as the list's name. */
    label: string;
    /** The cards. */
    children: Snippet;
    /** What shows when there are none (`EmptyState`). */
    empty?: Snippet;
    /** Whether there is a card to draw; without one, `empty`. */
    isEmpty?: boolean;
  }

  const { label, children, empty, isEmpty = false }: Props = $props();
</script>

{#if isEmpty && empty}
  <div class="empty">{@render empty()}</div>
{:else}
  <Grid as="ul" aria-label={label} columns={{ compact: 1, medium: 2, expanded: 3, large: 4, xlarge: 5 }} gap="200">
    {@render children()}
  </Grid>
{/if}

<style>
  .empty {
    padding: var(--sp-400) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
