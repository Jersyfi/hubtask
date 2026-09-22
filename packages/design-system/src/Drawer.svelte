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
  // that is what the direction axis exists to catch. `block-end` is the third edge (F8-05): a
  // sheet that rises from the bottom over a canvas that stays where it was, for a screen too
  // narrow to hold a panel beside it - the same one glass surface at a time (rule 2).
  //
  // A sheet **keeps its head and is sized by the reader** (F8-24, `milestone-F8.md` decision 27).
  // Its head - the title and the close - stays put and only the body scrolls, because a sheet
  // whose close scrolls away is a sheet that cannot be closed without scrolling back up. With
  // `isResizable` it carries a handle above the head: dragged, or moved by the arrow keys, it sizes
  // the sheet between a third and nine tenths of the screen, and let go below the floor it closes.
  // `size` is bindable so that **the caller keeps it** - where a reader's choice is remembered is
  // the application's question, never a component's.

  import type { Snippet } from 'svelte';

  import IconButton from './IconButton.svelte';
  import { escapeHandler, focusReturn } from './focus.ts';
  import { layers } from './layers.ts';

  interface Props {
    /** What the panel is, as a heading. Resolved text (ADR-0011). */
    title: string;
    isOpen?: boolean;
    /** Which edge it comes from, in logical terms. */
    edge?: 'inline-start' | 'inline-end' | 'block-end';
    /** The name of the close control. A drawer is always dismissible, so this is required. */
    dismissLabel: string;
    onClose?: () => void;
    /** The controls that belong to the panel rather than to its content. */
    actions?: Snippet;
    /** A sheet the reader sizes by a handle (F8-24): `block-end` only, where a sheet is what this is. */
    isResizable?: boolean;
    /** The sheet's share of the screen while it is resizable, between `MIN_SIZE` and `MAX_SIZE`. */
    size?: number;
    /** The handle's name for a screen reader. Required where `isResizable` is set. */
    resizeLabel?: string;
    children: Snippet;
  }

  let {
    title,
    isOpen = $bindable(false),
    edge = 'inline-end',
    dismissLabel,
    onClose,
    actions,
    isResizable = false,
    size = $bindable(0.5),
    resizeLabel,
    children,
  }: Props = $props();

  /** A third of the screen is the floor; below it the reader is closing the sheet, not sizing it. */
  const MIN_SIZE = 0.33;
  const MAX_SIZE = 0.9;
  const STEP = 0.05;

  const sheet = $derived(edge === 'block-end');
  const sized = $derived(sheet && isResizable);
  const clamp = (share: number): number => Math.min(MAX_SIZE, Math.max(MIN_SIZE, share));

  /** While the handle is held: the share the pointer is at, and whether it has gone below the floor. */
  let dragging = $state(false);
  let letting = $state(false);
  /** What the sheet measured when the drag began: what it keeps if the drag turns out to close it. */
  let before = 0.5;

  function shareAt(clientY: number): number {
    const height = window.innerHeight || 1;
    return (height - clientY) / height;
  }

  function grab(event: PointerEvent): void {
    if (!sized) return;
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
    before = size;
    dragging = true;
    letting = false;
    event.preventDefault();
  }

  function move(event: PointerEvent): void {
    if (!dragging) return;
    const share = shareAt(event.clientY);
    letting = share < MIN_SIZE;
    size = clamp(share);
  }

  function release(event: PointerEvent): void {
    if (!dragging) return;
    (event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId);
    dragging = false;
    // Let go below the floor: the reader pulled the sheet down, which is how a sheet is closed -
    // and closing it is not a wish for a sheet a third of the screen high next time, so the size
    // it had before the pull is what it keeps.
    if (letting) {
      letting = false;
      size = before;
      close();
    }
  }

  function sizeByKey(event: KeyboardEvent): void {
    const by: Record<string, number> = { ArrowUp: STEP, ArrowDown: -STEP, PageUp: STEP * 2, PageDown: -STEP * 2 };
    if (event.key in by) {
      size = clamp(size + by[event.key]!);
    } else if (event.key === 'Home') {
      size = MAX_SIZE;
    } else if (event.key === 'End') {
      size = MIN_SIZE;
    } else {
      return;
    }
    event.preventDefault();
  }

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
  class:sized
  class:letting
  data-edge={edge}
  style:--sheet-share={sized ? size : undefined}
  bind:this={node}
  aria-labelledby={titleId}
  oncancel={(event) => event.preventDefault()}
  onclose={() => close()}
  onclick={(event) => {
    if (event.target === node) close();
  }}
