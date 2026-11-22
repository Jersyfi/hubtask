<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import type { IconName } from './icons/index.ts';

  export interface ChoiceCard {
    readonly value: string;
    /** The card's title: a resolved message code (ADR-0011). */
    readonly label: string;
    /** One line of what choosing it means. */
    readonly description?: string;
    readonly icon?: IconName;
    readonly disabledReason?: string;
  }
</script>

<script lang="ts">
  // One of a few choices that each need a sentence to be understood, drawn as cards: a title, one
  // explaining line, an optional mark. `Radio` is the same question where a word per answer is
  // enough.
  //
  // Native radios of one name inside their cards' labels, so the whole card is pressed, the arrow
  // keys walk and choose as in any radio group, and a reader hears "radio, 1 of 2" in a named
  // group. The chosen card is marked by its edge, its surface and the filled dot - never by colour
  // alone. Nothing is chosen unless the caller chose it.

  import type { Disableable } from './control.ts';
  import Icon from './Icon.svelte';
  import { sharedReason } from './reason.ts';

  interface Props extends Disableable {
    /** The question the cards answer: the group's legend and accessible name. */
    label: string;
    options: readonly ChoiceCard[];
    /** Shared by every input in the group; generated when absent. */
    name?: string;
    value?: string;
  }

  const instanceId = $props.id();
  let { label, options, disabledReason, name = `choice-${instanceId}`, value = $bindable('') }: Props = $props();

  const uid = `choice-cards-${instanceId}`;
  const shared = sharedReason();
  const groupReasonId = $derived(shared(disabledReason));
  const reasonId = $derived(disabledReason ? (groupReasonId ?? `${uid}-reason`) : undefined);
</script>

<fieldset class="choices" role="radiogroup" aria-describedby={reasonId}>
  <legend class="legend">{label}</legend>

  {#each options as option (option.value)}
    {@const optionId = `${uid}-${option.value}`}
    {@const off = disabledReason !== undefined || option.disabledReason !== undefined}
    {@const described = [option.description ? `${optionId}-line` : undefined, option.disabledReason ? `${optionId}-reason` : undefined].filter(Boolean).join(' ')}
    <label class="card" data-icon={option.icon ? '' : undefined}>
      <span class="control">
        <input
          id={optionId}
          type="radio"
          class="native"
          {name}
          value={option.value}
          bind:group={value}
          disabled={off}
          aria-describedby={described || undefined}
        />
        <span class="dot" aria-hidden="true"></span>
      </span>
      {#if option.icon}<span class="mark" aria-hidden="true"><Icon name={option.icon} /></span>{/if}
      <span class="words">
        <span class="title">{option.label}</span>
        {#if option.description}<span id={`${optionId}-line`} class="line">{option.description}</span>{/if}
        {#if option.disabledReason}<span id={`${optionId}-reason`} class="reason">{option.disabledReason}</span>{/if}
      </span>
    </label>
  {/each}

  {#if disabledReason && !groupReasonId}
    <p id={reasonId} class="reason">{disabledReason}</p>
  {/if}
</fieldset>

<style>
  .choices {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-inline-size: 0;
    margin: 0;
    padding: 0;
    border: 0;
  }

  .legend {
    padding: 0;
    margin-block-end: var(--sp-100);
    color: var(--text-primary);
    font-weight: var(--fw-semibold);
    text-align: start;
  }

  /* The whole card is the label: a press anywhere on it chooses. */
  .card {
    display: grid;
    grid-template-columns: var(--sp-250) minmax(0, 1fr);
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    cursor: pointer;
    transition:
      background-color var(--motion-state-duration) var(--motion-state-easing),
      border-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .card[data-icon] { grid-template-columns: var(--sp-250) auto minmax(0, 1fr); }

  .card:hover { border-color: var(--border-default); }

  .card:has(.native:checked) {
    border-color: var(--accent-primary);
    background: var(--accent-primary-subtle);
  }

  .card:has(.native:focus-visible) {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .card:has(.native:disabled) { cursor: not-allowed; }
  .card:has(.native:disabled):hover { border-color: var(--border-subtle); }
  .card:has(.native:disabled) .title { color: var(--text-subtle); }

  /* The dot sits on the title's first line, as `Radio`'s does. */
  .control {
    position: relative;
    display: inline-flex;
    margin-block-start: calc((1lh - var(--sp-250)) / 2);
  }

  /* In the tree and focusable, drawn by the dot beside it. */
  .native {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    margin: 0;
    opacity: 0;
    cursor: inherit;
  }

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

  .native:checked + .dot {
    border-color: var(--accent-primary);
    box-shadow: inset 0 0 0 var(--sp-050) var(--accent-primary);
  }

  .native:disabled + .dot { background: var(--bg-surface-sunken); border-color: var(--border-subtle); }

  .mark {
    display: inline-flex;
    margin-block-start: calc((1lh - var(--sp-250)) / 2);
    color: var(--text-secondary);
  }

  .words { display: flex; flex-direction: column; gap: var(--sp-025); min-inline-size: 0; }

  .title { font-weight: var(--fw-semibold); overflow-wrap: anywhere; }

  .line { color: var(--text-secondary); font-size: var(--fs-090); overflow-wrap: anywhere; }

  .reason { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  @media (prefers-reduced-motion: reduce) {
    .card, .dot { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) :is(.card, .dot) { transition-duration: var(--dur-instant); }
</style>
