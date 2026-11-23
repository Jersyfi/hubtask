<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import Dialog from './Dialog.svelte';
  import DialogSizesDemo from './_DialogSizesDemo.svelte';
  import Input from './Input.svelte';
  import Popover from './Popover.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'confirm', size }: { mode?: 'confirm' | 'layers' | 'wide' | 'hang' | 'alert'; size?: 's' | 'm' | 'l' | 'discard' } = $props();

  let working = $state(false);
  let hanging = $state(false);
  let hook = $state<HTMLElement | undefined>(undefined);

  let confirming = $state(false);
  let alerting = $state(false);
  let choice = $state<'convert' | 'pause' | undefined>(undefined);
  let editing = $state(false);
</script>

{#if size}
  <DialogSizesDemo {size} />
{:else if mode === 'alert'}
  <Stack gap="200">
    <Button tone="secondary" onclick={() => { choice = undefined; alerting = true; }}>Switch Workflows off…</Button>
    <Dialog
      title="Switch Workflows off?"
      bind:isOpen={alerting}
      dismissLabel="Close"
      size="m"
      isAlert
      primary={{ label: 'Switch off', tone: 'danger', onclick: () => (alerting = false), disabledReason: choice ? undefined : 'Choose first what happens to the entries.' }}
    >
      {#snippet actions()}
        <Button onclick={() => (alerting = false)}>Keep it on</Button>
      {/snippet}
      <Stack gap="200">
        <span>Statuses with steps disappear for everyone. 17 open entries stand in such a status.</span>
        <Checkbox label="Convert them to columns" checked={choice === 'convert'} onchange={() => (choice = choice === 'convert' ? undefined : 'convert')} />
      </Stack>
    </Dialog>
  </Stack>
{:else if mode === 'hang'}
  <Stack gap="200" align="end">
    <span bind:this={hook}><Button tone="primary" icon="plus" onclick={() => (hanging = true)}>Create</Button></span>
    <Dialog title="Create" bind:isOpen={hanging} dismissLabel="Close" size="l" hangsFrom={hook}>
      <Input label="One sentence makes an entry" value="Write the release notes tomorrow" />
    </Dialog>
  </Stack>
{:else if mode === 'wide'}
  <Stack gap="200">
    <Button tone="primary" icon="plus" onclick={() => (working = true)}>Capture</Button>
    <Dialog title="Capture" bind:isOpen={working} dismissLabel="Close" isWide>
      <Input label="What do you want to remember?" value="Write the release notes tomorrow 10" />
    </Dialog>
  </Stack>
{:else if mode === 'layers'}
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
    <Button tone="danger" icon="trash" onclick={() => (confirming = true)}>Delete permanently</Button>
    <Dialog title="Delete this collection?" bind:isOpen={confirming} dismissLabel="Close">
      {#snippet actions()}
        <Button onclick={() => (confirming = false)}>Keep it</Button>
        <Button tone="danger" onclick={() => (confirming = false)}>Delete permanently</Button>
      {/snippet}
      Its 42 tasks are deleted with it. This cannot be undone.
    </Dialog>
  </Stack>
{/if}
