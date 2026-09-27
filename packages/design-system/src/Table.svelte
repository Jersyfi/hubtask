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
  // **It is a surface, not a grid drawn on the page.** Everything else in this product that holds
  // content has an edge — a border, a radius, a step of depth (design-system.md §6 rule 1). A
  // table that sat flat on the canvas with its text flush against the page's own gutter read as
  // the one thing in the interface that had been forgotten, which is what the owner found. So the
  // rows live inside a frame: `bg-surface`, a hairline, `r-lg`, and a gutter of its own that the
  // first and last column sit inside rather than on.
  //
  // **The name is a heading above the frame, not a `<caption>` inside it.** A drawn caption is
  // laid out inside the table's own box and reads as its first row. The name is still the table's
  // accessible name — `aria-labelledby` where it is drawn, `aria-label` where it is not — which is
  // Primer's `Table.Title`, and the accessibility tree is the same either way.
  //
  // **A row header is not a column header.** `th` matched both, so every `<th scope="row">` a
  // caller wrote was getting the column head's small secondary type, its `nowrap`, its thick rule
  // and its `position: sticky` — a vertical header rail nobody asked for, down the first column of
  // six screens. The column rule is `thead th` now, and a row header is a cell with a little more
  // weight.
  //
  // **There is no column-width prop, and that is a decision.** The obvious one - the growing
  // columns claim the width and the rest take what their content needs - makes the browser size
  // every other column at its *minimum* content instead, and a `Badge` whose own rule is
  // `overflow-wrap: anywhere` then hyphenates down a column two characters wide. Doing it
  // properly needs the width on the cells as well as on the heading, which means either an inline
  // style the CSP refuses (ADR-0028) or a `data-` attribute every call site repeats on every
  // `<td>`. The table's own algorithm is already close enough that neither is worth it.
  //
  // **The edge fade is CSS and not a listener.** A table that scrolls sideways has to say so, or
  // its last columns are a secret - and the website ships no script at all (ADR-0030), so a
  // measured shadow would be a shadow that only the app gets.
  //
  // It was a background on the scroll box, which put it *behind* the rows - so the row under the
  // pointer erased it, a hole in the one affordance the table has. It is two sticky overlays now,
  // drawn over the rows, and what fades them in is the scroll position itself through a scroll
  // timeline: no clock, no listener, and a box with nothing past its edge has no timeline at all,
  // so the fade stays at its base `opacity: 0`. Where scroll timelines are missing the old
  // background pair is still there, behind an `@supports`.

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

  // The drawn name and the table are tied together by an id, the way `IconButton` ties a control
  // to its reason. Generated rather than taken from the caller: a table has no natural key, and
  // two of them on one screen with the same id would name the wrong one.
  const titleId = `table-${Math.random().toString(36).slice(2, 9)}`;

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

