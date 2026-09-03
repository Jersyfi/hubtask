<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import Menu from './Menu.svelte';
  import Stack from './Stack.svelte';
  import type { MenuItem } from './overlay.ts';

  const { mode = 'actions' }: { mode?: 'actions' | 'long' } = $props();

  let chosen = $state<string | null>(null);

  const actions: MenuItem[] = [
    { id: 'edit', label: 'Rename', icon: 'pencil' },
    { id: 'copy', label: 'Duplicate', icon: 'copy' },
    { id: 'link', label: 'Copy link', icon: 'link' },
    { id: 'archive', label: 'Archive', icon: 'archive', hasSeparatorBefore: true },
    {
      id: 'delete',
      label: 'Delete permanently',
      icon: 'trash-2',
      isDestructive: true,
      disabledReason: 'Deleting needs the workspace owner’s permission.',
    },
  ];

  // Fifteen items is where arrows alone stop being usable and type-ahead starts earning its place:
  // press "a" twice and focus walks the items that share the letter.
  const many: MenuItem[] = [
    'Archive', 'Assign to me', 'Add a label', 'Copy link', 'Duplicate',
    'Export as CSV', 'Follow', 'Move to another collection', 'Mute notifications', 'Open in a new tab',
    'Pin to the top', 'Print', 'Rename', 'Share', 'Unfollow',
  ].map((label) => ({ id: label.toLowerCase().replace(/\s+/g, '-'), label }));
</script>

{#if mode === 'long'}
  <Stack gap="200">
    <Menu label="Task actions" items={many} onselect={(id) => (chosen = id)}>
      {#snippet trigger(props)}
        <Button icon="ellipsis" {...props}>Fifteen actions</Button>
      {/snippet}
    </Menu>
    <p>Chosen: {chosen ?? 'nothing yet'}</p>
  </Stack>
{:else}
  <Stack gap="200">
    <Inline gap="150" align="center">
      <Menu label="Task actions" items={actions} onselect={(id) => (chosen = id)}>
        {#snippet trigger(props)}
          <Button icon="ellipsis" {...props}>Actions</Button>
        {/snippet}
      </Menu>
      <Menu label="Task actions" items={actions} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => (chosen = id)}>
        {#snippet trigger(props)}
          <IconButton icon="ellipsis" label="Task actions" {...props} />
        {/snippet}
      </Menu>
    </Inline>
    <p>Chosen: {chosen ?? 'nothing yet'}</p>
  </Stack>
{/if}
