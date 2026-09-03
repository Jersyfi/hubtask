<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A surface anchored to the control that opened it, holding anything.
  //
  // Not a dialog: nothing behind it is inert, it takes no scrim, and the page keeps working while
  // it is open. That is the whole reason both exist - a modal is a claim that nothing else matters
  // until it is answered, and a filter panel is not that claim.
  //
  // The trigger is a snippet that receives the attributes it must carry: `aria-expanded` and
  // `aria-controls` belong on the control a reader operates, not on a wrapper around it, and this
  // is the only way to put them there while the control itself stays the caller's. The wrapper is
  // the anchor, which is also what `openOverlay` measures against.

  import type { Snippet } from 'svelte';

  import { focusReturn, focusables } from './focus.ts';
  import { openOverlay } from './overlay.ts';
  import type { Placement } from './positioning.ts';

  interface Props {
    /** What the surface is called, for the reader who arrives on it without seeing the trigger. */
    label: string;
    placement?: Placement;
    /** Open state, bindable: a caller that opens it from elsewhere is the ordinary case. */
    isOpen?: boolean;
    /** The control that opens it. It is handed the attributes it has to carry. */
    trigger: Snippet<[Record<string, unknown>]>;
    children: Snippet;
  }

  let {
    label,
    placement = { side: 'block-end', align: 'start' },
    isOpen = $bindable(false),
    trigger,
    children,
  }: Props = $props();

  const id = `popover-${Math.random().toString(36).slice(2, 9)}`;

  let anchor = $state<HTMLElement | null>(null);
  let surface = $state<HTMLElement | null>(null);
  /** What had focus when it opened, so that closing puts it back (F1-06's acceptance). */
  let opener: Element | null = null;

  const triggerProps = $derived({
    'aria-expanded': isOpen,
    'aria-controls': isOpen ? id : undefined,
    'aria-haspopup': 'dialog',
    onclick: () => toggle(),
  });

  function toggle() {
    opener = document.activeElement;
    isOpen = !isOpen;
  }

  function close() {
    if (!isOpen) return;
    isOpen = false;
    // The trigger can be gone - the popover's own content may have removed it - and focusing a
    // detached element drops focus on the body without saying so.
    if (!focusReturn(opener)) focusReturn(anchor?.firstElementChild);
  }

  $effect(() => {
    if (!isOpen || !anchor || !surface) return;
    const release = openOverlay({ layer: 'popover', trigger: anchor, surface, placement, onDismiss: close });
    // Focus moves into the surface: a popover that opened behind the reader's focus is one they
    // have to hunt for with the Tab key.
    focusables(surface)[0]?.focus();
    return release;
  });
</script>

<span class="anchor" bind:this={anchor}>
  {@render trigger(triggerProps)}
</span>
{#if isOpen}
  <div {id} class="surface" role="dialog" aria-label={label} bind:this={surface}>
    {@render children()}
  </div>
{/if}

<style>
  .anchor { display: inline-flex; }

  .surface {
    position: fixed;
    /* Anchored to a trigger and dismissed by it, which is what the `popover` rank means in
       tokens.json. Above a dialog, because one opened from inside a dialog belongs on top of it. */
    z-index: var(--z-popover);
    /* Rule 4: it is as wide as its content up to a limit, and never a fixed width. */
    max-width: 40ch;
    max-height: 80vh;
    overflow: auto;
    margin: var(--sp-050);
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    /* Rule 1: a temporary overlay is raised off the page it covers. */
    box-shadow: var(--shadow-overlay);
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    animation: open var(--dur-fast) var(--ease-entrance) both;
  }

  @keyframes open {
    from { opacity: 0; transform: scale(0.98); }
    to { opacity: 1; transform: none; }
  }

  @media (prefers-reduced-motion: reduce) {
    .surface { animation: none; }
  }

  :global([data-motion='reduced']) .surface { animation: none; }
</style>
