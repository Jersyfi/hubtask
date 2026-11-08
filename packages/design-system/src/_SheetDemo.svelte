<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import NavItem from './NavItem.svelte';
  import Sheet from './Sheet.svelte';

  const { mode = 'context' }: { mode?: 'context' | 'new' } = $props();
  let isOpen = $state(false);
</script>

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
