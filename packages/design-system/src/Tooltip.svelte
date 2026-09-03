<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A few words about the control under the pointer, and the component with the most rules per
  // line in this wave.
  //
  // **It is a description, not a name.** `aria-describedby`, set on the control itself rather than
  // on the wrapper - a description announced for a wrapper is a description nobody hears. An
  // icon-only control still needs its own name through `VisuallyHidden`; a tooltip that was the
  // name would vanish for anyone who never hovers.
  //
  // **It is not in the layer register.** `layers.ts` says why: a tooltip paints above a dialog and
  // is dismissed by the pointer or the blur that summoned it, so putting it in the register would
  // let it swallow the `Escape` meant for the dialog underneath. It handles `Escape` itself and
  // only while it is showing, which is what SC 1.4.13 requires and the narrowest reading of it.
  //
  // **It never intercepts the pointer.** The bubble is `pointer-events: none`, so the control
  // under it stays clickable and the tooltip cannot be the thing that swallows a click.

  import type { Snippet } from 'svelte';

  import { anchorTo, type Placement } from './positioning.ts';

  interface Props {
    /** The description. Resolved text (ADR-0011), and short: this is not a place for a paragraph. */
    text: string;
    /** Where it sits. Logical, so it follows the writing direction (ADR-0039). */
    placement?: Placement;
    /** The control being described. */
    children: Snippet;
  }

  const { text, placement = { side: 'block-start', align: 'center' }, children }: Props = $props();

  const id = `tooltip-${Math.random().toString(36).slice(2, 9)}`;

  let wrapper = $state<HTMLElement | null>(null);
  let bubble = $state<HTMLElement | null>(null);
  let isShowing = $state(false);

  /**
   * The described control is the first element inside the wrapper, and the attribute is set on it
   * rather than rendered here: the control is the caller's markup, and there is no way to hand it
   * a prop from out here.
   */
  $effect(() => {
    const control = wrapper?.firstElementChild;
    if (!control) return;
    if (isShowing) control.setAttribute('aria-describedby', id);
    else control.removeAttribute('aria-describedby');
  });

  $effect(() => {
    if (!isShowing || !wrapper || !bubble) return;
    return anchorTo(wrapper, bubble, { placement });
  });

  /**
   * The listeners are added rather than written as attributes, and the reason is a rule rather
   * than a preference: a `<span>` carrying `onpointerenter` is a static element with an
   * interaction, and the compiler asks for an ARIA role it must not be given. The interaction
   * belongs to the control *inside* the wrapper - which is the caller's markup - so a role here
   * would be a lie told to silence a warning.
   *
   * `focusin` and `focusout` rather than focus and blur: they bubble, and the control is a
   * descendant.
   */
  $effect(() => {
    const node = wrapper;
    if (!node) return;

    const show = () => (isShowing = true);
    const hide = () => (isShowing = false);
    const onKeydown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || !isShowing) return;
      isShowing = false;
      // Consumed only because it was showing. A dialog underneath keeps every other `Escape`,
      // which is the arrangement layers.ts describes.
      event.preventDefault();
    };

    node.addEventListener('pointerenter', show);
    node.addEventListener('pointerleave', hide);
    node.addEventListener('focusin', show);
    node.addEventListener('focusout', hide);
    node.addEventListener('keydown', onKeydown);
    return () => {
      node.removeEventListener('pointerenter', show);
      node.removeEventListener('pointerleave', hide);
      node.removeEventListener('focusin', show);
      node.removeEventListener('focusout', hide);
      node.removeEventListener('keydown', onKeydown);
    };
  });
</script>

<span class="wrapper" bind:this={wrapper}>
  {@render children()}
  {#if isShowing}
    <span {id} class="bubble" role="tooltip" bind:this={bubble}>{text}</span>
  {/if}
</span>

<style>
  .wrapper { display: inline-flex; }

  .bubble {
    position: fixed;
    /* Above everything it can sit over, including a dialog: a control described inside a modal is
       still described. The scale answers that (tokens.json), not a number written here. */
    z-index: var(--z-tooltip);
    max-width: 32ch;
    margin: var(--sp-050);
    padding: var(--sp-050) var(--sp-100);
    border-radius: var(--r-sm);
    /* Rule 1: a temporary overlay. The inverse surface rather than a glass one - glass is for a
       layer with content behind it that matters, and this covers a few pixels. */
    background: var(--text-primary);
    color: var(--bg-surface);
    font-size: var(--fs-075);
    line-height: var(--lh-snug);
    text-align: start;
    pointer-events: none;
    animation: appear var(--motion-attach-duration) var(--motion-attach-easing) both;
  }

  /* Opacity alone, and the `transform` that used to be here is the reason why. `animation-fill-mode:
     both` leaves the last keyframe standing, and a `transform: none` keyframe computes to an
     identity *matrix* rather than to `none` - which still makes the element a containing block for
     every `position: fixed` descendant. A popover opened from inside this one was then laid out in
     its box and clipped by its `overflow`. Rule 6 allows both properties; only one of them is safe
     for a surface that other overlays are opened from. */
  @keyframes appear {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .bubble { animation: none; }
  }

  :global([data-motion='reduced']) .bubble { animation: none; }
</style>
