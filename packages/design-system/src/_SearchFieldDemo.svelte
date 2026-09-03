<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import SearchField from './SearchField.svelte';
  import Stack from './Stack.svelte';
  import Toolbar from './Toolbar.svelte';

  const { mode = 'labelled' }: { mode?: 'labelled' | 'toolbar' | 'long' } = $props();

  let term = $state('socket');
</script>

<Stack gap="200">
  {#if mode === 'toolbar'}
    <!-- In a toolbar the label is announced rather than drawn: the row is already named, and a
         second heading above one control would be noise on screen and nothing in the tree. -->
    <Toolbar label="Find entries">
      <SearchField label="Search entries" isLabelHidden clearLabel="Clear the search" bind:value={term} />
    </Toolbar>
  {:else if mode === 'long'}
    <SearchField
      label="Einträge in dieser Sammlung durchsuchen"
      clearLabel="Suche zurücksetzen"
      placeholder="Titel und Notizen durchsuchen"
      bind:value={term}
    />
  {:else}
    <SearchField
      label="Search entries"
      clearLabel="Clear the search"
      placeholder="Title and notes"
      bind:value={term}
    />
  {/if}
</Stack>
