<!-- SPDX-License-Identifier: Apache-2.0
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

  import Sheet from './Sheet.svelte';
  import { focusReturn, focusables } from './focus.ts';
  import { isCompactWidth, closeName } from './media.ts';
  import { openOverlay } from './overlay.ts';
  import type { Placement } from './anchor.ts';

  interface Props {
    /** What the surface is called, for the reader who arrives on it without seeing the trigger. */
    label: string;
    placement?: Placement;
    /** Open state, bindable: a caller that opens it from elsewhere is the ordinary case. */
    isOpen?: boolean;
    /** The control that opens it. It is handed the attributes it has to carry. */
    trigger?: Snippet<[Record<string, unknown>]>;
    /**
     * Instead of a trigger: the control it hangs from when another door opened it - a menu's ⋯
     * opening the same picker the rail opens (one picker from every door).
     */
    at?: HTMLElement;
    children: Snippet;
    /**
     * Stays at its trigger on every width: the navigation's and the app bar's own. Every other
     * popover - a picker above all - is a bottom sheet on a phone's width (work-surface concept
     * §7), named with its label and closed by the application's `setCloseLabel`.
     */
    keepsAnchor?: boolean;
  }

  let {
    label,
    placement = { side: 'block-end', align: 'start' },
    isOpen = $bindable(false),
    trigger,
    children,
    keepsAnchor = false,
    at,
  }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const id = `popover-${instanceId}`;

  let anchor = $state<HTMLElement | null>(null);
  let surface = $state<HTMLElement | null>(null);
  /** What had focus when it opened, so that closing puts it back (design-system.md §10, 2.4.3). */
  let opener: Element | null = null;
  /** Opened as a sheet, and the name of its close: decided at the moment it opens. */
  let sheetName = $state<string | undefined>(undefined);

  // Decided whenever it opens, however it was opened - its trigger or a caller's binding.
  $effect.pre(() => {
    if (!isOpen) return;
    const name = keepsAnchor ? undefined : closeName();
    sheetName = name !== undefined && isCompactWidth() ? name : undefined;
  });

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
    const from = at ?? anchor;
    if (!isOpen || sheetName !== undefined || !from || !surface) return;
    if (at && !opener) opener = at;
    const release = openOverlay({ layer: 'popover', trigger: from, surface, placement, onDismiss: close });
    // Focus moves into the surface: a popover that opened behind the reader's focus is one they
    // have to hunt for with the Tab key.
    focusables(surface)[0]?.focus();
    return release;
  });
</script>

{#if trigger}
  <span class="anchor" bind:this={anchor}>
    {@render trigger(triggerProps)}
  </span>
{/if}
{#if sheetName !== undefined}
  <Sheet bind:isOpen title={label} closeLabel={sheetName} onClose={() => (sheetName = undefined)}>
    {@render children()}
  </Sheet>
{:else if isOpen}
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
    background: var(--bg-raised);
    /* Rule 1: a temporary overlay is raised off the page it covers. */
    box-shadow: var(--shadow-overlay);
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    animation: open var(--motion-attach-duration) var(--motion-attach-easing) both;
  }

  /* Opacity alone, because a `transform` here is a trap. `animation-fill-mode:
     both` leaves the last keyframe standing, and a `transform: none` keyframe computes to an
     identity *matrix* rather than to `none` - which still makes the element a containing block for
     every `position: fixed` descendant. A popover opened from inside this one is then laid out in
     its box and clipped by its `overflow`. Rule 6 allows both properties; only one of them is safe
     for a surface that other overlays are opened from. */
  @keyframes open {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .surface { animation: none; }
  }

  :global([data-motion='reduced']) .surface { animation: none; }
</style>
