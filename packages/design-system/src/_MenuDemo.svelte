<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Dialog from './Dialog.svelte';
  import IconButton from './IconButton.svelte';
  import Inline from './Inline.svelte';
  import Menu from './Menu.svelte';
  import Stack from './Stack.svelte';
  import Input from './Input.svelte';
  import { setCloseLabel } from './media.ts';
  import type { MenuItem } from './overlay.ts';
  import { scrollEdges } from './scrollEdges.ts';

  const { mode = 'actions' }: { mode?: 'actions' | 'long' | 'outside' | 'foot' | 'dialog' | 'edges' } = $props();

  // What the application sets once in its language; a menu becomes a sheet on a phone only with it.
  $effect(() => {
    if (mode !== 'dialog') return;
    setCloseLabel('Close');
    return () => setCloseLabel(undefined);
  });

  let isEditing = $state(false);
  let title = $state('');

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

  // Named apart: inside the dialog, `actions` is its foot.
  const stepItems = actions;

  // Fifteen items is where arrows alone stop being usable and type-ahead starts earning its place:
  // press "a" twice and focus walks the items that share the letter.
  const many: MenuItem[] = [
    'Archive', 'Assign to me', 'Add a label', 'Copy link', 'Duplicate',
    'Export as CSV', 'Follow', 'Move to another collection', 'Mute notifications', 'Open in a new tab',
    'Pin to the top', 'Print', 'Rename', 'Share', 'Unfollow',
  ].map((label) => ({ id: label.toLowerCase().replace(/\s+/g, '-'), label }));

  // Where each list of the shades' story stands: the share of its room it is scrolled to.
  const edgeCases = [
    { name: 'Nothing to scroll', at: 'short', items: actions.slice(0, 3) },
    { name: 'More below', at: 'start', items: many },
    { name: 'More on both sides', at: 'middle', items: many },
    { name: 'More above', at: 'end', items: many },
  ] as const;

  const SHARE = { short: 0, start: 0, middle: 0.5, end: 1 } as const;

  function scrolledTo(at: keyof typeof SHARE) {
    return (node: HTMLElement) => {
      node.scrollTop = (node.scrollHeight - node.clientHeight) * SHARE[at];
    };
  }
</script>

{#if mode === 'edges'}
  <!-- The four states of the scroll shades, each list held where it is scrolled to: a shade only at
       an edge with more beyond it. Scroll any of them and the shades follow. -->
  <div class="edges">
    {#each edgeCases as edge (edge.name)}
      <section class="edge-case" aria-label={edge.name}>
        <h3 class="edge-name">{edge.name}</h3>
        <div class="edge-frame">
          <ul class="edge-list" data-edge-case={edge.at} {@attach scrolledTo(edge.at)} {@attach scrollEdges}>
            {#each edge.items as item (item.id)}<li class="edge-item">{item.label}</li>{/each}
          </ul>
        </div>
      </section>
    {/each}
  </div>
{:else if mode === 'outside'}
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
{:else if mode === 'foot'}
  <!-- Rows that fill the screen, the last one with a menu longer than the room on either side of
       its trigger: the menu takes the larger side and scrolls inside it. -->
  <div class="foot">
    {#each Array.from({ length: 12 }, (_, index) => index + 1) as row (row)}
      <div class="filler">Row {row}</div>
    {/each}
    <Inline gap="150" align="center" justify="between">
      <span class="filler">Last row</span>
      <Menu label="Actions for the last row" items={many} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => (chosen = id)}>
        {#snippet trigger(props)}
          <IconButton icon="ellipsis" label="Actions for the last row" {...props} />
        {/snippet}
      </Menu>
    </Inline>
  </div>
{:else if mode === 'dialog'}
  <Stack gap="200">
    <Button tone="primary" onclick={() => (isEditing = true)}>Edit the step</Button>
    <Dialog title="Edit the step" bind:isOpen={isEditing} dismissLabel="Close">
      {#snippet actions()}
        <Button onclick={() => (isEditing = false)}>Cancel</Button>
        <Button tone="primary" onclick={() => (isEditing = false)}>Save</Button>
      {/snippet}
      <Inline gap="150" align="end">
        <Input label="Title" bind:value={title} />
        <Menu label="Step actions" items={stepItems} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => (chosen = id)}>
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label="Step actions" {...props} />
          {/snippet}
        </Menu>
      </Inline>
    </Dialog>
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
  .edges {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-300);
  }

  .edge-case { display: flex; flex-direction: column; gap: var(--sp-100); }

  .edge-name { margin: 0; font-size: var(--fs-090); font-weight: var(--fw-semibold); }

  /* A surface as a menu's: the frame that does not scroll, its corners the shades keep inside. */
  .edge-frame {
    position: relative;
    display: flex;
    flex-direction: column;
    inline-size: var(--layout-sidenav-width);
    max-block-size: var(--sp-1600);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    box-shadow: var(--shadow-overlay);
  }

  .edge-list {
    min-block-size: 0;
    margin: 0;
    padding: var(--sp-050);
    overflow: auto;
    list-style: none;
  }

  .edge-item {
    padding: var(--sp-050) var(--sp-100);
    border-radius: var(--r-sm);
    font-size: var(--fs-090);
  }

  .edge-item:hover { background: var(--bg-surface-hover); }

  .foot {
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    gap: var(--sp-100);
    min-block-size: 100vh;
  }

  .filler { padding: var(--sp-100) var(--sp-150); }

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
