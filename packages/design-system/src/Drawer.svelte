<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A panel that comes in from the edge and does not take the screen away.
  //
  // It is a `<dialog>` like `Dialog`, and for the same four reasons: the top layer, so no
  // `z-index` can lose to a stacking context; a real focus trap; `inert` on everything behind it;
  // and a backdrop nobody can click through. What differs is what it is *for* - a dialog is a
  // claim that nothing else matters until it is answered, a drawer is a place to work beside what
  // is already on screen - so this one is always dismissible and always has a close control.
  //
  // `Escape` closes one layer at a time, which is `layers.ts`'s answer and not the platform's: a
  // popover opened from inside a drawer goes first. So `cancel` is refused here exactly as it is
  // in `Dialog`, and the register decides.
  //
  // The **edge** is `inline-start`/`inline-end`, never left and right. A drawer that opened from
  // the left in Arabic would be a drawer opening from the far side of the reading direction, and
  // that is what the direction axis exists to catch.

  import type { Snippet } from 'svelte';

  import IconButton from './IconButton.svelte';
  import { escapeHandler, focusReturn } from './focus.ts';
  import { layers } from './layers.ts';

  interface Props {
    /** What the panel is, as a heading. Resolved text (ADR-0011). */
    title: string;
    isOpen?: boolean;
    /** Which edge it comes from, in logical terms. */
    edge?: 'inline-start' | 'inline-end';
    /** The name of the close control. A drawer is always dismissible, so this is required. */
    dismissLabel: string;
    onClose?: () => void;
    /** The controls that belong to the panel rather than to its content. */
    actions?: Snippet;
    children: Snippet;
  }

  let {
    title,
    isOpen = $bindable(false),
    edge = 'inline-end',
    dismissLabel,
    onClose,
    actions,
    children,
  }: Props = $props();

  const titleId = `drawer-${Math.random().toString(36).slice(2, 9)}`;

  let node = $state<HTMLDialogElement | null>(null);
  let opener: Element | null = null;

  function close() {
    if (!isOpen) return;
    isOpen = false;
  }

  $effect(() => {
    const drawer = node;
    if (!drawer) return;

    if (!isOpen) {
      if (drawer.open) drawer.close();
      return;
    }

    opener = document.activeElement;
    if (!drawer.open) drawer.showModal();

    // `overlay` rather than `dialog`: a drawer is the weakest dismissible layer, so a dialog
    // opened from inside one closes first. That ordering is the register's whole purpose.
    const handle = layers.open('overlay', close);
    const onKeydown = escapeHandler();
    document.addEventListener('keydown', onKeydown);

    return () => {
      document.removeEventListener('keydown', onKeydown);
      handle.release();
      if (drawer.open) drawer.close();
      // Back to what opened it, for the reason `Dialog` asks: the one case the browser cannot
      // answer is a trigger removed by the action the drawer performed.
      focusReturn(opener);
      onClose?.();
    };
  });
</script>

<dialog
  class="drawer"
  data-edge={edge}
  bind:this={node}
  aria-labelledby={titleId}
  oncancel={(event) => event.preventDefault()}
  onclose={() => close()}
  onclick={(event) => {
    if (event.target === node) close();
  }}
>
  <div class="panel">
    <header class="head">
      <h2 id={titleId} class="title">{title}</h2>
      <div class="head-actions">
        {#if actions}{@render actions()}{/if}
        <IconButton icon="x" label={dismissLabel} size="sm" onclick={() => close()} />
      </div>
    </header>
    <div class="body">{@render children()}</div>
  </div>
</dialog>

<style>
  /* A modal `<dialog>` is in the top layer, so there is no `z-index` here - the same reasoning
     `Dialog` records. The `overlay` rank still decides what `Escape` reaches (layers.ts). */
  .drawer {
    padding: 0;
    border: 0;
    background: var(--bg-surface);
    color: var(--text-primary);
    box-shadow: var(--shadow-overlay);
    /* Pinned to one edge over the full block size, which is what makes it a drawer rather than a
       dialog. `margin-inline` does the sidedness so the whole thing mirrors in RTL. */
    block-size: 100%;
    max-block-size: 100%;
    inline-size: min(42ch, 100%);
    max-inline-size: 100%;
    margin-block: 0;
    overflow: auto;
  }

  /* One animation, and the direction it slides from is a custom property rather than a second
     keyframe set. `translate` is physical - it knows nothing about `inline-start` - so the sign is
     what has to mirror, and that is one declaration per case instead of four. */
  .drawer {
    animation: arrive var(--motion-entrance-duration) var(--motion-entrance-easing) both;
  }

  .drawer[data-edge='inline-end'] {
    margin-inline: auto 0;
    border-inline-start: var(--bw-hairline) solid var(--border-subtle);
    --slide-from: 100%;
  }

  .drawer[data-edge='inline-start'] {
    margin-inline: 0 auto;
    border-inline-end: var(--bw-hairline) solid var(--border-subtle);
    --slide-from: -100%;
  }

  /* In RTL the same logical edge is the other physical side, so the sign flips. Without this a
     drawer at `inline-end` in Arabic sits on the left and slides in from the right, across the
     content it is meant to sit beside. */
  :global([dir='rtl']) .drawer[data-edge='inline-end'] { --slide-from: -100%; }
  :global([dir='rtl']) .drawer[data-edge='inline-start'] { --slide-from: 100%; }

  .drawer::backdrop { background: var(--bg-scrim); }

  .panel {
    display: flex;
    flex-direction: column;
    gap: var(--sp-200);
    padding: var(--sp-300);
    text-align: start;
  }

  .head {
    display: flex;
    align-items: start;
    justify-content: space-between;
    gap: var(--sp-200);
  }

  .head-actions { display: flex; align-items: center; gap: var(--sp-100); }

  .title {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .body { font-size: var(--fs-100); min-width: 0; }

  /* Rule 6: opacity and transform only. */
  @keyframes arrive {
    from { opacity: 0; translate: var(--slide-from) 0; }
    to { opacity: 1; translate: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .drawer { animation: none; }
  }

  :global([data-motion='reduced']) .drawer { animation: none; }
</style>
