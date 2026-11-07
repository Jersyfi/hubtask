<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Repeating blocks of a composite object - a workflow's statuses and steps, a type's fields - one
  // line each (page-system §6, PS-12): grip, mark or name, a sentence saying what it does, ⋯.
  // Pressing a block opens its dialog (§5.1); nothing of it is edited in the line. The heading,
  // the count and *Add* stand on the canvas above the one region.
  //
  // Reordering has three ways, each the same request (`onmove`): the grip dragged, `Alt` with the
  // vertical arrows on a focused block (`reorder.ts`, the keys every list here answers), and the
  // menu's *Move up* and *Move down*, which the caller puts into `menu`.
  import Badge from './Badge.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import Menu from './Menu.svelte';
  import RegionHead from './_RegionHead.svelte';
  import type { MenuItem } from './overlay.ts';
  import { rankIntent } from './reorder.ts';

  export interface Block {
    readonly id: string;
    readonly name: string;
    /** A status's colour: the name is drawn as a `Badge` of this tone. */
    readonly tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'info';
    /** What it does, in one sentence. */
    readonly summary?: string;
  }

  interface Props {
    title: string;
    /** *4*. */
    count?: string;
    blocks: readonly Block[];
    onopen: (id: string) => void;
    /** A block's ⋯ menu; *Move up*, *Move down* among its items. */
    menu?: (block: Block) => { label: string; items: readonly MenuItem[] } | undefined;
    onmenu?: (block: Block, id: string) => void;
    /** A block asks to stand at `to`. */
    onmove?: (id: string, to: number) => void;
    /** The grip's name, announced: *Move Start work*. */
    gripLabel?: (block: Block) => string;
    add?: { label: string; onclick: () => void };
  }

  const { title, count, blocks, onopen, menu, onmenu, onmove, gripLabel, add }: Props = $props();

  let dragged = $state<string | null>(null);

  function byKey(event: KeyboardEvent, index: number, id: string) {
    const to = rankIntent(event, index, blocks.length);
    if (to === null || !onmove) return;
    event.preventDefault();
    onmove(id, to);
  }

  function drop(event: DragEvent, to: number) {
    event.preventDefault();
    if (dragged !== null) onmove?.(dragged, to);
    dragged = null;
  }
</script>

<section class="block-list">
  <RegionHead {title} note={count}>
    {#snippet acts()}
      {#if add}<Button size="sm" icon="plus" onclick={add.onclick}>{add.label}</Button>{/if}
    {/snippet}
  </RegionHead>
  <ol class="region">
    {#each blocks as block, index (block.id)}
      {@const blockMenu = menu?.(block)}
      <li
        class="block"
        data-dragged={dragged === block.id ? '' : undefined}
        ondragover={(event) => { if (dragged !== null) event.preventDefault(); }}
        ondrop={(event) => drop(event, index)}
      >
        {#if onmove}
          <span
            class="grip"
            draggable="true"
            role="presentation"
            title={gripLabel?.(block)}
            ondragstart={(event) => { dragged = block.id; event.dataTransfer?.setData('text/plain', block.id); }}
            ondragend={() => (dragged = null)}
          ><Icon name="grip-vertical" size="sm" /></span>
        {/if}
        <button type="button" class="press" onclick={() => onopen(block.id)} onkeydown={(event) => byKey(event, index, block.id)}>
          {#if block.tone}
            <Badge tone={block.tone}>{block.name}</Badge>
          {:else}
            <span class="name">{block.name}</span>
          {/if}
        </button>
        {#if block.summary}<span class="summary">{block.summary}</span>{/if}
        {#if blockMenu && blockMenu.items.length > 0}
          <span class="menu">
            <Menu label={blockMenu.label} items={blockMenu.items} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => onmenu?.(block, id)}>
              {#snippet trigger(props)}
                <IconButton icon="ellipsis" label={blockMenu.label} size="sm" {...props} />
              {/snippet}
            </Menu>
          </span>
        {/if}
      </li>
    {/each}
  </ol>
</section>

<style>
  .block-list { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }

  .region {
    margin: 0;
    padding: 0;
    list-style: none;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: clip;
  }

  .block {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    padding: var(--sp-150) var(--sp-200);
    font-size: var(--fs-100);
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .block + .block { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .block:hover { background: var(--bg-surface-hover); }
  .block[data-dragged] { opacity: 0.5; }

  .grip { position: relative; z-index: var(--z-raised); display: inline-flex; color: var(--text-subtle); cursor: grab; }

  .press {
    flex: none;
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .press::after { content: ''; position: absolute; inset: 0; }

  .press:focus-visible { outline: none; }
  /* The ring inside the line, so the region's clip at its corners cannot cut it. */
  .press:focus-visible::after {
    inset: var(--sp-050);
    border-radius: var(--r-md);
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .name { font-size: var(--fs-200); font-weight: var(--fw-semibold); white-space: nowrap; }

  .summary { flex: 1; min-width: 0; overflow: hidden; color: var(--text-secondary); text-overflow: ellipsis; white-space: nowrap; }

  .menu { position: relative; z-index: var(--z-raised); margin-inline-start: auto; }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .block { flex-wrap: wrap; }
    .summary { white-space: normal; }
  }

  @media (prefers-reduced-motion: reduce) {
    .block { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .block { transition-duration: var(--dur-instant); }
</style>
