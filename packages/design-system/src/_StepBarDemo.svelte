<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import StepBar from './StepBar.svelte';

  interface Props {
    mode?: 'stage' | 'ends';
  }

  const { mode = 'stage' }: Props = $props();

  const STAGES = [
    { id: 'planned', label: 'Planned' },
    { id: 'ordered', label: 'Ordered' },
    { id: 'stock', label: 'In stock' },
    { id: 'use', label: 'In use' },
    { id: 'maintenance', label: 'Maintenance' },
    { id: 'retired', label: 'Retired' },
    { id: 'disposed', label: 'Disposed of' },
  ];
</script>

<div class="demo">
  {#if mode === 'stage'}
    <StepBar label="Stage of Portal-API" steps={STAGES} current="use" doneLabel="done" />
  {:else}
    <StepBar label="Stage of a planned object" steps={STAGES} current="planned" doneLabel="done" />
    <StepBar label="Stage of a disposed object" steps={STAGES} current="disposed" doneLabel="done" />
  {/if}
</div>

<style>
  .demo { display: flex; flex-direction: column; gap: var(--sp-200); }
</style>
