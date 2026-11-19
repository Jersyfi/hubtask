<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import DensitySwitch from './DensitySwitch.svelte';
  import type { ListDensity } from './density.ts';

  interface Props {
    list?: string;
  }

  const { list = 'workbench-density' }: Props = $props();
  let density = $state<ListDensity>('comfortable');
</script>

<div class="column">
  <DensitySwitch label="Density" comfortableLabel="Comfortable" compactLabel="Compact" {list} bind:density />
  <p class="quiet" role="status">Rows at {density}.</p>
</div>

<style>
  .column { display: flex; flex-direction: column; align-items: start; gap: var(--sp-150); }
  .quiet { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
