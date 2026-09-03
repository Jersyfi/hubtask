<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import Inline from './Inline.svelte';
  import Popover from './Popover.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'filter' }: { mode?: 'filter' | 'nested' } = $props();

  let open = $state(false);
</script>

{#if mode === 'nested'}
  <!-- Two layers, which is where the register earns its keep: `Escape` closes the popover and
       leaves the one underneath open (design-system.md §6). -->
  <Popover label="Filter">
    {#snippet trigger(props)}
      <Button icon="funnel" {...props}>Filter</Button>
    {/snippet}
    <Stack gap="150">
      <Checkbox label="Only my tasks" />
      <Popover label="Due date" placement={{ side: 'inline-end', align: 'start' }}>
        {#snippet trigger(props)}
          <Button size="sm" icon="calendar" {...props}>Due date</Button>
        {/snippet}
        <Stack gap="100">
          <Checkbox label="Today" />
          <Checkbox label="This week" />
          <Checkbox label="Overdue" />
        </Stack>
      </Popover>
    </Stack>
  </Popover>
{:else}
  <Inline gap="200" align="center">
    <Popover label="Filter" bind:isOpen={open}>
      {#snippet trigger(props)}
        <Button icon="funnel" {...props}>Filter</Button>
      {/snippet}
      <Stack gap="150">
        <Checkbox label="Only my tasks" />
        <Checkbox label="Include completed" />
        <Checkbox label="Has an attachment" />
        <Button size="sm" tone="primary">Apply</Button>
      </Stack>
    </Popover>
    <Button tone="subtle" onclick={() => (open = !open)}>Open it from out here</Button>
  </Inline>
{/if}
