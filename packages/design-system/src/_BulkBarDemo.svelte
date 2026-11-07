<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BulkBar from './BulkBar.svelte';
  import Button from './Button.svelte';

  let isSelecting = $state(true);
  let last = $state('');

  // No props: the story's arguments, if any, are ignored.
  const {}: Record<string, unknown> = $props();
</script>

{#if isSelecting}
  <BulkBar
    label="3 selected"
    actions={[{ id: 'role', label: 'Change role…' }, { id: 'group', label: 'Add to a group…' }, { id: 'export', label: 'Export' }]}
    onaction={(id) => (last = id)}
    menu={{ label: 'More for the selected', items: [{ id: 'suspend', label: 'Suspend' }, { id: 'revoke', label: 'Revoke access…', isDestructive: true, hasSeparatorBefore: true }] }}
    doneLabel="Done"
    onDone={() => (isSelecting = false)}
  />
{:else}
  <Button size="sm" icon="square-check" onclick={() => (isSelecting = true)}>Select</Button>
{/if}
{#if last}<p>Chosen: {last}</p>{/if}
