<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import Dialog from './Dialog.svelte';
  import Input from './Input.svelte';
  import Popover from './Popover.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'confirm' }: { mode?: 'confirm' | 'layers' } = $props();

  let confirming = $state(false);
  let editing = $state(false);
</script>

{#if mode === 'layers'}
  <Stack gap="200">
    <!-- Two layers. `Escape` closes the popover and leaves the dialog standing; the second press
         closes the dialog and focus lands back on the button that opened it. -->
    <Button tone="primary" onclick={() => (editing = true)}>Edit task</Button>
    <Dialog title="Edit task" bind:isOpen={editing} dismissLabel="Close">
      {#snippet actions()}
        <Button onclick={() => (editing = false)}>Cancel</Button>
        <Button tone="primary" onclick={() => (editing = false)}>Save</Button>
      {/snippet}
      <Stack gap="200">
        <Input label="Title" value="Write the release notes" />
        <Popover label="Filter">
          {#snippet trigger(props)}
            <Button size="sm" icon="funnel" {...props}>Add a filter</Button>
          {/snippet}
          <Stack gap="100">
            <Checkbox label="Only my tasks" />
            <Checkbox label="Include completed" />
          </Stack>
        </Popover>
      </Stack>
    </Dialog>
  </Stack>
{:else}
  <Stack gap="200">
    <Button tone="danger" icon="trash-2" onclick={() => (confirming = true)}>Delete permanently</Button>
    <Dialog title="Delete this collection?" bind:isOpen={confirming} dismissLabel="Close">
      {#snippet actions()}
        <Button onclick={() => (confirming = false)}>Keep it</Button>
        <Button tone="danger" onclick={() => (confirming = false)}>Delete permanently</Button>
      {/snippet}
      Its 42 tasks are deleted with it. This cannot be undone.
    </Dialog>
  </Stack>
{/if}
