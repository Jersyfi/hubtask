<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';
  import Drawer from './Drawer.svelte';
  import Inline from './Inline.svelte';
  import Input from './Input.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'end' }: { mode?: 'end' | 'start' | 'bottom' | 'sized' | 'layered' } = $props();

  let isOpen = $state(false);
  let isDialogOpen = $state(false);
  let title = $state('Move the socket by the window');
  // What the caller keeps: the sheet answers its share, the application decides where it lives.
  let size = $state(0.5);
</script>

<Stack gap="200">
  <Button onclick={() => (isOpen = true)}>Open the details</Button>

  <Drawer
    bind:isOpen
    title="Entry details"
    edge={mode === 'start' ? 'inline-start' : mode === 'bottom' || mode === 'sized' ? 'block-end' : 'inline-end'}
    dismissLabel="Close the details"
    isResizable={mode === 'sized'}
    resizeLabel="Size the sheet"
    bind:size
  >
    <Stack gap="200">
      <Input label="Title" bind:value={title} />
      <p>
        A drawer sits beside what is already on screen rather than taking it away, which is the one
        thing that separates it from a dialog.
      </p>
      {#if mode === 'sized'}
        <p>The sheet is {Math.round(size * 100)} per cent of the screen.</p>
        {#each ['One', 'Two', 'Three', 'Four', 'Five', 'Six', 'Seven', 'Eight', 'Nine', 'Ten'] as row (row)}
          <p>{row}: enough under the head for the body to scroll while the head stays where it is.</p>
        {/each}
      {/if}
      {#if mode === 'layered'}
        <Button tone="secondary" onclick={() => (isDialogOpen = true)}>
          Open a dialog from inside it
        </Button>
      {/if}
    </Stack>
  </Drawer>

  {#if mode === 'layered'}
    <!-- Two layers open at once, which is what makes `Escape` observable: the dialog goes first
         even though the drawer is nearer the pointer, because the register ranks by layer and not
         by what was opened last. -->
    <Dialog bind:isOpen={isDialogOpen} title="Discard the changes?" dismissLabel="Keep editing">
      {#snippet actions()}
        <Inline gap="100">
          <Button tone="secondary" onclick={() => (isDialogOpen = false)}>Keep editing</Button>
          <Button tone="danger" onclick={() => (isDialogOpen = false)}>Discard</Button>
        </Inline>
      {/snippet}
      The entry has unsaved changes.
    </Dialog>
  {/if}
</Stack>
