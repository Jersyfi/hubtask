<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One column of a board.
  //
  // Two of a bucket's fields are behaviour rather than decoration, and §4 names both.
  //
  // **`wipLimit`** is a limit the *board* shows. The server does not refuse a card that takes a
  // column past it (`domain-model.md` §3.5 makes it a property of the bucket, not a constraint on
  // the write), so this must not pretend it did: the column says it is over, and the reader
  // decides. A client that blocked the drop would be inventing a rule the workspace does not have.
  //
  // **`isDoneBucket`** marks the column that means "finished". The server completes nothing on its
  // account — `Bucket.IsDoneBucket` says so in the domain: "stored and reported; what reacts to it
  // is the client that renders the board". So this column *announces* what dropping here means and
  // the board is what acts on it; a component that acted would be a component with a write in it.
  //
  // A column is a **region** with an accessible name, and its cards are a list. A board of `div`s
  // is a board a screen reader reads as one long run of titles with no way to tell which column
  // any of them is in.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';

  interface Props {
    /** The bucket's own name. Content, not a message code. */
    name: string;
    /** How many entries the column holds in total, where the query counted them. */
    count?: number | null;
    /** The bucket's limit, or null for none. */
    wipLimit?: number | null;
    /** Resolved sentence for a column that is over its limit. */
    overLimitLabel?: string;
    /** Whether dropping here completes an entry, and the resolved sentence that says so. */
    isDoneBucket?: boolean;
    doneBucketLabel?: string;
    /** Controls that belong to the column rather than to a card. */
    actions?: Snippet;
    children: Snippet;
  }

  const {
    name,
    count,
    wipLimit,
    overLimitLabel,
    isDoneBucket = false,
    doneBucketLabel,
    actions,
    children,
  }: Props = $props();

  const isOverLimit = $derived(
    wipLimit !== null && wipLimit !== undefined && count !== null && count !== undefined && count > wipLimit,
  );
</script>

<section class="column" aria-label={name} data-over={isOverLimit ? '' : undefined}>
  <header class="head">
    <h3 class="name">{name}</h3>
    {#if count !== null && count !== undefined}
      <!-- The count and the limit together, because a limit with no count is a number nobody can
           act on. `aria-hidden` on the separator only: both numbers are read. -->
      <span class="count">
        {count}{#if wipLimit !== null && wipLimit !== undefined}<span aria-hidden="true"> / </span>{wipLimit}{/if}
      </span>
    {/if}
    {#if isDoneBucket}
      <span class="done" title={doneBucketLabel}>
        <Icon name="circle-check" size="sm" />
      </span>
    {/if}
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </header>

  {#if isOverLimit && overLimitLabel}
    <!-- Said, not enforced. The server accepts the card; the column tells the reader it is over.
         Rule 3: the mark and the sentence carry it, not the colour alone. -->
    <p class="notice">
      <span class="mark" aria-hidden="true"><Icon name="triangle-alert" size="sm" /></span>
      {overLimitLabel}
    </p>
  {/if}

  <div class="cards">{@render children()}</div>
</section>

<style>
  .column {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    /* A column is as wide as a card wants to be and no wider; the board scrolls, not the column. */
    inline-size: calc(var(--sp-1000) * 4);
    flex: none;
    padding: var(--sp-150);
    border-radius: var(--r-lg);
    /* Rule 1: recessed, because a column is a child region rather than a standalone element. */
    background: var(--bg-surface-sunken);
    min-block-size: 0;
  }

  .head { display: flex; align-items: center; gap: var(--sp-100); }

  .name {
    margin: 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--fs-100);
    font-weight: var(--fw-medium);
  }

  .count { flex: none; color: var(--text-subtle); font-size: var(--fs-075); }

  .column[data-over] .count { color: var(--text-warning); font-weight: var(--fw-medium); }

  .done { display: inline-flex; flex: none; color: var(--text-success); }

  .actions { margin-inline-start: auto; display: flex; gap: var(--sp-050); }

  .notice {
    display: flex;
    align-items: start;
    gap: var(--sp-050);
    margin: 0;
    color: var(--text-warning);
    font-size: var(--fs-075);
  }

  .mark { display: inline-flex; flex: none; }

  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-block-size: var(--sp-600);
    overflow-y: auto;
  }
</style>
