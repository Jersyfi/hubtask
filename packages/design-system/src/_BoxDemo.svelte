<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The visible frame around Box, which has no colour of its own to show.
  //
  // Underscored, so check-stories treats it as a part rather than as a component that owes the
  // specification an entry (ADR-0037). What it draws is the padding: the tinted band is the Box,
  // the plain block inside it is the child the padding is holding away from the edge.

  import Box from './Box.svelte';
  import type { Space } from './space.ts';

  interface Props {
    /** `all` walks the scale, `inline` shows the axis prop, `bare` shows a Box with no props. */
    mode?: 'all' | 'inline' | 'bare';
  }

  const { mode = 'all' }: Props = $props();

  const steps: Space[] = ['025', '050', '100', '150', '200', '250', '300', '400', '500', '600', '800', '1000'];
</script>

{#if mode === 'bare'}
  <Box>
    <span class="child">No padding, no colour, no border. If anything is visible around this
      sentence, the primitive has started decorating.</span>
  </Box>
{:else}
  <div class="scale">
    {#each steps as step (step)}
      <div class="row">
        <span class="step">{step}</span>
        <div class="frame">
          {#if mode === 'inline'}
            <Box paddingInline={step}><span class="child">padding-inline</span></Box>
          {:else}
            <Box padding={step}><span class="child">padding</span></Box>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .scale {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
  }

  .step {
    min-width: 5ch;
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
    text-align: end;
  }

  /* The tint belongs to the demo, not to Box. It is what makes an invisible primitive visible. */
  .frame {
    background: var(--accent-primary-subtle);
    border-radius: var(--r-sm);
  }

  .child {
    display: block;
    padding: var(--sp-025) var(--sp-050);
    border-radius: var(--r-xs);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
</style>
