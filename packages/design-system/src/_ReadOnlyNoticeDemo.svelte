<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import ReadOnlyNotice from './ReadOnlyNotice.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'archived' }: { mode?: 'archived' | 'role' | 'plan' } = $props();
</script>

<Stack gap="200">
  {#if mode === 'archived'}
    <ReadOnlyNotice text="This collection is archived. Its entries can be read, not changed.">
      {#snippet action()}<Button tone="secondary" size="sm">Restore</Button>{/snippet}
    </ReadOnlyNotice>
  {:else if mode === 'role'}
    <ReadOnlyNotice text="You can read this hub. Ask its owner to change something." icon="eye" />
  {:else}
    <ReadOnlyNotice text="Automation is not part of this workspace's plan. The rules below are kept as they were." />
  {/if}
  <ul class="rows">
    <li>Order the tiles</li>
    <li>Book the electrician</li>
    <li>Measure the hallway</li>
  </ul>
</Stack>

<style>
  .rows { margin: 0; padding-inline-start: var(--sp-300); color: var(--text-primary); }
</style>