>
  <div class="panel">
    {#if sized}
      <!-- The handle: dragged with a pointer, moved with the arrow keys, and let go below the
           floor to close. A focusable separator with a value is ARIA's window splitter, which is
           a widget and takes the focus by design; the linter reads every separator as decoration. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        class="grip"
        role="separator"
        tabindex="0"
        aria-label={resizeLabel ?? dismissLabel}
        aria-orientation="horizontal"
        aria-valuemin={Math.round(MIN_SIZE * 100)}
        aria-valuemax={Math.round(MAX_SIZE * 100)}
        aria-valuenow={Math.round(size * 100)}
        onpointerdown={grab}
        onpointermove={move}
        onpointerup={release}
        onpointercancel={release}
        onkeydown={sizeByKey}
      >
        <span class="bar"></span>
      </div>
    {/if}
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
    /* A caller that is a navigation (NavDrawer) sets the width to the shell's measure through the
       custom property; every other drawer is as wide as a column of text. Inheritance reaches the
       top layer, which is why this needs no prop and no inline style. */
    inline-size: var(--drawer-inline-size, min(42ch, 100%));
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

  /* The sheet: the full inline size, as tall as its content up to most of the screen, rising from
     the bottom. `translate` takes the y here and no x, so the one keyframe set serves all three. */
  .drawer[data-edge='block-end'] {
    inline-size: 100%;
    block-size: auto;
    max-block-size: 78%;
    margin-inline: 0;
    margin-block: auto 0;
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    border-start-start-radius: var(--r-xl);
    border-start-end-radius: var(--r-xl);
    --slide-from: 0;
    --slide-from-y: 100%;
  }

  /* In RTL the same logical edge is the other physical side, so the sign flips. Without this a
     drawer at `inline-end` in Arabic sits on the left and slides in from the right, across the
     content it is meant to sit beside. */
  :global([dir='rtl']) .drawer[data-edge='inline-end'] { --slide-from: -100%; }
  :global([dir='rtl']) .drawer[data-edge='inline-start'] { --slide-from: 100%; }

  /* A sheet the reader sizes: exactly the share it was dragged to, its head staying while the
     body scrolls (decision 27). `block-size` rather than `max-block-size`, or a short panel would
     shrink back and the handle would jump under the finger. */
  .drawer.sized {
    block-size: calc(var(--sheet-share, 0.5) * 100%);
    max-block-size: 100%;
    overflow: hidden;
  }

  /* While it is being pulled below the floor: what letting go now would do, said by the sheet. */
  .drawer.letting { opacity: 0.8; }

  .drawer.sized .panel { block-size: 100%; min-block-size: 0; }

  .drawer.sized .body { flex: 1 1 auto; min-block-size: 0; overflow: auto; }

  .grip { margin-block: calc(-1 * var(--sp-150)) 0; padding-block: var(--sp-100); display: grid; place-items: center; cursor: ns-resize; touch-action: none; }

  .grip .bar { display: block; inline-size: var(--sp-600); block-size: var(--sp-050); border-radius: var(--r-full); background: var(--border-strong); }

  .grip:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }

  .drawer::backdrop { background: var(--bg-scrim); }

  .panel {
    display: flex;
    flex-direction: column;
    gap: var(--sp-200);
    padding: var(--sp-300);
    text-align: start;
  }

  /* The head stays, whatever the body does under it. */
  .drawer.sized .head { position: sticky; inset-block-start: 0; }

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
    from { opacity: 0; translate: var(--slide-from) var(--slide-from-y, 0); }
    to { opacity: 1; translate: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .drawer { animation: none; }
  }

  :global([data-motion='reduced']) .drawer { animation: none; }
</style>
