<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where a value comes from, at the end of its name's line (design-system.md §4, UC-WRK-32
  // check 5, P-06, P-07): "Default", "• Set here", "🔒 Locked by the installation".
  //
  // **One mark for every value set above.** Settings resolve Hubtask's minimum → installation →
  // plan → workspace, and a value a person meets without learning who set it is P-07 broken. Six
  // phrasings for that across the screens became this one chip; pressed, it opens the value, its
  // source, and whom to ask.
  //
  // **Quiet metadata of the row**, not a control that competes with the value: no frame, no fill,
  // the row's small subtle words (the owner's review of the settings rows). Still a button with
  // its focus ring and a target of the small control's height (WCAG 2.5.8).
  //
  // **A lock is a mark and a word**, never a colour alone (rule 3): the caller's label carries the
  // word ("Locked by the installation"), the lock is drawn before it; a value set here carries the
  // accent dot before its word, beside the row's *Reset*.
  //
  // It knows no levels. Which source a value has and whether it is locked is the application's
  // answer; the chip takes the words.

  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';

  interface Props {
    /** The source in a word or two: "Set here", "Default", "Locked by your plan". Resolved text (ADR-0011). */
    label: string;
    /** Whether the value is locked by its source: the chip carries a lock before its word. */
    isLocked?: boolean;
    /** Whether the value is set at the level the reader is on: the chip carries the accent dot. */
    isSetHere?: boolean;
    /** What the opened surface is called, for a reader who arrives in it: "Where this comes from". */
    detailsLabel: string;
    /** The value in force, as a person reads it. */
    value: string;
    /** The source in a sentence: "Set by your plan, Team." */
    source: string;
    /** Whom to ask to change it, where somebody else decides: "Ask the workspace owner." */
    whomToAsk?: string;
  }

  const { label, isLocked = false, isSetHere = false, detailsLabel, value, source, whomToAsk }: Props = $props();
</script>

<Popover label={detailsLabel} placement={{ side: 'block-end', align: 'start' }}>
  {#snippet trigger(props)}
    <button type="button" class="chip" data-locked={isLocked ? '' : undefined} data-set-here={isSetHere && !isLocked ? '' : undefined} {...props}>
      {#if isLocked}<Icon name="lock" size="sm" />{:else if isSetHere}<span class="dot" aria-hidden="true"></span>{/if}
      <span>{label}</span>
    </button>
  {/snippet}
  <dl class="details">
    <dt class="term">{detailsLabel}</dt>
    <dd class="value">{value}</dd>
    <dd class="line">{source}</dd>
    {#if whomToAsk}<dd class="line">{whomToAsk}</dd>{/if}
  </dl>
</Popover>

<style>
  .chip {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-height: var(--sp-300);
    padding: 0 var(--sp-025);
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-subtle);
    font: inherit;
    font-size: var(--fs-075);
    font-weight: var(--fw-regular);
    cursor: pointer;
    vertical-align: middle;
  }

  .chip:hover { color: var(--text-secondary); text-decoration: underline; text-underline-offset: var(--sp-025); }

  .chip:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .dot {
    flex: none;
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--accent-primary);
  }

  .details {
    display: flex;
    flex-direction: column;
    gap: var(--sp-050);
    margin: 0;
    max-width: 40ch;
  }

  .term {
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .value {
    margin: 0;
    color: var(--text-primary);
    font-weight: var(--fw-medium);
  }

  .line {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
</style>
