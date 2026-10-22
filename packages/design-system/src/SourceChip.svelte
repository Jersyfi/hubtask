<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Where a value comes from, beside its name (design-system.md §4, UC-WRK-32 check 5, P-06,
  // P-07): "Set here", "Default", "Plan 🔒", "Installation 🔒".
  //
  // **One mark for every value set above.** Settings resolve Hubtask's minimum → installation →
  // plan → workspace, and a value a person meets without learning who set it is P-07 broken. Six
  // phrasings for that across the screens became this one chip; pressed, it opens the value, its
  // source, and whom to ask.
  //
  // **A lock is a mark and a word**, never a colour alone (rule 3): the caller's label carries the
  // word ("Plan, locked"), the lock is drawn beside it.
  //
  // It knows no levels. Which source a value has and whether it is locked is the application's
  // answer; the chip takes the words.

  import Icon from './Icon.svelte';
  import Popover from './Popover.svelte';

  interface Props {
    /** The source in a word or two: "Set here", "Default", "Plan, locked". Resolved text (ADR-0011). */
    label: string;
    /** Whether the value is locked by its source: the chip carries a lock beside its word. */
    isLocked?: boolean;
    /** What the opened surface is called, for a reader who arrives in it: "Where this comes from". */
    detailsLabel: string;
    /** The value in force, as a person reads it. */
    value: string;
    /** The source in a sentence: "Set by your plan, Team." */
    source: string;
    /** Whom to ask to change it, where somebody else decides: "Ask the workspace owner." */
    whomToAsk?: string;
  }

  const { label, isLocked = false, detailsLabel, value, source, whomToAsk }: Props = $props();
</script>

<Popover label={detailsLabel} placement={{ side: 'block-end', align: 'start' }}>
  {#snippet trigger(props)}
    <button type="button" class="chip" data-locked={isLocked ? '' : undefined} {...props}>
      {#if isLocked}<Icon name="lock" size="sm" />{/if}
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
    min-height: var(--density-control-sm-min);
    padding: 0 var(--sp-100);
    border: var(--bw-hairline) solid var(--status-neutral-border);
    border-radius: var(--r-full);
    background: var(--status-neutral-surface);
    color: var(--status-neutral-text);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
    vertical-align: middle;
  }

  .chip:hover { border-color: var(--border-strong); }

  .chip:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
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
