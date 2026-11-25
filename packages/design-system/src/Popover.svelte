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

  import { tick, type Snippet } from 'svelte';

  import Sheet from './Sheet.svelte';
  import { focusReturn, focusables } from './focus.ts';
  import { isCompactWidth, closeName } from './media.ts';
  import { openOverlay } from './overlay.ts';
  import type { Placement } from './anchor.ts';
  import { scrollEdges } from './scrollEdges.ts';

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
    isAnchored?: boolean;
    /** The buttons at the sheet's foot where it is drawn as one - a list's *Done*. */
    sheetActions?: Snippet;
    /**
     * Whether a tip points from the surface at its trigger: a help tip's ⓘ. The surface is then as
     * wide as its content and placed by `anchor.ts`'s `placeWithTip`.
     */
    hasTip?: boolean;
  }

  let {
    label,
    placement = { side: 'block-end', align: 'start' },
    isOpen = $bindable(false),
    trigger,
    children,
    isAnchored = false,
    at,
    sheetActions,
    hasTip = false,
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
    const name = isAnchored ? undefined : closeName();
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

  /**
   * Its trigger scrolled out of the window: a surface with a tip would stand at the edge pointing
   * at nothing. It closes, and focus goes back to the trigger without scrolling the page back to it.
   */
  function lose() {
    if (!isOpen) return;
    const inside = surface?.contains(document.activeElement) ?? false;
    isOpen = false;
    const back = (opener ?? anchor?.firstElementChild) as HTMLElement | null | undefined;
    if (inside && back?.isConnected) back.focus({ preventScroll: true });
  }

  // The sheet returns focus to what had it when it opened. A press that does not focus a button -
  // Safari's, and Firefox's on macOS - left nothing, and focus is lost with the sheet: once it is
  // gone, focus goes to the control the popover hangs from.
  async function closeSheet() {
    sheetName = undefined;
    await tick();
    const now = document.activeElement;
    if (now === null || now === document.body) focusReturn(at ?? anchor?.firstElementChild);
  }

  $effect(() => {
    const from = at ?? anchor;
    if (!isOpen || sheetName !== undefined || !from || !surface) return;
    if (at && !opener) opener = at;
    const release = openOverlay({ layer: 'popover', trigger: from, surface, placement, tip: hasTip ? { onLost: lose } : undefined, onDismiss: close });
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
  <Sheet bind:isOpen title={label} closeLabel={sheetName} layer="popover" actions={sheetActions} onClose={closeSheet}>
    {@render children()}
  </Sheet>
{:else if isOpen}
  <div {id} class="surface" role="dialog" aria-label={label} data-tip={hasTip ? '' : undefined} bind:this={surface}>
    <!-- The surface is the frame that does not scroll and carries the shades; this scrolls. -->
    <div class="scroll" {@attach scrollEdges}>{@render children()}</div>
    {#if hasTip}<span class="tip" aria-hidden="true"></span>{/if}
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
    display: flex;
    flex-direction: column;
    overflow: hidden;
    margin: var(--sp-050);
    /* The padding is the inner box's, which scrolls; none from the popover's own default. */
    padding: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    /* anchor.ts's slack for its shrunk fallback, grown by what comes on top of the content box it
       sizes - the border on both sides; the padding is inside it, on the part that scrolls. Without
       it a list near the middle of the screen fits on neither side. */
    --hbt-fit-slack: calc(var(--sp-200) + var(--bw-hairline) * 2);
    border-radius: var(--r-lg);
    background: var(--bg-raised);
    /* Rule 1: a temporary overlay is raised off the page it covers. */
    box-shadow: var(--shadow-overlay);
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    animation: open var(--motion-attach-duration) var(--motion-attach-easing) both;
  }

  /* With a tip: as wide as its content whatever room is left beside the trigger, so placing it never
     changes its width; and nothing clipped, so the tip can stand outside it. The scroller keeps the
     rounded corners instead. */
  .surface[data-tip] {
    inline-size: max-content;
    max-inline-size: min(40ch, calc(100% - 2 * var(--sp-200)));
    overflow: visible;
    --hbt-tip: var(--layout-tooltip-helptip);
  }

  .surface[data-tip] .scroll { border-radius: inherit; }

  /* The tip: a square turned a quarter of the way in the surface's colour, with its hairline. The
     triangle that stands out is kept and the half inside cut away, all but a strip as wide as the
     surface's border, which it covers where the tip joins it. */
  .tip {
    position: absolute;
    z-index: -1;
    box-sizing: border-box;
    inline-size: calc(var(--hbt-tip) * 1.4142);
    block-size: calc(var(--hbt-tip) * 1.4142);
    inset-inline-start: calc(var(--hbt-tip-inline) - var(--bw-hairline) - var(--hbt-tip) * 0.7071);
    inset-block-start: calc(var(--hbt-tip-block) - var(--bw-hairline) - var(--hbt-tip) * 0.7071);
    border: var(--bw-hairline) solid var(--border-subtle);
    background: var(--bg-raised);
    rotate: 45deg;
  }

  /* Where anchor.ts puts the tip, set by its rule; nothing until it has. */
  :where(.surface) {
    --hbt-tip-inline: 0;
    --hbt-tip-block: 0;
  }

  .surface:not(:global([data-hbt-tip])) .tip { display: none; }
  .tip { --strip: calc(2 * var(--bw-hairline)); }
  .surface:global([data-hbt-tip='top']) .tip { clip-path: polygon(0 0, 100% 0, 100% var(--strip), var(--strip) 100%, 0 100%); }
  .surface:global([data-hbt-tip='right']) .tip { clip-path: polygon(0 0, 100% 0, 100% 100%, calc(100% - var(--strip)) 100%, 0 var(--strip)); }
  .surface:global([data-hbt-tip='bottom']) .tip { clip-path: polygon(100% 0, 100% 100%, 0 100%, 0 calc(100% - var(--strip)), calc(100% - var(--strip)) 0); }
  .surface:global([data-hbt-tip='left']) .tip { clip-path: polygon(0 0, var(--strip) 0, 100% calc(100% - var(--strip)), 100% 100%, 0 100%); }

  /* The content scrolls inside the surface, so the surface can carry the shades at its edges. */
  .scroll {
    flex: 1 1 auto;
    min-block-size: 0;
    overflow: auto;
    padding: var(--sp-200);
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
