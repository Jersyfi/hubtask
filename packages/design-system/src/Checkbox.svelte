<!-- SPDX-License-Identifier: BUSL-1.1
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
  import type { Disableable } from './control.ts';

  interface Props extends Omit<HTMLInputAttributes, 'disabled' | 'type' | 'checked'>, Disableable {
    label: string;
    /** Standing help under the label. */
    hint?: string;
    /** Some but not all: `aria-checked="mixed"`, and the box shows a bar rather than a tick. */
    isIndeterminate?: boolean;
    checked?: boolean;
  }

  let {
    label,
    hint,
    disabledReason,
    isIndeterminate = false,
    checked = $bindable(false),
    ...rest
  }: Props = $props();

  const uid = `checkbox-${Math.random().toString(36).slice(2, 9)}`;
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const reasonId = $derived(disabledReason ? `${uid}-reason` : undefined);
  const describedBy = $derived([reasonId, hintId].filter(Boolean).join(' ') || undefined);
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
        disabled={disabledReason !== undefined}
        aria-describedby={describedBy}
        {...rest}
      />
      <span class="box" aria-hidden="true">
        {#if isIndeterminate}
          <Icon name="minus" size="sm" />
        {:else if checked}
          <Icon name="check" size="sm" />
        {/if}
      </span>
    </span>
    <label class="label" for={uid}>{label}</label>
  </div>
  {#if disabledReason}
    <p id={reasonId} class="message">{disabledReason}</p>
  {/if}
  {#if hint}
    <p id={hintId} class="message">{hint}</p>
  {/if}
</div>

<style>
  .checkbox { display: flex; flex-direction: column; gap: var(--sp-050); }

  .row { display: flex; align-items: start; gap: var(--sp-100); }

  .control {
    position: relative;
    display: inline-flex;
    flex: none;
    /* Lines up with the first line of the label rather than with the block, so a label that wraps
       does not drag the box down with it. */
    margin-block-start: var(--sp-025);
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

  .native:disabled { cursor: not-allowed; }

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
    transition: background-color var(--dur-fast) var(--ease-standard);
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

  .native:hover:not(:disabled) + .box { border-color: var(--accent-primary); }

  .native:disabled + .box {
    background: var(--bg-surface-sunken);
    border-color: var(--border-subtle);
    color: var(--text-subtle);
  }

  .native:disabled:checked + .box,
  .native:disabled:indeterminate + .box { background: var(--border-default); border-color: var(--border-default); }

  .label {
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    cursor: pointer;
    overflow-wrap: anywhere;
  }

  /* `:has` on the row rather than a sibling combinator: the label is not a sibling of the input,
     it is a sibling of the span the input sits in. */
  .row:has(.native:disabled) .label { color: var(--text-subtle); cursor: not-allowed; }

  .message {
    max-width: 60ch;
    margin: 0;
    /* Under the label, not under the box: the text is what it explains. */
    margin-inline-start: calc(var(--sp-250) + var(--sp-100));
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
  }

  @media (prefers-reduced-motion: reduce) {
    .box { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .box { transition-duration: var(--dur-instant); }
</style>
