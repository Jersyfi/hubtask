<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import BlockHandle from './BlockHandle.svelte';
  import BoxPopover from './BoxPopover.svelte';
  import FloatingToolbar from './FloatingToolbar.svelte';
  import IconButton from './IconButton.svelte';
  import type { Box } from './anchor.ts';

  interface Props {
    mode?: 'selection' | 'line' | 'panel';
  }

  const { mode = 'selection' }: Props = $props();

  let page = $state<HTMLElement | undefined>(undefined);
  let marked = $state<HTMLElement | undefined>(undefined);
  let line = $state<HTMLElement | undefined>(undefined);
  let box = $state<Box | undefined>(undefined);
  let said = $state('');

  $effect(() => {
    const element = mode === 'selection' ? marked : line;
    if (element) box = element.getBoundingClientRect();
  });
</script>

<div class="page" bind:this={page}>
  <p>The app speaks only to the <span class="selected" bind:this={marked}>public interface</span>. There is no direct access to the databases.</p>
  <p bind:this={line}>Requests made offline are kept with a key that makes a second send harmless.</p>
  <p class="said" aria-live="polite">{said}</p>
</div>
{#if mode === 'selection'}
  <FloatingToolbar label="Format" container={page} {box}>
    <IconButton icon="bold" size="sm" tone="subtle" label="Bold" onclick={() => (said = 'Bold')} />
    <IconButton icon="italic" size="sm" tone="subtle" label="Italic" onclick={() => (said = 'Italic')} />
    <IconButton icon="strikethrough" size="sm" tone="subtle" label="Struck through" onclick={() => (said = 'Struck through')} />
    <IconButton icon="code" size="sm" tone="subtle" label="Code" onclick={() => (said = 'Code')} />
    <IconButton icon="link" size="sm" tone="subtle" label="Link" onclick={() => (said = 'Link')} />
    <IconButton icon="message-square" size="sm" tone="subtle" label="Comment" onclick={() => (said = 'Comment')} />
  </FloatingToolbar>
{:else if mode === 'panel'}
  <BoxPopover label="Insert" container={page} {box} onclose={() => (said = 'Closed')}>
    <p>Blocks to insert at the line</p>
  </BoxPopover>
{:else}
  <BlockHandle container={page} {box} addLabel="Insert a block below" handleLabel="This line" onadd={() => (said = 'Insert')} items={[{ id: 'up', label: 'Move up', icon: 'arrow-up' }, { id: 'down', label: 'Move down', icon: 'arrow-down' }, { id: 'remove', label: 'Remove', icon: 'trash', isDestructive: true, hasSeparatorBefore: true }]} onselect={(id) => (said = id)} />
{/if}

<style>
  .page { max-inline-size: var(--layout-reading); padding-block: var(--sp-600); padding-inline: var(--sp-600); }
  .selected { background: var(--accent-primary-subtle); }
  .said { color: var(--text-subtle); }
</style>
