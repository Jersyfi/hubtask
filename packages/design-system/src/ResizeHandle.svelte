<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The edge a person drags to make a region wider or narrower: the navigation column, the entry
  // panel (design-system.md §11.3, §11.8). A `separator` with a value, so a screen reader hears
  // the width and the arrows change it — the keyboard way to the same act, never a second one.
  //
  // It owns no width. The region does, and the caller keeps it (on the device, §11.10); this
  // draws the edge and reports where the person wants it, clamped to the bounds it is given.
  // `edge` is where the handle sits on the region, so dragging away from the region grows it on
  // either side and in either reading direction.

  interface Props {
    /** What is resized, for the separator's name: "Width of the navigation". Resolved text. */
    label: string;
    /** The width in CSS pixels. */
    value: number;
    min: number;
    max: number;
    /** What a double-click puts back. */
    reset: number;
    /** Which edge of the region the handle stands on. */
    edge: 'inline-start' | 'inline-end';
    /** How far one arrow press moves the edge. */
    step?: number;
    /** Every new width while the person drags or presses; the caller keeps the last. */
    onresize: (width: number) => void;
  }

  const { label, value, min, max, reset, edge, step = 16, onresize }: Props = $props();

  let handle: HTMLElement | undefined = $state();
  let dragging = $state(false);

  const clamp = (width: number) => Math.round(Math.min(max, Math.max(min, width)));

  /** +1 when moving the pointer towards the physical right widens the region. */
  function growth(): number {
    const rtl = handle ? getComputedStyle(handle).direction === 'rtl' : false;
    const towardsEnd = edge === 'inline-end' ? 1 : -1;
    return rtl ? -towardsEnd : towardsEnd;
  }

  function start(event: PointerEvent) {
    if (event.button !== 0 || !handle) return;
    event.preventDefault();
    const origin = event.clientX;
    const from = value;
    const sign = growth();
    dragging = true;
    handle.setPointerCapture(event.pointerId);
    const move = (moved: PointerEvent) => onresize(clamp(from + (moved.clientX - origin) * sign));
    const end = () => {
      dragging = false;
      handle?.removeEventListener('pointermove', move);
      handle?.removeEventListener('pointerup', end);
      handle?.removeEventListener('pointercancel', end);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  }

  function key(event: KeyboardEvent) {
    // The arrows move the edge the way they point; which way that grows the region depends on
    // the edge and the direction, the same as the pointer.
    const sign = growth();
    const moves: Record<string, () => number> = {
      ArrowRight: () => value + step * sign,
      ArrowLeft: () => value - step * sign,
      Home: () => min,
      End: () => max,
    };
    const next = moves[event.key];
    if (!next) return;
    event.preventDefault();
    onresize(clamp(next()));
  }
</script>

<!-- Focusable, because a separator with a value is a control (WAI-ARIA window splitter): Svelte's
     check reads every separator as static. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div
  bind:this={handle}
  class="handle"
  role="separator"
  aria-orientation="vertical"
  aria-label={label}
  aria-valuenow={value}
  aria-valuemin={min}
  aria-valuemax={max}
  tabindex="0"
  data-edge={edge}
  data-dragging={dragging ? '' : undefined}
  onpointerdown={start}
  onkeydown={key}
  ondblclick={() => onresize(clamp(reset))}
></div>

<style>
  /* A strip over the region's edge, wider than the line it draws, so a pointer finds it. The
     region positions it (`position: relative` on the region); `touch-action: none` lets a finger
     drag it, and it is on the strip alone. */
  .handle {
    position: absolute;
    inset-block: 0;
    inline-size: var(--sp-150);
    cursor: col-resize;
    touch-action: none;
    z-index: var(--z-raised);
  }

  .handle[data-edge='inline-end'] { inset-inline-end: calc(-0.5 * var(--sp-150)); }
  .handle[data-edge='inline-start'] { inset-inline-start: calc(-0.5 * var(--sp-150)); }

  /* The line, drawn where the pointer or the focus is, and while dragging. */
  .handle::after {
    content: '';
    position: absolute;
    inset-block: 0;
    inset-inline-start: calc(50% - 0.5 * var(--bw-ring));
    inline-size: var(--bw-ring);
    background: var(--accent-primary);
    opacity: 0;
    transition: opacity var(--motion-state-duration) var(--motion-state-easing);
  }

  .handle:hover::after,
  .handle[data-dragging]::after,
  .handle:focus-visible::after { opacity: 1; }

  .handle:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(-1 * var(--bw-ring));
    border-radius: var(--r-xs);
  }

  @media (prefers-reduced-motion: reduce) {
    .handle::after { transition: none; }
  }

  :global([data-motion='reduced']) .handle::after { transition: none; }
</style>
