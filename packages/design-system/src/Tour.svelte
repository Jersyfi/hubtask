<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The tour's pattern (design-system.md §8, F6-14): a spotlight on one element of the real
  // interface and a coach mark beside it. Never a slide show - the element is the real one, left
  // interactive, and what is dimmed is everything else.
  //
  // **The cut-out is positioned by CSS.** `spotlightTo` gives it the element's box through anchor
  // positioning (ADR-0039) - no measuring, no listener, and it follows the element through a
  // resize. The overlay of rule 2 is the cut-out's own shadow, cast as far as a viewport reaches:
  // the scrim the dialog's backdrop uses, and one surface rather than four panels. The mark is
  // anchored to the cut-out, below it and flipping above where there is no room.
  //
  // **`Escape` skips**, through the layer register like every overlay; the tab order is the mark's
  // (`CoachMark`). The caller decides what a step says, which element it points at and where to go
  // next; this only draws one.

  import CoachMark from './CoachMark.svelte';
  import { anchorTo, spotlightTo, type Placement } from './anchor.ts';
  import { escapeHandler } from './focus.ts';
  import { layers } from './layers.ts';

  interface Props {
    /** The element this step points at. Absent draws nothing: a step without its element is skipped by the caller. */
    target: HTMLElement | null;
    caption: string;
    body: string;
    countLabel: string;
    nextLabel: string;
    backLabel: string;
    skipLabel: string;
    hasBack?: boolean;
    placement?: Placement;
    onNext: () => void;
    onBack?: () => void;
    onSkip: () => void;
  }

  let {
    target,
    caption,
    body,
    countLabel,
    nextLabel,
    backLabel,
    skipLabel,
    hasBack = false,
    placement = { side: 'block-end', align: 'start' },
    onNext,
    onBack,
    onSkip,
  }: Props = $props();

  let cutout = $state<HTMLElement | null>(null);
  let surface = $state<HTMLElement | null>(null);

  $effect(() => {
    const element = target;
    const hole = cutout;
    const mark = surface;
    if (!element || !hole || !mark) return;
    // The cut-out first, so that the mark anchored to it comes after it in the top layer.
    const unspot = spotlightTo(element, hole);
    const unanchor = anchorTo(hole, mark, { placement });
    const handle = layers.open('dialog', onSkip);
    const onKeydown = escapeHandler();
    document.addEventListener('keydown', onKeydown);
    return () => {
      document.removeEventListener('keydown', onKeydown);
      handle.release();
      unanchor();
      unspot();
    };
  });
</script>

{#if target}
  <!-- The spotlight: the element's box with a margin of air, and the whole viewport dimmed
       around it. Nothing in it takes a pointer: the element beneath keeps its own. -->
  <div class="spotlight" aria-hidden="true" bind:this={cutout}></div>
  <CoachMark
    {caption}
    {body}
    {countLabel}
    {nextLabel}
    {backLabel}
    {skipLabel}
    {hasBack}
    {target}
    {onNext}
    {onBack}
    {onSkip}
    bind:surface
  />
{/if}

<style>
  .spotlight {
    position: fixed;
    z-index: var(--z-overlay);
    /* The air around the element: a negative margin grows the box the anchor gave it. */
    margin: calc(var(--sp-100) * -1);
    padding: var(--sp-100);
    box-sizing: content-box;
    border-radius: var(--r-md);
    pointer-events: none;
    background: transparent;
    /* Rule 2's overlay, as the cut-out's shadow cast past every edge of the viewport: the same
       scrim a dialog's backdrop draws, and one surface. `vmax` is the viewport's own measure. */
    box-shadow: 0 0 0 100vmax var(--bg-scrim);
    /* The user agent gives a `[popover]` a border and a background; the spotlight has neither. */
    border: 0;
  }
</style>
