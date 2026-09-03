<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Select from './Select.svelte';
  import Stack from './Stack.svelte';
  import Toolbar from './Toolbar.svelte';

  const { mode = 'mixed' }: { mode?: 'mixed' | 'icons' | 'long' } = $props();

  let sort = $state('manual');
</script>

<Stack gap="200">
  {#if mode === 'icons'}
    <Toolbar label="Entry actions">
      <IconButton icon="plus" label="Add an entry" size="sm" />
      <IconButton icon="pencil" label="Rename" size="sm" />
      <IconButton icon="copy" label="Duplicate" size="sm" />
      <IconButton icon="tag" label="Add a label" size="sm" />
      <IconButton icon="archive" label="Archive" size="sm" />
      <IconButton
        icon="trash-2"
        label="Delete"
        size="sm"
        disabledReason="Deleting needs the workspace owner’s permission."
      />
    </Toolbar>
  {:else if mode === 'long'}
    <Toolbar label="Aktionen für Einträge">
      <Button size="sm">Eintrag hinzufügen</Button>
      <Button size="sm" tone="secondary">Mehrere bearbeiten</Button>
      <Button size="sm" tone="secondary">Sammlungseinstellungen</Button>
      <Button size="sm" tone="secondary">Änderungsverlauf anzeigen</Button>
    </Toolbar>
  {:else}
    <!-- Heterogeneous on purpose: a toolbar takes children rather than data because its contents
         are a button, a select and an icon button, and a list of items would grow a kind for each. -->
    <Toolbar label="Entry actions">
      <Button size="sm">Add an entry</Button>
      <IconButton icon="funnel" label="Filter" size="sm" />
      <Select
        label="Sort by"
        size="sm"
        bind:value={sort}
        options={[
          { value: 'manual', label: 'Manual order' },
          { value: 'due', label: 'Due date' },
          { value: 'title', label: 'Title' },
        ]}
      />
      <IconButton icon="ellipsis" label="More actions" size="sm" />
    </Toolbar>
  {/if}
</Stack>
