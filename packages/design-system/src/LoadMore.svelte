<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The next page, and **no page numbers** - the API has none, so no component may imply them
  // (apps/webapp/CLAUDE.md). There is no `page`, no `total pages`, and no way for a caller to ask
  // for the fourth one.
  //
  // It is a control a person presses rather than an infinite scroll, and that is an accessibility
  // decision rather than a taste. A list that loads on scroll has **no end** for a keyboard or a
  // screen reader to reach: whatever follows it - a footer, the next region - recedes every time
  // the reader gets close. F2-03 built `loadMore` to append for the same reason this is a button:
  // what the reader already had must still be there afterwards.
  //
  // What arrived is **announced**. Pressing a button and being told nothing is the case where a
  // sighted reader sees six new rows and a screen-reader user hears silence, so the count goes
  // into a polite live region - polite, because the reader asked for this and is not waiting on it
  // the way they wait on a failure.

  import Button from './Button.svelte';

  interface Props {
    /** The name of the control. Resolved text (ADR-0011). */
    label: string;
    /** What it says while the page is on its way (§2.4: it keeps its place and changes its verb). */
    busyLabel?: string;
    /** Whether there is another page. False renders the end of the list rather than a dead button. */
    hasMore?: boolean;
    isBusy?: boolean;
    /**
     * What to say once a page has landed, already resolved with its count - "6 more entries".
     * The component announces it; forming the sentence is the caller's, because a count is a
     * plural and a plural is the message catalogue's (i18n-l10n.md §2).
     */
    arrivedLabel?: string;
    /** What the end of the list says, where a list wants to say anything at all. */
    endLabel?: string;
    onLoadMore?: () => void;
  }

  const {
    label,
    busyLabel,
    hasMore = true,
    isBusy = false,
    arrivedLabel,
    endLabel,
    onLoadMore,
  }: Props = $props();
</script>

<div class="load-more">
  {#if hasMore}
    <Button tone="secondary" {isBusy} {busyLabel} onclick={() => onLoadMore?.()}>{label}</Button>
  {:else if endLabel}
    <p class="end">{endLabel}</p>
  {/if}

  <!-- Always present, never conditionally rendered: a live region that appears at the moment it
       has something to say is a region the screen reader was not watching, and the announcement is
       lost. It is empty until there is something in it. -->
  <p class="announcement" role="status" aria-live="polite">{arrivedLabel ?? ''}</p>
</div>

<style>
  .load-more {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--sp-100);
    padding-block: var(--sp-200);
  }

  .end {
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  /* Announced and not shown: the count is already visible as six new rows, so repeating it in
     print would be noise for the reader who does not need it. `VisuallyHidden`'s technique, kept
     local because this is a live region rather than a label. */
  .announcement {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    padding: 0;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
    border: 0;
  }
</style>
