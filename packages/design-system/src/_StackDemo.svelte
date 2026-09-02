<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The visible frame around Stack. See _BoxDemo.svelte for why it is underscored.

  import Stack from './Stack.svelte';
  import type { Align, Space } from './space.ts';

  interface Props {
    gap?: Space;
    align?: Align;
    /** Long enough that rule 4 has something to break when the pseudo-locale grows it. */
    isLong?: boolean;
  }

  const { gap = '200', align, isLong = false }: Props = $props();

  const short = ['One', 'Two', 'Three'];
  const wordy = [
    'Aufgabenzuordnung',
    'Arbeitspaket abgeschlossen',
    'Fähigkeit in diesem Arbeitsbereich abgeschaltet',
  ];
</script>

<div class="frame">
  <Stack {gap} {align}>
    {#each isLong ? wordy : short as item (item)}
      <span class="item">{item}</span>
    {/each}
  </Stack>
</div>

<style>
  .frame {
    background: var(--accent-primary-subtle);
    border-radius: var(--r-sm);
    padding: var(--sp-100);
  }

  .item {
    display: block;
    padding: var(--sp-050) var(--sp-100);
    border-radius: var(--r-xs);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-size: var(--fs-100);
    overflow-wrap: anywhere;
  }
</style>
