<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A panel opened at a box that is no element - the line "/" was typed in, the line whose "+" was
  // pressed (S-1) - through the phantom anchor of `rectAnchor` and the same `openOverlay` a menu
  // uses: a place in the register, `Escape`, and a press outside the container closes it. A press
  // inside the container does not: the writer goes on typing in the line it stands at.

  import type { Snippet } from 'svelte';

  import { rectAnchor, type Box } from './anchor.ts';
  import { openOverlay } from './overlay.ts';

  interface Props {
    /** What the panel is called. */
    label: string;
    /** The element the box lies in; the anchor is laid inside it. */
    container: HTMLElement | undefined;
    /** The box the panel opens under; absent draws nothing. */
    box: Box | undefined;
    /** Called by `Escape` and by a press outside the container. */
    onclose: () => void;
    children: Snippet;
  }

  const { label, container, box, onclose, children }: Props = $props();

  let surface = $state<HTMLElement | null>(null);

  $effect(() => {
    const host = container;
    const panel = surface;
    if (!host || !panel || !box) return;
    const anchor = rectAnchor(host);
    anchor.place(box);
    const close = openOverlay({
      layer: 'popover', trigger: host, anchor: anchor.element, surface: panel, placement: { side: 'block-end', align: 'start' }, onDismiss: () => onclose(),
    });
    return () => {
      close();
      anchor.release();
    };
  });
</script>

{#if box}
  <div class="panel" role="group" aria-label={label} bind:this={surface} data-box-popover>
    {@render children()}
  </div>
{/if}

<style>
  .panel {
    max-inline-size: min(100vw, var(--layout-rail));
    max-block-size: var(--layout-rail);
    margin: 0;
    margin-block: var(--sp-050);
    padding: var(--sp-100);
    overflow: auto;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    box-shadow: var(--shadow-overlay);
  }
</style>
