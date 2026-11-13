<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // "+" and the handle at a line of a page while writing (S-1): beside the line's start, drawn
  // through a phantom anchor over the line's box (`rectAnchor`). "+" inserts a block below the
  // line; the handle opens what can be done with the line - turn it into another block, move it,
  // remove it - as a menu, so nothing is reachable only by dragging (WCAG 2.5.7).

  import IconButton from './IconButton.svelte';
  import Menu from './Menu.svelte';
  import type { MenuItem } from './overlay.ts';
  import { anchorTo, rectAnchor, type Box, type RectAnchor } from './anchor.ts';

  interface Props {
    /** The element the line lies in; the anchor is laid inside it. */
    container: HTMLElement | undefined;
    /** The line's box in the viewport; absent draws nothing. */
    box: Box | undefined;
    addLabel: string;
    handleLabel: string;
    onadd: () => void;
    /** What the handle's menu offers for the line: turn into, move, remove. */
    items: readonly MenuItem[];
    onselect: (id: string) => void;
  }

  const { container, box, addLabel, handleLabel, onadd, items, onselect }: Props = $props();

  let surface = $state<HTMLElement | null>(null);
  let anchor = $state<RectAnchor | undefined>(undefined);

  $effect(() => {
    const host = container;
    const element = surface;
    if (!host || !element) return;
    const made = rectAnchor(host);
    made.place(box);
    const untie = anchorTo(made.element, element, { placement: { side: 'inline-start', align: 'start' } });
    anchor = made;
    return () => {
      untie();
      made.release();
      anchor = undefined;
    };
  });

  $effect(() => {
    anchor?.place(box);
  });

  // A press keeps the caret where it is: the line it belongs to stays the one acted on.
  const keep = (event: Event) => event.preventDefault();
</script>

{#if box}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="handle" bind:this={surface} onmousedown={keep} data-block-handle>
    <IconButton icon="plus" size="sm" tone="subtle" label={addLabel} data-block-add onclick={onadd} />
    <Menu label={handleLabel} {items} {onselect} placement={{ side: 'block-end', align: 'start' }}>
      {#snippet trigger(props)}
        <IconButton icon="grip-vertical" size="sm" tone="subtle" label={handleLabel} data-block-grip {...props} />
      {/snippet}
    </Menu>
  </div>
{/if}

<style>
  .handle {
    display: flex;
    gap: var(--sp-025);
    margin: 0;
    margin-inline-end: var(--sp-050);
    padding: 0;
    border: none;
    background: none;
    color: var(--text-subtle);
  }
</style>
