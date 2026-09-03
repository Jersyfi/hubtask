<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A setting that takes effect at once.
  //
  // Not a checkbox with a different skin. A checkbox is a value in a form that is submitted; a
  // switch is a setting that applies the moment it moves, and that difference is the whole reason
  // both exist. It is `role="switch"` on a real checkbox input, so the keyboard and the
  // accessibility tree are the platform's and only the announcement changes.
  //
  // Rule 3: colour never stands alone. The knob moves, which is the state a person reads without
  // seeing a colour at all.

  import type { HTMLInputAttributes } from 'svelte/elements';

  import type { Disableable } from './control.ts';

  interface Props extends Omit<HTMLInputAttributes, 'disabled' | 'type' | 'checked'>, Disableable {
    label: string;
    hint?: string;
    checked?: boolean;
  }

  let { label, hint, disabledReason, checked = $bindable(false), ...rest }: Props = $props();

  const uid = `switch-${Math.random().toString(36).slice(2, 9)}`;
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const reasonId = $derived(disabledReason ? `${uid}-reason` : undefined);
  const describedBy = $derived([reasonId, hintId].filter(Boolean).join(' ') || undefined);
</script>

<div class="switch">
  <div class="row">
    <span class="control">
      <input
        id={uid}
        type="checkbox"
        role="switch"
        class="native"
        bind:checked
        disabled={disabledReason !== undefined}
        aria-describedby={describedBy}
        {...rest}
      />
      <span class="track" aria-hidden="true"><span class="knob"></span></span>
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
  .switch { display: flex; flex-direction: column; gap: var(--sp-050); }

  .row { display: flex; align-items: center; gap: var(--sp-150); }

  .control { position: relative; display: inline-flex; flex: none; }

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

  .track {
    /* How far the knob travels: the track's width less the knob's box. Named here so the RTL rule
       below can negate it rather than repeat the arithmetic. */
    --travel: calc(var(--sp-500) - var(--sp-250));
    /* The input above is the control; this is a picture of it. Without this the knob swallowed the
       click that was meant to switch it off - a translated element creates a stacking context, so
       the moved knob painted above the transparent input and took the pointer with it. The switch
       could then only be turned off by clicking the track beside its own knob. */
    pointer-events: none;
    display: inline-flex;
    align-items: center;
    width: var(--sp-500);
    height: var(--sp-250);
    padding: var(--sp-025);
    border: var(--bw-hairline) solid var(--border-strong);
    border-radius: var(--r-full);
    background: var(--bg-surface-sunken);
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .knob {
    width: var(--sp-150);
    height: var(--sp-150);
    border-radius: var(--r-full);
    background: var(--border-strong);
    /* Rule 6: transform only. The knob moves; nothing is laid out again. */
    transition: transform var(--motion-state-duration) var(--motion-state-easing);
  }

  .native:checked + .track .knob {
    translate: var(--travel) 0;
    background: var(--text-inverse);
  }

  /* `translate` is physical: its X is the screen's X and not the writing direction's. The knob has
     to travel towards the *end* of the line, so in RTL the same distance is negated. This is the
     one place in the wave where a logical property does not exist and the direction has to be
     asked for by name. */
  .native:checked + .track:dir(rtl) .knob {
    translate: calc(-1 * var(--travel)) 0;
  }

  .native:checked + .track {
    background: var(--accent-primary);
    border-color: var(--accent-primary);
  }

  .native:focus-visible + .track {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .native:hover:not(:disabled) + .track { border-color: var(--accent-primary); }

  .native:disabled + .track { background: var(--bg-surface-sunken); border-color: var(--border-subtle); }
  .native:disabled + .track .knob { background: var(--border-default); }
  .native:disabled:checked + .track { background: var(--border-default); border-color: var(--border-default); }

  .label {
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    cursor: pointer;
    overflow-wrap: anywhere;
  }

  .row:has(.native:disabled) .label { color: var(--text-subtle); cursor: not-allowed; }

  .message {
    max-width: 60ch;
    margin: 0;
    margin-inline-start: calc(var(--sp-500) + var(--sp-150));
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
  }

  @media (prefers-reduced-motion: reduce) {
    .track, .knob { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .track,
  :global([data-motion='reduced']) .knob { transition-duration: var(--dur-instant); }
</style>
