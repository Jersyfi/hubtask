<!-- SPDX-License-Identifier: Apache-2.0
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
  // **`isDoneBucket`** marks the column that means "finished": the server keeps the done column and
  // completion together through every door (domain-model.md §3.5a), so the column only announces it.
  //
  // **`isFolded`** draws the column narrow, its name and count only (the done column, *Ohne
  // Spalte*); the head is a button that unfolds it. **`refusal`** dims a column a carried card may
  // not enter and says why, in words beside the dimming (rule 3).
  //
  // A column is a **region** with an accessible name, and its cards are a list. A board of `div`s
  // is a board a screen reader reads as one long run of titles with no way to tell which column
  // any of them is in.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import { arrivals } from './motion.ts';

  interface Props {
    /** The bucket's own name. Content, not a message code. */
    name: string;
    /** How many entries the column holds in total, where the query counted them. */
    count?: number | null;
    /** The bucket's limit, or null for none. */
    wipLimit?: number | null;
    /** The limit beside the count, resolved ("WIP 2 / 3"); the bare numbers without it. */
    limitLabel?: string;
    /** Resolved sentence for a column that is over its limit. */
    overLimitLabel?: string;
    /** Whether dropping here completes an entry, and the resolved sentence that says so. */
    isDoneBucket?: boolean;
    doneBucketLabel?: string;
    /** Under a workflow, the statuses the column shows, as one resolved line. */
    statusesLabel?: string;
    /** Drawn narrow: name and count only, and `unfoldLabel` on the head that opens it. */
    isFolded?: boolean;
    unfoldLabel?: string;
    onunfold?: () => void;
    /** Why a carried card may not enter, while one is carried; the column dims and says so. */
    refusal?: string;
    /** Controls that belong to the column rather than to a card. */
    actions?: Snippet;
    /** The column's foot: capture in place ("+ entry", C-4). */
    foot?: Snippet;
    children: Snippet;
  }

  const {
    name,
    count,
    wipLimit,
    limitLabel,
    overLimitLabel,
    isDoneBucket = false,
    doneBucketLabel,
    statusesLabel,
    isFolded = false,
    unfoldLabel,
    onunfold,
    refusal,
    actions,
    foot,
    children,
  }: Props = $props();

  const isOverLimit = $derived(
    wipLimit !== null && wipLimit !== undefined && count !== null && count !== undefined && count > wipLimit,
  );
</script>

{#if isFolded}
  <section class="column" data-folded="" aria-label={name} data-refused={refusal ? '' : undefined}>
    <button type="button" class="unfold" aria-expanded="false" title={unfoldLabel} onclick={() => onunfold?.()}>
      <span class="name">{name}</span>
      {#if count !== null && count !== undefined}<span class="count">{count}</span>{/if}
      {#if isDoneBucket}<span class="done" title={doneBucketLabel}><Icon name="circle-check" size="sm" /></span>{/if}
    </button>
    {#if refusal}<p class="refusal">{refusal}</p>{/if}
    <div class="cards" hidden>{@render children()}</div>
  </section>
{:else}
<section class="column" aria-label={name} data-over={isOverLimit ? '' : undefined} data-refused={refusal ? '' : undefined}>
  <header class="head">
    <h3 class="name">{name}</h3>
    {#if count !== null && count !== undefined}
      <!-- The count and the limit together, because a limit with no count is a number nobody can
           act on. `aria-hidden` on the separator only: both numbers are read. -->
      <span class="count">
        {count}{#if wipLimit !== null && wipLimit !== undefined}<span aria-hidden="true"> · </span>{limitLabel ?? `${count} / ${wipLimit}`}{/if}
      </span>
    {/if}
    {#if isDoneBucket}
      <span class="done" title={doneBucketLabel}>
        <Icon name="circle-check" size="sm" />
      </span>
    {/if}
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </header>
  {#if statusesLabel}<p class="statuses">{statusesLabel}</p>{/if}

  {#if isOverLimit && overLimitLabel}
    <!-- Said, not enforced. The server accepts the card; the column tells the reader it is over.
         Rule 3: the mark and the sentence carry it, not the colour alone. -->
    <p class="notice">
      <span class="mark" aria-hidden="true"><Icon name="triangle-alert" size="sm" /></span>
      {overLimitLabel}
    </p>
  {/if}
  {#if refusal}<p class="refusal" role="status">{refusal}</p>{/if}

  <div class="cards" {@attach arrivals}>{@render children()}</div>
  {#if foot}<div class="foot">{@render foot()}</div>{/if}
</section>
{/if}

<style>
  .column {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    /* A column is as wide as a card wants to be and no wider; the board scrolls, not the column. */
    box-sizing: border-box;
    inline-size: var(--layout-board-column);
    flex: none;
    padding: var(--sp-150);
    border-radius: var(--r-lg);
    /* Rule 1: recessed, because a column is a child region rather than a standalone element. */
    background: var(--bg-recess);
    box-shadow: inset 0 0 0 var(--bw-hairline) var(--border-subtle);
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

  /* Over its limit is a status: the count on the warning surface, the word beside it (rule 3). */
  .column[data-over] .count { padding-inline: var(--sp-050); border-radius: var(--r-sm); background: var(--status-warning-surface); color: var(--status-warning-text); font-weight: var(--fw-medium); }

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

  .statuses { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .refusal { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  /* A column a carried card may not enter: dimmed, and its reason in words above (rule 3). */
  .column[data-refused] { opacity: 0.5; }

  .column[data-folded] { inline-size: auto; padding: var(--sp-100); }

  .unfold {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-050);
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
    writing-mode: vertical-rl;
  }

  .unfold:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .foot { display: flex; }

  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    min-block-size: var(--sp-600);
    overflow-y: auto;
  }

  /* `display: flex` above would otherwise show a folded column's cards and its add button. */
  .cards[hidden] { display: none; }
</style>
