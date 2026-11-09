<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import Dialog from './Dialog.svelte';
  import NavItem from './NavItem.svelte';
  import Sheet from './Sheet.svelte';
  import Stack from './Stack.svelte';
  import type { MenuItem } from './overlay.ts';

  const { mode = 'context' }: { mode?: 'context' | 'new' | 'menu' | 'filters' | 'layered' } = $props();

  let isOpen = $state(false);
  let isDialogOpen = $state(false);
  let chosen = $state('');

  const items: MenuItem[] = [
    { id: 'open', label: 'Open' },
    { id: 'role', label: 'Change role…' },
    { id: 'view', label: 'View as' },
    { id: 'revoke', label: 'Revoke access…', isDestructive: true, hasSeparatorBefore: true },
  ];
</script>

{#if mode === 'context' || mode === 'new'}
  <Button onclick={() => (isOpen = true)}>{mode === 'new' ? 'New' : 'Hold “Team”'}</Button>
  <Sheet title={mode === 'new' ? 'New in Contents' : 'Team'} closeLabel="Close" bind:isOpen>
    <ul>
      {#if mode === 'new'}
        <NavItem label="Page" icon="file-text" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Page from a template…" icon="file-text" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Whiteboard" icon="layout-template" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Collection" icon="collection" size="touch" onnavigate={() => (isOpen = false)} />
      {:else}
        <NavItem label="Star" icon="star" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Create under" icon="plus" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Move to…" icon="arrow-right-left" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Change order" icon="grip-vertical" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Archive" icon="archive" size="touch" onnavigate={() => (isOpen = false)} />
        <NavItem label="Move to the trash" icon="trash" size="touch" onnavigate={() => (isOpen = false)} />
      {/if}
    </ul>
  </Sheet>
{:else}
  <Stack gap="200" align="start">
    <Button onclick={() => (isOpen = true)}>{mode === 'filters' ? 'Filters (1)' : 'More for Lukas Brandt'}</Button>
    {#if chosen}<p>Chosen: {chosen}</p>{/if}
  </Stack>

  {#if mode === 'menu'}
    <Sheet bind:isOpen title="Lukas Brandt" closeLabel="Close" grabLabel="Pull down to close" {items} onselect={(id) => (chosen = id)} />
  {:else}
    <Sheet bind:isOpen title="Filters" closeLabel="Close" grabLabel="Pull down to close">
      <Checkbox label="Owner" />
      <Checkbox label="Administrator" checked />
      <Checkbox label="Member" />
      <Checkbox label="Only active people" checked />
      {#if mode === 'layered'}
        <Button tone="secondary" onclick={() => (isDialogOpen = true)}>Save these filters…</Button>
      {/if}
      {#snippet actions()}
        <Button tone="subtle" onclick={() => (isOpen = false)}>Clear</Button>
        <Button tone="primary" onclick={() => (isOpen = false)}>Show 9 people</Button>
      {/snippet}
    </Sheet>
    {#if mode === 'layered'}
      <Dialog bind:isOpen={isDialogOpen} size="s" title="Save these filters?" dismissLabel="Close">
        They are kept on this device.
        {#snippet actions()}
          <Button tone="secondary" onclick={() => (isDialogOpen = false)}>Cancel</Button>
          <Button tone="primary" onclick={() => (isDialogOpen = false)}>Save</Button>
        {/snippet}
      </Dialog>
    {/if}
  {/if}
{/if}
