<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // On, off, or partly - the third state a parent row has when some of its children are done.
  //
  // The native input carries the state and the keyboard; the box beside it is what is painted,
  // because a styled `appearance: none` checkbox loses the platform's own indeterminate rendering
  // and its high-contrast-mode outline. The input is not hidden with `display: none`, which would
  // take it out of the accessibility tree - it is transparent and on top, so the pointer target is
  // the control itself.

  import type { HTMLInputAttributes } from 'svelte/elements';

  import Icon from './Icon.svelte';
  import Reason from './_Reason.svelte';
  import Spinner from './Spinner.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import type { Busyable, Disableable } from './control.ts';
  import { guard, sharedReason } from './reason.ts';

  interface Props extends Omit<HTMLInputAttributes, 'disabled' | 'type' | 'checked'>, Disableable, Busyable {
    label: string;
    /** What it is doing while busy, said to a reader: the present participle (voice-and-tone.md §2.4). */
    busyLabel?: string;
    /** Standing help under the label. */
    hint?: string;
    /**
     * What is wrong now - a box the form needs ticked and that is not. Said under the label, the
     * box `aria-invalid`, so a form finds the first thing to fix (design-system.md §4, case 3).
     */
    error?: string;
    /** Some but not all: `aria-checked="mixed"`, and the box shows a bar rather than a tick. */
    isIndeterminate?: boolean;
    /**
     * Announced but not drawn, for a checkbox whose meaning the row around it already carries.
     *
     * A completion box in a list of entries is the case: the title beside it is what the reader
     * sees, and a second "Completed" printed against every row would be noise on screen and
     * nothing in the accessibility tree. Hidden, **never** absent — a checkbox with no accessible
     * name is a control a screen reader announces as "checkbox" and nothing else.
     */
    isLabelHidden?: boolean;
    checked?: boolean;
  }

  let {
    label,
    hint,
    error,
    disabledReason,
    isIndeterminate = false,
    isLabelHidden = false,
    isBusy = false,
    busyLabel,
    checked = $bindable(false),
    ...rest
  }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const uid = `checkbox-${instanceId}`;
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const errorId = $derived(error ? `${uid}-error` : undefined);

  // Unavailable, it stays focusable and says why in a bubble at the box: `aria-disabled` rather
  // than the native attribute, and the guard (reason.ts) refuses the click - on the box and on the
  // label that forwards one to it - and `Space`. Busy, it shows progress inside the box and is
  // refused the same way, without a reason: it is not unavailable, it is working.
  const unavailable = $derived(disabledReason !== undefined);
  const shared = sharedReason();
  const groupReasonId = $derived(shared(disabledReason));
  const ownReasonId = `${uid}-reason`;
  const reasonId = $derived(disabledReason === undefined ? undefined : (groupReasonId ?? ownReasonId));
  const busyId = $derived(isBusy && busyLabel ? `${uid}-busy` : undefined);
  const describedBy = $derived([errorId, reasonId, busyId, hintId].filter(Boolean).join(' ') || undefined);
  let native = $state<HTMLInputElement | null>(null);
  let labelElement = $state<HTMLLabelElement | null>(null);

  $effect(() => {
    if (!isBusy || unavailable) return;
    return guard([native, labelElement], () => {});
  });

  // Off for a reason a group states once - a composite's mark, or a notice the caller drew over a
  // whole area - the box draws no bubble of its own, but it is still refused here: an area's notice
  // has no guard of its own, and a box that flipped while it said it was unavailable would be the
  // one lie this rule exists to prevent. Under a composite's mark the mark's guard runs first.
  $effect(() => {
    if (!unavailable || !groupReasonId) return;
    return guard([native, labelElement], () => {});
  });
</script>

