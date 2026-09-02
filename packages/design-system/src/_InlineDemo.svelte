<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The visible frame around Inline. See _BoxDemo.svelte for why it is underscored.

  import Inline from './Inline.svelte';
  import type { Align, Justify, Space } from './space.ts';

  interface Props {
    gap?: Space;
    align?: Align;
    justify?: Justify;
    wrap?: boolean;
    /** The German labels rule 4 is written for. */
    long?: boolean;
  }

  const { gap = '150', align, justify, wrap = true, long = false }: Props = $props();

  const short = ['Save', 'Cancel', 'More'];
  const wordy = ['Aufgabe erstellen', 'Verwerfen', 'Weitere Möglichkeiten'];
</script>

<div class="frame">
  <Inline {gap} {align} {justify} {wrap}>
    {#each long ? wordy : short as item (item)}
      <span class="item">{item}</span>
    {/each}
  </Inline>
</div>

<style>
  .frame {
    background: var(--accent-primary-subtle);
    border-radius: var(--r-sm);
    padding: var(--sp-100);
  }

  .item {
    padding: var(--sp-050) var(--sp-150);
    border-radius: var(--r-xs);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }
</style>
