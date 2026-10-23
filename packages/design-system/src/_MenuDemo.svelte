<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import Menu from './Menu.svelte';
  import Stack from './Stack.svelte';
  import type { MenuItem } from './overlay.ts';

  const { mode = 'actions' }: { mode?: 'actions' | 'long' | 'outside' } = $props();

  let chosen = $state<string | null>(null);

  // A menu opened from the row it belongs to rather than from its button: what a right click, a
  // long press and the context-menu key do. The surface still opens at the button.
  let rowMenu = $state<ReturnType<typeof Menu> | undefined>(undefined);

  function fromRow(event: MouseEvent | KeyboardEvent) {
    if (event instanceof KeyboardEvent && !(event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey))) return;
    // A right click with Shift keeps the browser's own menu.
    if (event instanceof MouseEvent && event.shiftKey) return;
    event.preventDefault();
    rowMenu?.open(0);
  }

  const actions: MenuItem[] = [
    { id: 'edit', label: 'Rename', icon: 'pencil' },
    { id: 'copy', label: 'Duplicate', icon: 'copy' },
    { id: 'link', label: 'Copy link', icon: 'link' },
    { id: 'archive', label: 'Archive', icon: 'archive', hasSeparatorBefore: true },
    {
      id: 'delete',
      label: 'Delete permanently',
      icon: 'trash',
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

{#if mode === 'outside'}
  <Stack gap="200">
    <Inline gap="150" align="center">
      <button type="button" class="row" oncontextmenu={fromRow} onkeydown={fromRow}>Buy milk</button>
      <Menu bind:this={rowMenu} label="Actions for Buy milk" items={actions} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => (chosen = id)}>
        {#snippet trigger(props)}
          <IconButton icon="ellipsis" label="Actions for Buy milk" {...props} />
        {/snippet}
      </Menu>
    </Inline>
    <p>Chosen: {chosen ?? 'nothing yet'}</p>
  </Stack>
{:else if mode === 'long'}
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

<style>
  .row {
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
  }

  .row:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
</style>