<div class="checkbox">
  <div class="row">
    <span class="control">
      <input
        id={uid}
        type="checkbox"
        class="native"
        bind:checked
        indeterminate={isIndeterminate}
        aria-disabled={unavailable ? 'true' : undefined}
        aria-busy={isBusy ? 'true' : undefined}
        aria-invalid={error ? 'true' : undefined}
        aria-describedby={describedBy}
        bind:this={native}
        {...rest}
      />
      <span class="box" aria-hidden="true">
        {#if isBusy}
          <Spinner size="sm" />
        {:else if isIndeterminate}
          <Icon name="minus" size="sm" />
        {:else if checked}
          <Icon name="check" size="sm" />
        {/if}
      </span>
      {#if busyId}
        <!-- Inside the box's own span, which is positioned: a hidden sentence there is no flex
             item of the column and adds no gap. -->
        <span id={busyId}><VisuallyHidden>{busyLabel}</VisuallyHidden></span>
      {/if}
    </span>
    <label class="label" class:is-hidden={isLabelHidden} for={uid} bind:this={labelElement}>{label}</label>
  </div>
  {#if disabledReason !== undefined && !groupReasonId}
    <Reason id={ownReasonId} text={disabledReason} control={native} forwarders={[labelElement]} />
  {/if}
  {#if error}
    <p id={errorId} class="message error">{error}</p>
  {/if}
  {#if hint}
    <p id={hintId} class="message">{hint}</p>
  {/if}
</div>

<style>
  .checkbox { display: flex; flex-direction: column; gap: var(--sp-050); }

  /* The label's size on the row, so `1lh` below is the label's line. */
  .row { display: flex; align-items: start; gap: var(--sp-100); font-size: var(--fs-100); }

  /* Announced but not drawn. Never `display: none`, which would take the name out of the
     accessibility tree along with the text — and a checkbox with no name is a control a screen
     reader can only call "checkbox". */
  .label.is-hidden {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .control {
    position: relative;
    display: inline-flex;
    flex: none;
    /* Centred on the first line of the label rather than on the block, so a label that wraps
       does not drag the box down with it - and a label of one line stands centred beside it. */
    margin-block-start: calc((1lh - var(--sp-250)) / 2);
  }

  /* On top and transparent, never `display: none`: the input is what the keyboard and the
     accessibility tree see, and hiding it would remove both. */
  .native {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    cursor: pointer;
  }

  .native[aria-disabled='true'] { cursor: not-allowed; }

  /* The input above is the control; this is a picture of it, and a picture never takes the click
     (the failure is worked through in Switch.svelte, where a moved knob did). */
  .box {
    pointer-events: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--sp-250);
    height: var(--sp-250);
    border: var(--bw-thick) solid var(--border-strong);
    border-radius: var(--r-xs);
    background: var(--bg-surface);
    color: var(--text-inverse);
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .native:checked + .box,
  .native:indeterminate + .box {
    background: var(--accent-primary);
    border-color: var(--accent-primary);
  }

  /* Rule 5. The ring is on the painted box, because the input it belongs to is invisible. */
  .native:focus-visible + .box {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .native:hover:not([aria-disabled='true']) + .box { border-color: var(--accent-primary); }

  .native[aria-disabled='true'] + .box {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    color: var(--text-subtle);
  }

  .native[aria-disabled='true']:checked + .box,
  .native[aria-disabled='true']:indeterminate + .box { background: var(--border-default); border-color: var(--border-default); }

  .label {
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    cursor: pointer;
    overflow-wrap: anywhere;
  }

  /* `:has` on the row rather than a sibling combinator: the label is not a sibling of the input,
     it is a sibling of the span the input sits in. */
  .row:has(.native[aria-disabled='true']) .label { color: var(--text-subtle); cursor: not-allowed; }

  .message {
    max-width: 60ch;
    margin: 0;
    /* Under the label, not under the box: the text is what it explains. */
    margin-inline-start: calc(var(--sp-250) + var(--sp-100));
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
  }

  .message.error { color: var(--text-danger); }

  .native[aria-invalid='true'] + .box { border-color: var(--status-danger-border); background: var(--status-danger-surface); }

  @media (prefers-reduced-motion: reduce) {
    .box { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .box { transition-duration: var(--dur-instant); }
</style>
