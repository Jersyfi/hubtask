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
  //
  // ## What the second pass added, and what it refused
  //
  // **Sorting is the column's question, announced as one.** A sortable heading is a `<button>`
  // filling the cell and the `<th>` carries `aria-sort` - which is the part Atlassian's dynamic
  // table leaves out, along with `scope` and the caption. The direction cycles through *none*, so
  // the order the caller handed over is reachable again; `table.ts` holds that arithmetic and
  // `table.test.js` checks it.
  //
  // **The component sorts nothing.** It reports which column was pressed and draws what it is
  // given. That is the seam that lets a list the client holds in full sort in the client and a
  // cursor-paged list sort on the server, without the component having an opinion about which it
  // is looking at - the same split `reorder.ts` makes for a rank.
  //
  // **An empty list keeps its headings.** Replacing the table with an `EmptyState` takes the
  // column names off the screen and moves everything under it; the emptiness is a row now, and
  // the reader can still see what the table would have held.
  //
  // **The edge fade is CSS and not a listener.** A table that scrolls sideways has to say so, or
  // its last columns are a secret - and the website ships no script at all (ADR-0030), so a
  // measured shadow would be a shadow that only the app gets. Two pairs of gradients, one pinned
  // to the box and one travelling with the content, and the covering pair hides the shadow at
  // whichever end the reader has reached.

  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import { nextSort, type Sort } from './table.ts';

  /** One column. `align` is `start`/`end`, never left and right. */
  export interface Column {
    readonly id: string;
    /** Resolved text (ADR-0011). */
    readonly label: string;
    readonly align?: 'start' | 'end';
    /** Announced but not drawn, for a column of controls that needs no heading on screen. */
    readonly isLabelHidden?: boolean;
    /**
     * Whether the reader may sort by this column. The component reports the press and sorts
     * nothing - see the note above.
     */
    readonly isSortable?: boolean;
    /**
     * `auto` takes only the width the content needs, which is what a date, a count or a column of
     * controls wants; `grow` shares out the rest. The default is `grow`, because a column that
     * had to declare itself would be a column every call site declares.
     */
    readonly width?: 'auto' | 'grow';
  }

  interface Props {
    /** What the table is. Becomes its caption. */
    label: string;
    /** Whether the caption is drawn or only announced. */
    isLabelHidden?: boolean;
    columns: readonly Column[];
    /** Which column is sorted and which way. Absent means the caller's own order. */
    sort?: Sort;
    /** Called with the sort a press asks for. Without it, no heading is a control. */
    onSort?: (sort: Sort) => void;
    /** A page is on its way. The rows fade and stay put rather than being replaced by a spinner. */
    isBusy?: boolean;
    /** What an empty list says, drawn as a row so the headings stay on screen. */
    empty?: Snippet;
    /** The rows. Each renders its own `<td>`s, in the columns' order. */
    children: Snippet;
  }

  const {
    label,
    isLabelHidden = false,
    columns,
    sort = { direction: 'none' },
    onSort,
    isBusy = false,
    empty,
    children,
  }: Props = $props();

  /** `aria-sort` for one column: the attribute, not a word drawn anywhere. */
  function ariaSort(column: Column): 'ascending' | 'descending' | 'none' | undefined {
    if (!column.isSortable || !onSort) return undefined;
    return sort.columnId === column.id && sort.direction !== 'none' ? sort.direction : 'none';
  }

  /**
   * Which mark a sortable heading carries.
   *
   * The unsorted mark is drawn rather than hidden until hover: a column whose sortability only
   * appears under a pointer is a column a touch reader never finds. It is drawn quietly instead.
   */
  function mark(column: Column): 'chevron-up' | 'chevron-down' | 'chevrons-up-down' {
    if (sort.columnId !== column.id) return 'chevrons-up-down';
    if (sort.direction === 'ascending') return 'chevron-up';
    if (sort.direction === 'descending') return 'chevron-down';
    return 'chevrons-up-down';
  }
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
<div class="scroll" tabindex="0" role="region" aria-label={label} aria-busy={isBusy}>
  <table class="table" class:is-busy={isBusy}>
    <caption class="caption" class:is-hidden={isLabelHidden}>{label}</caption>
    <thead>
      <tr>
        {#each columns as column (column.id)}
          <th
            scope="col"
            data-align={column.align ?? 'start'}
            data-width={column.width ?? 'grow'}
            aria-sort={ariaSort(column)}
          >
            {#if column.isSortable && onSort}
              <!-- The control fills the cell rather than sitting in it: a heading whose target is
                   the word alone is a target the width of the word, and `Fällig` is six
                   characters. -->
              <button
                type="button"
                class="sorter"
                class:is-active={sort.columnId === column.id && sort.direction !== 'none'}
                onclick={() => onSort(nextSort(sort, column.id))}
              >
                <span class:is-hidden={column.isLabelHidden}>{column.label}</span>
                <Icon name={mark(column)} size="sm" />
              </button>
            {:else}
              <span class:is-hidden={column.isLabelHidden}>{column.label}</span>
            {/if}
          </th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {@render children()}
      {#if empty}
        <!-- Drawn by the stylesheet only when it is the sole row, so the caller renders it
             unconditionally and no call site has to count its own rows to decide. -->
        <tr class="empty-row">
          <td colspan={columns.length}>{@render empty()}</td>
        </tr>
      {/if}
    </tbody>
  </table>
</div>

<style>
  /* The table scrolls inside this rather than widening the page. Rule 4 makes it ordinary: the
     German column headings are what push a table past its container first.

     The four background layers are the edge fade. The first pair travels with the content
     (`local`) and is the surface colour, so at either end it covers the shadow behind it; the
     second pair is pinned to the box (`scroll`) and is the shadow itself. Nothing measures
     anything and nothing listens, which is what lets the website have it too. The colours are
     neutrals, so fading one to `transparent` has no hue to fringe. */
  .scroll {
    overflow-x: auto;
    max-inline-size: 100%;
    background:
      linear-gradient(to right, var(--bg-surface), transparent) 0 0 / var(--sp-400) 100% no-repeat
        local,
      linear-gradient(to left, var(--bg-surface), transparent) 100% 0 / var(--sp-400) 100% no-repeat
        local,
      radial-gradient(
          farthest-side at 0 50%,
          color-mix(in oklab, var(--text-primary) 18%, transparent),
          transparent
        )
        0 0 / var(--sp-150) 100% no-repeat scroll,
      radial-gradient(
          farthest-side at 100% 50%,
          color-mix(in oklab, var(--text-primary) 18%, transparent),
          transparent
        )
        100% 0 / var(--sp-150) 100% no-repeat scroll;
    background-color: var(--bg-surface);
  }

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

  /* A page on its way. The rows stay where they are and fade, which keeps the reader's place and
     the page's height - a spinner in place of the body moves everything under it twice. */
  .table.is-busy :global(tbody) {
    opacity: 0.4;
    transition: opacity var(--motion-state-duration) var(--motion-state-easing);
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

  /* Flush with the container at both ends, so the first column's text sits on the same line as
     the heading above the table rather than a gutter inside it. The inner gutters stay: they are
     what separates two columns, and the outer one separated the table from nothing. */
  .table :global(tr > :first-child) { padding-inline-start: 0; }
  .table :global(tr > :last-child) { padding-inline-end: 0; }

  .table :global(th[data-align='end']),
  .table :global(td[data-align='end']) { text-align: end; }

  /* A column that takes only what it needs. `1%` with `nowrap` is the table layout algorithm's
     way of saying "as narrow as the content allows"; the grow columns share out the rest. */
  .table :global(th[data-width='auto']) { inline-size: 1%; white-space: nowrap; }

  .table :global(th) {
    position: sticky;
    inset-block-start: 0;
    /* The header stays put while the rows scroll under it, which is what the `sticky` rank in
       tokens.json is for - and it is the token rather than a number, because five components each
       choosing their own is the failure the scale exists to prevent. */
    z-index: var(--z-sticky);
    background: var(--bg-surface);
    /* Heavier than a row rule on purpose: the head is a different kind of line from the ones
       between rows, and at a glance that difference is what tells a reader where the data starts.
       Atlassian's table draws the same distinction and it is the reason its head reads as a head
       rather than as the first row. */
    border-block-end: var(--bw-thick) solid var(--border-default);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    white-space: nowrap;
  }

  /* The sortable heading. It fills the cell, keeps the head's own type, and carries its mark at
     the end of the word - after it in reading order, so `dir="rtl"` moves both together. */
  .sorter {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    inline-size: 100%;
    min-block-size: var(--density-control-sm-min);
    margin: 0;
    padding: 0;
    border: 0;
    border-radius: var(--r-xs);
    background: none;
    color: inherit;
    font: inherit;
    text-align: inherit;
    cursor: pointer;
  }

  th[data-align='end'] .sorter { justify-content: flex-end; }

  /* The unsorted mark is quiet rather than absent: a column whose sortability only appears under
     a pointer is one a touch reader never finds. */
  .sorter:not(.is-active) :global(svg) {
    color: var(--text-subtle);
    opacity: 0.6;
    transition: opacity var(--motion-state-duration) var(--motion-state-easing);
  }

  .sorter:hover { color: var(--text-primary); }
  .sorter:hover:not(.is-active) :global(svg) { opacity: 1; }
  .sorter.is-active { color: var(--text-primary); }
  .sorter.is-active :global(svg) { color: var(--accent-primary); }

  .sorter:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .table :global(tbody tr) {
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  /* The rule goes *between* rows rather than under each of them. Under-each needs the last one
     cancelled, and "the last one" stops being the last one the moment the empty row is in the
     markup but not on the screen - which is exactly the case this table has. */
  .table :global(tbody tr + tr) {
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  /* Hover on every row, not only on interactive ones. It is a reading aid before it is an
     affordance: a wide row is read across, and the band under the eye is what keeps the last
     column on the same line as the first. */
  .table :global(tbody tr:hover) { background: var(--bg-surface-hover); }

  /* A row the caller has marked. `data-selected` travels on the `<tr>` at the call site, and the
     rule is the colour *and* the bar - a row told apart by tint alone is a row half the readers
     cannot tell apart (rule 3). */
  .table :global(tbody tr[data-selected]) {
    background: var(--accent-primary-subtle);
    box-shadow: inset var(--sp-050) 0 0 0 var(--accent-primary);
  }

  /* Rule 6, and its second half: the attribute alongside the media query, because a preference
     only the operating system can set is not one this product offers (ADR-0037). The fade of a
     busy body is the one that matters here - it is the only movement on the screen while a page
     is on its way, and a reader who asked for stillness gets the new opacity at once. */
  @media (prefers-reduced-motion: reduce) {
    .table.is-busy :global(tbody),
    .table :global(tbody tr),
    .sorter:not(.is-active) :global(svg) { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .table.is-busy tbody,
  :global([data-motion='reduced']) .table tbody tr,
  :global([data-motion='reduced']) .sorter:not(.is-active) :global(svg) { transition-duration: var(--dur-instant); }

  /* The emptiness is a row, so the headings stay. Drawn only when it is the only row left, which
     is what lets the caller render it unconditionally. */
  .empty-row:not(:first-child) { display: none; }
  .empty-row td {
    padding-block: var(--sp-400);
    text-align: center;
  }
</style>
