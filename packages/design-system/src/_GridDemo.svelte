<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Grid, { type GridColumns } from './Grid.svelte';

  const { columns = { compact: 1, medium: 2, expanded: 3, large: 4, xlarge: 5 } }: { columns?: GridColumns } = $props();

  const cells = ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven', 'Eight', 'Nine', 'Ten'];
</script>

<Grid as="ul" {columns}>
  {#each cells as cell (cell)}
    <li class="cell">{cell}</li>
  {/each}
</Grid>

<style>
  .cell {
    padding: var(--sp-200);
    border-radius: var(--r-sm);
    background: var(--accent-primary-subtle);
    color: var(--text-primary);
  }
</style>
