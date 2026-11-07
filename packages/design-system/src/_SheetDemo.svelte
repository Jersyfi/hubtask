<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import Dialog from './Dialog.svelte';
  import Sheet from './Sheet.svelte';
  import Stack from './Stack.svelte';
  import type { MenuItem } from './overlay.ts';

  const { mode = 'menu' }: { mode?: 'menu' | 'filters' | 'layered' } = $props();

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

<Stack gap="200" align="start">
  <Button onclick={() => (isOpen = true)}>{mode === 'filters' ? 'Filters (1)' : 'More for Lukas Brandt'}</Button>
  {#if chosen}<p>Chosen: {chosen}</p>{/if}
</Stack>

{#if mode === 'menu'}
  <Sheet bind:isOpen title="Lukas Brandt" dismissLabel="Close" grabLabel="Pull to full height" {items} onselect={(id) => (chosen = id)} />
{:else}
  <Sheet bind:isOpen title="Filters" dismissLabel="Close" grabLabel="Pull to full height">
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
    <Dialog bind:isOpen={isDialogOpen} title="Save these filters?" dismissLabel="Close">
      They are kept on this device.
      {#snippet actions()}
        <Button tone="secondary" onclick={() => (isDialogOpen = false)}>Cancel</Button>
        <Button tone="primary" onclick={() => (isDialogOpen = false)}>Save</Button>
      {/snippet}
    </Dialog>
  {/if}
{/if}
