<!-- SPDX-License-Identifier: Apache-2.0
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

  import Reason from './_Reason.svelte';
  import Spinner from './Spinner.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import type { Busyable, Disableable } from './control.ts';
  import { guard, sharedReason } from './reason.ts';

  interface Props extends Omit<HTMLInputAttributes, 'disabled' | 'type' | 'checked'>, Disableable, Busyable {
    label: string;
    /** What it is doing while busy, said to a reader: the present participle (voice-and-tone.md §2.4). */
    busyLabel?: string;
    hint?: string;
    checked?: boolean;
  }

  let { label, hint, disabledReason, isBusy = false, busyLabel, checked = $bindable(false), ...rest }: Props = $props();

  // Svelte's own id, not a random one: unique per instance and the same on every render, so a
  // prerendered page is identical between builds and a hydrated one matches its markup.
  const instanceId = $props.id();
  const uid = `switch-${instanceId}`;
  const hintId = $derived(hint ? `${uid}-hint` : undefined);

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
  const describedBy = $derived([reasonId, busyId, hintId].filter(Boolean).join(' ') || undefined);
  let native = $state<HTMLInputElement | null>(null);
  let labelElement = $state<HTMLLabelElement | null>(null);

  $effect(() => {
    if (!isBusy || unavailable) return;
    return guard([native, labelElement], () => {});
  });
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
        aria-disabled={unavailable ? 'true' : undefined}
        aria-busy={isBusy ? 'true' : undefined}
        aria-describedby={describedBy}
        bind:this={native}
        {...rest}
      />
      <span class="track" aria-hidden="true">
        <span class="knob">{#if isBusy}<Spinner size="sm" />{/if}</span>
      </span>
      {#if busyId}
        <!-- Inside the box's own span, which is positioned: a hidden sentence there is no flex
             item of the column and adds no gap. -->
        <span id={busyId}><VisuallyHidden>{busyLabel}</VisuallyHidden></span>
      {/if}
    </span>
    <label class="label" for={uid} bind:this={labelElement}>{label}</label>
  </div>
  {#if disabledReason !== undefined && !groupReasonId}
    <Reason id={ownReasonId} text={disabledReason} control={native} forwarders={[labelElement]} />
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

  .native[aria-disabled='true'] { cursor: not-allowed; }

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
    display: inline-flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
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
     asked for by name - and asked of the `dir` attribute, not of `:dir(rtl)`: Chromium does not
     match `:dir()` on an element inserted after its ancestor's `dir` was set, which is every
     element the workbench renders into a pane and every element the app renders after the
     manifest turns the document round. */
  :global([dir='rtl']) .native:checked + .track .knob {
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

  .native:hover:not([aria-disabled='true']) + .track { border-color: var(--accent-primary); }

  .native[aria-disabled='true'] + .track { background: var(--bg-surface-sunken); border-color: var(--border-subtle); }
  .native[aria-disabled='true'] + .track .knob { background: var(--border-default); }
  .native[aria-disabled='true']:checked + .track { background: var(--border-default); border-color: var(--border-default); }

  /* A switch that is on and cannot be changed still has to read as *on*. With the disabled knob
     grey on the disabled track's grey it disappeared, and the control read as off - which is the
     opposite of what it says. The position is what carries the state (rule 3: never colour
     alone), so the knob keeps a surface the track cannot swallow. */
  .native[aria-disabled='true']:checked + .track .knob { background: var(--bg-surface); }

  .label {
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    cursor: pointer;
    overflow-wrap: anywhere;
  }

  .row:has(.native[aria-disabled='true']) .label { color: var(--text-subtle); cursor: not-allowed; }

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
