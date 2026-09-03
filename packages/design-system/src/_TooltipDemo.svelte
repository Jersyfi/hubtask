<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import Stack from './Stack.svelte';
  import Tooltip from './Tooltip.svelte';
  import type { Placement } from './positioning.ts';

  const { mode = 'row' }: { mode?: 'row' | 'edges' } = $props();

  const sides: Placement[] = [
    { side: 'block-start', align: 'center' },
    { side: 'block-end', align: 'center' },
    { side: 'inline-start', align: 'center' },
    { side: 'inline-end', align: 'center' },
  ];
</script>

{#if mode === 'edges'}
  <!-- Against the edge of the viewport, which is where a positioner is worth having: the bubble
       flips to the other side rather than being cut off (ADR-0039). Scroll the pane if the
       trigger is not near an edge yet. -->
  <Inline gap="150" justify="between">
    <Tooltip text="Flips when there is no room on the side it was asked for" placement={{ side: 'inline-start', align: 'center' }}>
      <IconButton icon="chevron-left" label="Previous" />
    </Tooltip>
    <Tooltip text="Flips when there is no room on the side it was asked for" placement={{ side: 'inline-end', align: 'center' }}>
      <IconButton icon="chevron-right" label="Next" />
    </Tooltip>
  </Inline>
{:else}
  <Stack gap="300">
    <Inline gap="200" align="center">
      {#each sides as placement (placement.side)}
        <Tooltip text="Delete permanently" {placement}>
          <IconButton icon="trash-2" label="Delete" tone="danger" />
        </Tooltip>
      {/each}
    </Inline>
  </Stack>
{/if}
