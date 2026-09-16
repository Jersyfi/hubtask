<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A proposal the server made, rendered once for every kind of proposal there is.
  //
  // `design-system.md` §4 asks one thing of this component: that AI be *visually separable*, so
  // that switching it off removes it "without residue". The separation is the `ai.*` tokens - a
  // surface at the neutrals' luminance that differs by hue, and the border that carries the
  // boundary because rule 3 says a hue never stands alone - and this component is their only
  // consumer. Nothing glows: a proposal arrives beside what it is about, which is the `attach`
  // motion role, and it arrives the way a popover does, in opacity alone.
  //
  // What it does not know is as deliberate as what it does. It renders no payload - a title, a
  // tree of work, a set of labels and a summary are four shapes, and the product already has an
  // editor for each; the caller renders it into the slot. It wires no button - accepting is a use
  // case the application performs, and a component that performed one would be a component with a
  // server in it. It resolves no code - the heading, the provenance and the notes arrive as text
  // (ADR-0011), phrased by `voice-and-tone.md` §7: offered, never asserted; the model named where
  // the reader can find it and nowhere the eye lands first; accepted in one gesture and dismissed
  // in one; never counting, nudging or celebrating.
  //
  // Three states, because a screen gets two of them wrong. `pending` is the job still running,
  // and a strip that showed nothing until it finished would be a strip that appears from nowhere.
  // `stale` is the target having moved since the proposal was made - `suggestion.stale` is the
  // server's refusal, and the caller sets it here so that the strip says so before the server has
  // to; a stale proposal offers the ask and the dismissal, never the apply, and that is the
  // caller's to wire from the same fact.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import Spinner from './Spinner.svelte';

  /** Where the proposal stands. `open` is the one that can be accepted. */
  export type SuggestionState = 'open' | 'pending' | 'stale';

  interface Props {
    /**
     * The kind, as a heading that says it is a proposal (§7.1): "Suggested title", never
     * "Title". Resolved text - the caller turns the kind into words.
     */
    heading: string;
    state?: SuggestionState;
    /** What a pending one says while the job runs (§7.4, §2.4): "Suggesting…". */
    pendingLabel?: string;
    /** What a stale one says in the heading's line (§7.3): "This entry changed since — ask again". */
    staleNote?: string;
    /**
     * The provenance, one line, resolved: the model, when, the prompt version. Collapsed by
     * default behind `provenanceLabel` (§7.2). Absent while pending - there is nothing to name yet.
     */
    provenance?: string;
    /** The control that opens the provenance: "Where this came from". */
    provenanceLabel?: string;
    /** The payload, rendered by the caller with the editor the product has for its shape. */
    children?: Snippet;
    /** Accept and dismiss, as `Button`s the caller wires (§7.3). */
    actions?: Snippet;
  }

  const {
    heading,
    state = 'open',
    pendingLabel,
    staleNote,
    provenance,
    provenanceLabel,
    children,
    actions,
  }: Props = $props();

  const headingId = `proposal-${Math.random().toString(36).slice(2, 9)}`;
</script>

<!-- A `section` with an accessible name is a region: a landmark a screen reader can jump to and
     leave, named by its own heading - a proposal is something a reader chooses to look at. -->
<section class="proposal" data-ai data-state={state} aria-labelledby={headingId} aria-busy={state === 'pending' ? 'true' : undefined}>
  <div class="head">
    <span class="mark" aria-hidden="true"><Icon name="sparkles" size="sm" /></span>
    <p class="heading" id={headingId}>{heading}</p>
    {#if state === 'pending'}
      <!-- The spinner is decoration here: the region is already `aria-busy`, and the label is the
           one announcement. -->
      <span class="pending"><Spinner size="sm" />{pendingLabel ?? '…'}</span>
    {/if}
  </div>

  {#if state === 'stale' && staleNote}
    <p class="stale">
      <span class="stale-mark" aria-hidden="true"><Icon name="circle-alert" size="sm" /></span>
      {staleNote}
    </p>
  {/if}

  {#if children && state !== 'pending'}
    <div class="payload">{@render children()}</div>
  {/if}

  {#if provenance && state !== 'pending'}
    <!-- Native disclosure: collapsed by default, opened by the keyboard, and it needs no script
         to be one. The summary is a control, so it draws rule 5's ring. -->
    <details class="provenance">
      <summary>{provenanceLabel ?? provenance}</summary>
      <p class="origin">{provenance}</p>
    </details>
  {/if}

  {#if actions}
    <div class="actions">{@render actions()}</div>
  {/if}
</section>

<style>
  .proposal {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-150) var(--sp-200);
    border: var(--bw-hairline) solid var(--ai-border);
    border-inline-start-width: var(--bw-thick);
    border-radius: var(--r-md);
    background: var(--ai-surface);
    color: var(--text-primary);
    font-size: var(--fs-100);
    text-align: start;
    /* Rule 1: a proposal is a child of the entry it sits in - recessed, not raised. No shadow. */
    animation: arrive var(--motion-attach-duration) var(--motion-attach-easing) both;
  }

  .head { display: flex; align-items: center; gap: var(--sp-100); flex-wrap: wrap; min-width: 0; }
  .mark { display: inline-flex; color: var(--ai-text); }
  .heading {
    margin: 0;
    color: var(--ai-text);
    font-weight: var(--fw-medium);
    overflow-wrap: anywhere;
  }
  .pending {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    margin-inline-start: auto;
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  /* Rule 3: the stale state is a sentence with a mark, not a colour. The surface goes stronger
     so that a stale strip is still told apart from an open one when the text is skimmed. */
  .proposal[data-state='stale'] { background: var(--ai-surface-strong); }
  .stale { display: flex; align-items: start; gap: var(--sp-050); margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .stale-mark { display: inline-flex; margin-block-start: var(--sp-025); color: var(--text-warning); }

  .payload { min-width: 0; overflow-wrap: anywhere; }

  .provenance { font-size: var(--fs-075); color: var(--text-secondary); }
  .provenance summary {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    cursor: pointer;
    border-radius: var(--r-sm);
    color: var(--text-subtle);
    list-style: none;
  }
  .provenance summary::-webkit-details-marker { display: none; }
  .provenance summary::before {
    content: '';
    display: inline-block;
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-inline-end: var(--bw-thick) solid currentColor;
    border-block-end: var(--bw-thick) solid currentColor;
    transform: rotate(-45deg);
    transition: transform var(--motion-state-duration) var(--motion-state-easing);
  }
  .provenance[open] summary::before { transform: rotate(45deg); }
  .provenance summary:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }
  .origin { margin: var(--sp-050) 0 0; overflow-wrap: anywhere; font-family: var(--font-mono); font-size: var(--fs-075); }

  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  @keyframes arrive {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .proposal { animation: none; }
    .provenance summary::before { transition: none; }
  }

  :global([data-motion='reduced']) .proposal { animation: none; }
  :global([data-motion='reduced']) .provenance summary::before { transition: none; }
</style>
