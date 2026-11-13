<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The writing bar above selected text (S-1): a `Toolbar` drawn over a box that is no element -
  // the selection - through a phantom anchor inside the container (`rectAnchor`), so it follows
  // the text through a scroll and flips below it where the top has no room.
  //
  // Pressing it never takes the selection away: a pointer press on the bar is not a focus change,
  // so the words stay selected while a mark is applied. The keyboard reaches it as any toolbar,
  // one stop and the arrows.

  import type { Snippet } from 'svelte';

  import Toolbar from './Toolbar.svelte';
  import { anchorTo, rectAnchor, type Box, type RectAnchor } from './anchor.ts';

  interface Props {
    /** What the bar is called. */
    label: string;
    /** The element the selection's text lies in; the anchor is laid inside it. */
    container: HTMLElement | undefined;
    /** The selection's box in the viewport; absent draws nothing. */
    box: Box | undefined;
    children: Snippet;
  }

  const { label, container, box, children }: Props = $props();

  let bar = $state<HTMLElement | null>(null);
  let anchor = $state<RectAnchor | undefined>(undefined);

  $effect(() => {
    const host = container;
    const surface = bar;
    if (!host || !surface) return;
    const made = rectAnchor(host);
    made.place(box);
    const untie = anchorTo(made.element, surface, { placement: { side: 'block-start', align: 'center' } });
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

  const keep = (event: PointerEvent) => {
    if (!(event.target instanceof HTMLInputElement)) event.preventDefault();
  };
</script>

{#if box}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="floating" bind:this={bar} onpointerdown={keep} onmousedown={keep as never} data-floating-toolbar>
    <Toolbar {label}>{@render children()}</Toolbar>
  </div>
{/if}

<style>
  .floating {
    margin: 0;
    margin-block: var(--sp-100);
    padding: var(--sp-050);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    box-shadow: var(--shadow-overlay);
  }
</style>
