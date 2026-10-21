<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Why a control cannot be used now, said at the control and nowhere else (design-system.md §4).
  //
  // Underscored: a part, shared by every control that takes a `disabledReason` and by `Tooltip`,
  // so that they cannot disagree about when a bubble shows, how long it stays and what closes it.
  //
  // **It never takes space.** The bubble is rendered beside the control rather than around it -
  // a wrapper would become the flex item in a caller's row instead of the control - and it is out
  // of the flow at every moment: `hidden` while it is not showing, raised into the top layer by
  // `anchor.ts` while it is. Those are two switches, and they have to agree: a `[popover]` that
  // was not shown is `display: none` whatever `hidden` says (known-traps.md).
  //
  // **It is always in the tree.** The control's `aria-describedby` points at it whether or not it
  // shows, because a description added at the moment of focus is announced by nobody - focus was
  // announced before the attribute arrived. A reference to a hidden element still names its text.
  //
  // **It shows** on a mouse over the control or over the bubble itself (SC 1.4.13, *hoverable*:
  // the reader can move onto it), on focus, and on a click or a tap of the control - the guard's
  // `onTried`. **It hides** when the pointer has left both for the `attach` role's duration, on
  // blur, on `Escape`, and on a press anywhere else. A finger's `pointerleave` arrives the moment
  // it lifts, so only a mouse's counts.

  import { anchorTo, type Placement } from './anchor.ts';
  import { graceOf, guard, type GuardOptions } from './reason.ts';

  interface Props {
    /** The id the control's `aria-describedby` names. */
    id: string;
    /** Resolved text (ADR-0011), and short: a reason, not a paragraph. */
    text: string;
    /**
     * The control it describes: what is hovered, focused and tried. For a composite that states one
     * reason for its parts, the composite itself - its parts' events reach it.
     */
    control: HTMLElement | null;
    /** What the bubble is drawn from, when that is not the control: a composite's mark. */
    anchor?: HTMLElement | null;
    /** What else forwards a click to the control - its `label` - and is guarded with it. */
    forwarders?: readonly (HTMLElement | null)[];
    /**
     * Whether trying the control is refused. True for an unavailable control; false where the
     * bubble only describes a control that works (`Tooltip`).
     */
    isGuarding?: boolean;
    guardOptions?: GuardOptions;
    placement?: Placement;
  }

  const {
    id,
    text,
    control,
    anchor,
    forwarders = [],
    isGuarding = true,
    guardOptions = {},
    placement = { side: 'block-start', align: 'start' },
  }: Props = $props();

  let bubble = $state<HTMLElement | null>(null);
  let isShowing = $state(false);
  /** The control's writing direction, taken when the bubble shows (see the markup). */
  let direction = $state<'ltr' | 'rtl'>('ltr');
  let leaving: ReturnType<typeof setTimeout> | undefined;

  function show() {
    clearTimeout(leaving);
    direction = control && getComputedStyle(control).direction === 'rtl' ? 'rtl' : 'ltr';
    isShowing = true;
  }

  function hide() {
    clearTimeout(leaving);
    isShowing = false;
  }

  /** A focused control keeps its bubble: the reader is still there, only the mouse moved on. */
  function hideAfterGrace() {
    clearTimeout(leaving);
    leaving = setTimeout(() => {
      if (control?.matches(':focus-visible, :has(:focus-visible)')) return;
      isShowing = false;
    }, graceOf(control));
  }

  const onEnter = (event: PointerEvent) => {
    if (event.pointerType === 'mouse') show();
  };
  const onLeave = (event: PointerEvent) => {
    if (event.pointerType === 'mouse') hideAfterGrace();
  };

  $effect(() => {
    const node = control;
    if (!node) return;
    // A composite's parts pass focus among themselves: leaving one for the next is not leaving.
    // Asked once focus has settled rather than from `relatedTarget`: WebKit moves focus between a
    // date field's own parts with a focusout that names nobody, and the field has not been left.
    const onFocusout = () => {
      setTimeout(() => {
        if (!node.contains(document.activeElement)) hide();
      });
    };
    const onKeydown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || !isShowing) return;
      hide();
      // Consumed only because it was showing: a dialog underneath keeps every other `Escape`
      // (layers.ts).
      event.preventDefault();
    };
    node.addEventListener('pointerenter', onEnter);
    node.addEventListener('pointerleave', onLeave);
    // `focus` and `blur` caught on the way down rather than `focusin`: WebKit sends no `focusin` for
    // a date field.
    node.addEventListener('focus', show, { capture: true });
    node.addEventListener('blur', onFocusout, { capture: true });
    node.addEventListener('keydown', onKeydown);
    const lift = isGuarding ? guard([node, ...forwarders], show, guardOptions) : () => {};
    return () => {
      clearTimeout(leaving);
      lift();
      node.removeEventListener('pointerenter', onEnter);
      node.removeEventListener('pointerleave', onLeave);
      node.removeEventListener('focus', show, { capture: true });
      node.removeEventListener('blur', onFocusout, { capture: true });
      node.removeEventListener('keydown', onKeydown);
    };
  });

  $effect(() => {
    const node = bubble;
    if (!node) return;
    node.addEventListener('pointerenter', onEnter);
    node.addEventListener('pointerleave', onLeave);
    return () => {
      node.removeEventListener('pointerenter', onEnter);
      node.removeEventListener('pointerleave', onLeave);
    };
  });

  // A press anywhere but the control and the bubble closes it - listened for only while it shows,
  // so a page with forty unavailable controls does not carry forty document listeners.
  $effect(() => {
    if (!isShowing) return;
    const onPress = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && (control?.contains(target) || anchor?.contains(target) || bubble?.contains(target))) return;
      hide();
    };
    document.addEventListener('pointerdown', onPress, { capture: true });
    return () => document.removeEventListener('pointerdown', onPress, { capture: true });
  });

  $effect(() => {
    const from = anchor ?? control;
    if (!isShowing || !from || !bubble) return;
    return anchorTo(from, bubble, { placement });
  });
</script>

<!-- The bubble is left-to-right and only its text follows the control's direction, and it lines up
     with the control's left edge in both directions. Firefox places an anchored element whose
     area reaches the anchor's other edge - centred, or spanning towards the start in a
     right-to-left box - a viewport's width away from its control, measured by the workbench
     layout test; the left edge is the one it resolves in every engine. -->
<span {id} class="bubble" role="tooltip" dir="ltr" hidden={!isShowing} bind:this={bubble}><span class="text" dir={direction}>{text}</span></span>

<style>
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
    font-weight: var(--fw-regular);
    line-height: var(--lh-snug);
    text-align: start;
    white-space: normal;
    overflow-wrap: anywhere;
    animation: appear var(--motion-attach-duration) var(--motion-attach-easing) both;
  }

  .bubble[hidden] { display: none; }

  .text { display: block; }

  /* Opacity alone, because a `transform` here is a trap. `animation-fill-mode:
     both` leaves the last keyframe standing, and a `transform: none` keyframe computes to an
     identity *matrix* rather than to `none` - which still makes the element a containing block for
     every `position: fixed` descendant. Rule 6 allows both properties; only one of them is safe
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