<!-- The name, above the frame. `aria-labelledby` gives the table the same accessible name a
     `<caption>` would, without the caption's layout: a drawn caption sits inside the table's own
     box and reads as its first row. -->
{#if !isLabelHidden}<p class="title" id={titleId}>{label}</p>{/if}

<div class="frame">
  <!-- `tabindex="0"` on the scroll container, and the first `svelte-ignore` in this package.

       The rule it suspends is a good one — a `div` nobody can interact with should not be in the
       tab order — and this is the documented exception to it rather than a way around it. A region
       that scrolls must be reachable by keyboard (WCAG 2.1.1), because otherwise the columns past
       the fold are unreachable to anybody who does not use a pointer; `role="region"` with an
       accessible name plus `tabindex="0"` is the technique the WAI publishes for it. The name is
       what keeps it from being an unlabelled stop.

       Suspended for this element and this rule only, with the reason beside it: an ignore with no
       explanation is how a codebase acquires a second one nobody can argue with. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="scroll" tabindex="0" role="region" aria-label={label} aria-busy={isBusy}>
    <!-- One grid cell holding three things: the table, and the two fades laid over it. The fades
         have to be *inside* the scroll box, because that is the only box a scroll timeline can be
         read from and the only one `position: sticky` can pin them to.
         
         The name is deliberately not a short common word: `apps/website` styles a `.rail` of its
         own, and its class gate fails on a plain name a design-system component also uses -
         Svelte scopes this component's rules outwards but cannot stop the site's from reaching
         in. -->
    <div class="overlay-grid">
      <table
        class="table"
        class:is-busy={isBusy}
        aria-labelledby={isLabelHidden ? undefined : titleId}
        aria-label={isLabelHidden ? label : undefined}
      >
        <thead>
          <tr>
            {#each columns as column (column.id)}
              <th scope="col" data-align={column.align ?? 'start'} aria-sort={ariaSort(column)}>
                {#if column.isSortable && onSort}
                  <!-- The control fills the cell rather than sitting in it: a heading whose target
                       is the word alone is a target the width of the word, and `Fällig` is six
                       characters. It is the same box as the plain heading beside it, so a sortable
                       column and an unsortable one are the same height - which they were not, and
                       the head of a table with one sorted column visibly grew. -->
                  <button
                    type="button"
                    class="head-label sorter"
                    class:is-active={sort.columnId === column.id && sort.direction !== 'none'}
                    onclick={() => onSort(nextSort(sort, column.id))}
                  >
                    <span class:is-hidden={column.isLabelHidden}>{column.label}</span>
                    <Icon name={mark(column)} size="sm" />
                  </button>
                {:else}
                  <span class="head-label"
                    ><span class:is-hidden={column.isLabelHidden}>{column.label}</span></span
                  >
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

      <span class="fade fade-start" aria-hidden="true"></span>
      <span class="fade fade-end" aria-hidden="true"></span>
    </div>
  </div>
</div>

<style>
  /* The name, above the frame and clearly not a row of it. */
  .title {
    margin: 0 0 var(--sp-150);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  /* The frame. §6 rule 1's raised step for a block that stands on its own: a surface, a hairline
     and a radius, which is what every other thing in this product that holds content has. The
     radius is what `overflow: hidden` is for - a row scrolled to the corner must not square it
     off. */
  .frame {
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: hidden;
    max-inline-size: 100%;
  }

  /* The table scrolls inside this rather than widening the page. Rule 4 makes it ordinary: the
     German column headings are what push a table past its container first.

     `max-block-size: 100%` is what makes the sticky head below reachable. `overflow-x: auto`
     computes `overflow-y` to `auto` as well, so this element is the scrollport in **both** axes -
     which means the head sticks to *this* box and never to the page. Without a height this box is
     exactly as tall as its table and never scrolls vertically, so the rule was inert from the day
     it was written. `100%` resolves only against a parent with a definite height: give the table's
     container one and the head sticks inside it, leave it alone and this is `none`. */
  .scroll {
    overflow-x: auto;
    max-inline-size: 100%;
    max-block-size: 100%;
  }

  .scroll:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: calc(var(--sp-025) * -1);
  }

  /* One cell, three children stacked in it. An implicit `auto` track, so the table keeps exactly
     the width behaviour it had when it was the scroll box's only child. */
  .overlay-grid { display: grid; }
  .overlay-grid > :global(*) { grid-area: 1 / 1; }

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

  /* Cells. **Middle**, not top: a name beside a badge beside a number sat on three different
     baselines, and the row read as three rows that happened to line up. */
  .table :global(th),
  .table :global(td) {
    padding-block: var(--density-row-block);
    padding-inline: var(--sp-150);
    vertical-align: middle;
  }

  /* Only the `th`, which the browser's own sheet centres. A `td` inherits `start` from the table
     above, and saying it again here is not harmless: `.table :global(td)` outranks the single
     class a call site writes on its own cell, so a view that right-aligned its last column got
     `start` and never found out. The contract is `data-align`, and now a class works too. */
  .table :global(th) { text-align: start; }

  /* The frame's own gutter. The outer columns sit inside the edge rather than on it. */
  .table :global(tr > :first-child) { padding-inline-start: var(--sp-200); }
  .table :global(tr > :last-child) { padding-inline-end: var(--sp-200); }

  .table :global(th[data-align='end']),
  .table :global(td[data-align='end']) { text-align: end; }

  /* The column head. `thead th` and not `th`: the row headers are cells and are styled below. */
  .table :global(thead th) {
    position: sticky;
    inset-block-start: 0;
    /* The header stays put while the rows scroll under it, inside the scroll box above - which is
       the only thing it can stick to. The rank is the `sticky` one from tokens.json rather than a
       number, because five components each choosing their own is the failure the scale exists to
       prevent. */
    z-index: var(--z-sticky);
    background: var(--bg-surface);
    /* Heavier than a row rule on purpose: the head is a different kind of line from the ones
       between rows, and at a glance that difference is what tells a reader where the data starts. */
    border-block-end: var(--bw-thick) solid var(--border-default);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    white-space: nowrap;
  }

  /* A row header is a cell that names its row. It keeps `scope="row"` - that is what a screen
     reader reads before each cell - and takes the body's type with a little more weight, because
     it is the name of the row and not a heading of the table. */
  .table :global(tbody th) {
    color: var(--text-primary);
    font-size: var(--fs-100);
    font-weight: var(--fw-medium);
  }

  /* One box for both kinds of heading, so a sortable column is exactly as tall as an unsortable
     one. It was the sort control that carried the minimum height, and a head with one sortable
     column stood taller than a head without. */
  .head-label {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
    min-block-size: var(--density-control-sm-min);
    inline-size: 100%;
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: inherit;
  }

  .sorter { border-radius: var(--r-xs); cursor: pointer; }

  th[data-align='end'] .head-label { justify-content: flex-end; }

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

  /* The emptiness is a row, so the headings stay. Drawn only when it is the only row left, which
     is what lets the caller render it unconditionally. */
  .empty-row:not(:first-child) { display: none; }
  .empty-row td {
    padding-block: var(--sp-400);
    text-align: center;
  }

  /* ── The edge fade ──────────────────────────────────────────────────────────

     Two overlays pinned to the ends of the scroll box, **over** the rows - which is the whole
     point of moving them here. As a background they were behind the rows, so the row under the
     pointer painted over them and the affordance had a hole in it exactly where the reader was
     looking.

     What fades them in is the scroll position, read through a scroll timeline: no clock and no
     listener, so the website has it too. A box with nothing past its edge has no scroll timeline
     at all, and the animation then does not apply - which is how the fade knows to stay away
     without anybody measuring anything. */
  .fade {
    z-index: var(--z-sticky);
    align-self: stretch;
    inline-size: var(--sp-200);
    pointer-events: none;
    opacity: 0;
    animation: table-edge linear both;
    animation-timeline: scroll(nearest inline);
  }

  @keyframes table-edge {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  .fade-start {
    position: sticky;
    inset-inline-start: 0;
    justify-self: start;
    background: linear-gradient(
      to right,
      color-mix(in oklab, var(--text-primary) 16%, transparent),
      transparent
    );
    animation-range: 0 var(--sp-400);
  }

  .fade-end {
    position: sticky;
    inset-inline-end: 0;
    justify-self: end;
    background: linear-gradient(
      to left,
      color-mix(in oklab, var(--text-primary) 16%, transparent),
      transparent
    );
    /* Reversed, so it is at full strength while there is something past the end and gone once the
       reader has reached it. */
    animation-direction: reverse;
    animation-range: calc(100% - var(--sp-400)) 100%;
  }

  /* Where scroll timelines are missing, the pair of background gradients this started as. It has
     the hole under the hovered row - but a hole in the affordance is better than no affordance,
     and every engine this product supports has been growing the timeline. */
  @supports not (animation-timeline: scroll()) {
    .fade { display: none; }

    .scroll {
      background:
        linear-gradient(to right, var(--bg-surface), transparent) 0 0 / var(--sp-400) 100% no-repeat
          local,
        linear-gradient(to left, var(--bg-surface), transparent) 100% 0 / var(--sp-400) 100%
          no-repeat local,
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
    }
  }

  /* Rule 6, and its second half: the attribute alongside the media query, because a preference
     only the operating system can set is not one this product offers (ADR-0037).

     The fade is deliberately **not** switched off here. It is driven by the reader's own scrolling
     rather than by a clock - there is no movement to reduce, any more than a scrollbar moving
     under the thumb is movement - and taking it away would remove the only sign the table gives
     that it continues. What is stilled is what actually moves on its own: the busy fade, the row
     tint and the sort mark. */
  @media (prefers-reduced-motion: reduce) {
    .table.is-busy :global(tbody),
    .table :global(tbody tr),
    .sorter:not(.is-active) :global(svg) { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .table.is-busy tbody,
  :global([data-motion='reduced']) .table tbody tr,
  :global([data-motion='reduced']) .sorter:not(.is-active) :global(svg) { transition-duration: var(--dur-instant); }
</style>
