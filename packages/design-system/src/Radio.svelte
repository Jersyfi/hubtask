<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One of a few, all of them visible at once.
  //
  // A group and not a single button, because a lone radio is a control a person cannot switch off
  // and the browser only gives arrow-key navigation to a named group. The `name` is generated when
  // the caller does not give one, so two groups on one page cannot silently become one.

  import type { Disableable } from './control.ts';

  export interface RadioOption {
    readonly value: string;
    /** Display text: a resolved message code (ADR-0011). */
    readonly label: string;
    readonly hint?: string;
    readonly disabledReason?: string;
  }

  interface Props extends Disableable {
    /** The question the options answer. Rendered as the group's accessible name. */
    label: string;
    options: readonly RadioOption[];
    hint?: string;
    error?: string;
    /** Shared by every input in the group; generated when absent. */
    name?: string;
    value?: string;
  }

  let {
    label,
    options,
    hint,
    error,
    disabledReason,
    name = `radio-${Math.random().toString(36).slice(2, 9)}`,
    value = $bindable(''),
  }: Props = $props();

  const uid = `radio-group-${Math.random().toString(36).slice(2, 9)}`;
  const hintId = $derived(hint ? `${uid}-hint` : undefined);
  const errorId = $derived(error ? `${uid}-error` : undefined);
  const reasonId = $derived(disabledReason ? `${uid}-reason` : undefined);
  const describedBy = $derived([errorId, reasonId, hintId].filter(Boolean).join(' ') || undefined);
</script>

<!-- No `aria-invalid`: a fieldset's implicit `group` role does not support it, so the error is
     carried by `aria-describedby` and by the message below, which is where a person reads it. -->
<fieldset class="group" aria-describedby={describedBy}>
  <legend class="legend">{label}</legend>

  {#each options as option (option.value)}
    {@const optionId = `${uid}-${option.value}`}
    {@const off = disabledReason !== undefined || option.disabledReason !== undefined}
    <div class="row">
      <span class="control">
        <input
          id={optionId}
          type="radio"
          class="native"
          {name}
          value={option.value}
          bind:group={value}
          disabled={off}
          aria-describedby={option.disabledReason ? `${optionId}-reason` : undefined}
        />
        <span class="dot" aria-hidden="true"></span>
      </span>
      <label class="label" for={optionId}>{option.label}</label>
      {#if option.disabledReason}
        <p id={`${optionId}-reason`} class="message">{option.disabledReason}</p>
      {:else if option.hint}
        <p class="message">{option.hint}</p>
      {/if}
    </div>
  {/each}

  {#if error}
    <p id={errorId} class="message error">{error}</p>
  {/if}
  {#if disabledReason}
    <p id={reasonId} class="message">{disabledReason}</p>
  {/if}
  {#if hint}
    <p id={hintId} class="message">{hint}</p>
  {/if}
</fieldset>

<style>
  .group {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    margin: 0;
    padding: 0;
    border: 0;
  }

  .legend {
    padding: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    text-align: start;
  }

  .row {
    display: grid;
    grid-template-columns: auto 1fr;
    align-items: start;
    gap: var(--sp-025) var(--sp-100);
  }

  .control {
    position: relative;
    display: inline-flex;
    margin-block-start: var(--sp-025);
  }

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

  /* A picture of the control, never the control: the transparent input above it takes every
     click (the failure is worked through in Switch.svelte). */
  .dot {
    pointer-events: none;
    display: inline-block;
    width: var(--sp-250);
    height: var(--sp-250);
    border: var(--bw-thick) solid var(--border-strong);
    border-radius: var(--r-full);
    background: var(--bg-surface);
    transition: box-shadow var(--motion-state-duration) var(--motion-state-easing);
  }

  /* The filled centre is an inset ring rather than a child element, so there is nothing to
     mis-centre when the box grows with the type scale. */
  .native:checked + .dot {
    border-color: var(--accent-primary);
    box-shadow: inset 0 0 0 var(--sp-050) var(--accent-primary);
  }

  .native:focus-visible + .dot {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .native:hover:not(:disabled) + .dot { border-color: var(--accent-primary); }

  .native:disabled + .dot { background: var(--bg-surface-sunken); border-color: var(--border-subtle); }
  .native:disabled:checked + .dot { box-shadow: inset 0 0 0 var(--sp-050) var(--border-default); border-color: var(--border-default); }

  .label {
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    cursor: pointer;
    overflow-wrap: anywhere;
  }

  .row:has(.native:disabled) .label { color: var(--text-subtle); cursor: not-allowed; }

  .message {
    grid-column: 2;
    max-width: 60ch;
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
    text-align: start;
  }

  .message.error { grid-column: auto; color: var(--text-danger); }

  @media (prefers-reduced-motion: reduce) {
    .dot { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .dot { transition-duration: var(--dur-instant); }
</style>
