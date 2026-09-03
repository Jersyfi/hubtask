<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A real table, which is the whole point of the component existing.
  //
  // A grid of `div`s with `display: grid` looks identical and is a different thing entirely to
  // anybody not looking at it: no row and column relationships, no "column 3 of 7" as focus moves,
  // no way to read down a column. So this is `<table>`, the headers are `<th scope="col">`, and
  // the association is the element's rather than a set of `aria-` attributes reimplementing it.
  //
  // Two things it owes beyond that. It has an **accessible name** - a `<caption>`, drawn or only
  // announced - because a table with no name is a grid a screen reader lists as "table" among
  // other tables. And it **scrolls inside its own box**: a wide table that widened the page would
  // make every other thing on it scroll sideways, which rule 4's German makes ordinary rather than
  // exotic. The scroll container is focusable so it can be reached by keyboard, which is what the
  // WCAG scrollable-region requirement asks for.

  import type { Snippet } from 'svelte';

  /** One column. `align` is `start`/`end`, never left and right. */
  export interface Column {
    readonly id: string;
    /** Resolved text (ADR-0011). */
    readonly label: string;
    readonly align?: 'start' | 'end';
    /** Announced but not drawn, for a column of controls that needs no heading on screen. */
    readonly isLabelHidden?: boolean;
  }

  interface Props {
    /** What the table is. Becomes its caption. */
    label: string;
    /** Whether the caption is drawn or only announced. */
    isLabelHidden?: boolean;
    columns: readonly Column[];
    /** The rows. Each renders its own `<td>`s, in the columns' order. */
    children: Snippet;
  }

  const { label, isLabelHidden = false, columns, children }: Props = $props();
</script>

<!-- `tabindex="0"` on the scroll container, and the first `svelte-ignore` in this package.
     
     The rule it suspends is a good one — a `div` nobody can interact with should not be in the tab
     order — and this is the documented exception to it rather than a way around it. A region that
     scrolls must be reachable by keyboard (WCAG 2.1.1), because otherwise the columns past the
     fold are unreachable to anybody who does not use a pointer; `role="region"` with an accessible
     name plus `tabindex="0"` is the technique the WAI publishes for it. The name is what keeps it
     from being an unlabelled stop.
     
     Suspended for this element and this rule only, with the reason beside it: an ignore with no
     explanation is how a codebase acquires a second one nobody can argue with. -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="scroll" tabindex="0" role="region" aria-label={label}>
  <table class="table">
    <caption class="caption" class:is-hidden={isLabelHidden}>{label}</caption>
    <thead>
      <tr>
        {#each columns as column (column.id)}
          <th scope="col" data-align={column.align ?? 'start'}>
            <span class:is-hidden={column.isLabelHidden}>{column.label}</span>
          </th>
        {/each}
      </tr>
    </thead>
    <tbody>{@render children()}</tbody>
  </table>
</div>

<style>
  /* The table scrolls inside this rather than widening the page. Rule 4 makes it ordinary: the
     German column headings are what push a table past its container first. */
  .scroll { overflow-x: auto; max-inline-size: 100%; }

  .scroll:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .table {
    inline-size: 100%;
    border-collapse: collapse;
    font-size: var(--fs-100);
    text-align: start;
  }

  .caption {
    padding-block-end: var(--sp-150);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    text-align: start;
  }

  /* Announced but not drawn. Never `display: none`, which would take the name out of the
     accessibility tree along with the text. */
  .is-hidden {
    position: absolute;
    inline-size: var(--sp-025);
    block-size: var(--sp-025);
    margin: calc(var(--sp-025) * -1);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .table :global(th),
  .table :global(td) {
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-150);
    text-align: start;
    vertical-align: top;
  }

  .table :global(th[data-align='end']),
  .table :global(td[data-align='end']) { text-align: end; }

  .table :global(th) {
    position: sticky;
    inset-block-start: 0;
    /* The header stays put while the rows scroll under it, which is what the `sticky` rank in
       tokens.json is for - and it is the token rather than a number, because five components each
       choosing their own is the failure the scale exists to prevent. */
    z-index: var(--z-sticky);
    background: var(--bg-surface);
    border-block-end: var(--bw-hairline) solid var(--border-default);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    white-space: nowrap;
  }

  .table :global(tbody tr) { border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .table :global(tbody tr:last-child) { border-block-end: 0; }
</style>
