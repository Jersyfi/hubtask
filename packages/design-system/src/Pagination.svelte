<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Pages of a list somebody **already holds in full**.
  //
  // ## The line this component may not cross
  //
  // `apps/webapp/CLAUDE.md` says: cursor pagination, never page numbers - the API has none, so no
  // component may imply them. That rule stands and this does not break it, because of where the
  // numbers come from: `total` is a count, and a caller can only pass a count it has. There is no
  // `pageCount` prop to hand in, no way to say "there are 40 pages, fetch the fourth", and no
  // request anywhere in this file. Pressing 4 slices an array.
  //
  // So the two components divide the world and do not overlap:
  //
  // * the list arrives in pages from the server behind an opaque cursor - `LoadMore`, and there
  //   is no fourth page to ask for because the server does not know what a page number is;
  // * the list arrives whole and is too long to read at once - `Pagination`, which pages what is
  //   already in the browser.
  //
  // A caller holding a cursor cannot reach for this by accident: it has no total to give.
  //
  // ## What it says, and in which order
  //
  // **The range sentence is the component and the numbers are an addition to it.** "17-32 of 91"
  // is true at every width, on every list, and it is the answer to the question a reader actually
  // has - how much of this is there, and where am I in it. Primer leads with it; Atlassian draws
  // only the steps, so a reader of a long list is told which page they are on and never how many
  // rows that is. Below `medium` the steps drop out and the sentence and the two arrows remain,
  // because eleven targets in a phone's width are eleven targets nobody can hit.
  //
  // **The gap is a control.** Atlassian names the pages its ellipsis hides - "skipped pages from
  // 6 to 9" - and then leaves it inert, which tells a screen-reader reader about a door that does
  // not open. Ours goes to the middle of what it hides.
  //
  // **The current page is `aria-current="page"`,** not a colour. The fill is how it looks; the
  // attribute is what it is.
  //
  // **The two arrows are `aria-disabled`, not `disabled`, and they stay.** A keyboard reader who
  // tabs to the next arrow, reaches the last page and finds the control gone from under them has
  // lost their place in the tab order; `aria-disabled` announces it and keeps it. It is also why
  // they are not `IconButton`s: this package's `disabledReason` draws a visible sentence beside
  // the control, which is right for a field a form has switched off and is noise beside a pager
  // whose own numbers already say which end it is at.

  import Icon from './Icon.svelte';
  import { gapTarget, pageOf, pageSteps } from './table.ts';

  interface Props {
    /**
     * How many rows there are in total. A count, which is the whole safeguard: a caller that has
     * to fetch to know this does not have it, and is holding a cursor instead.
     */
    total: number;
    /** How many rows a page holds. */
    pageSize: number;
    /** Which page the reader is on, one-based. */
    page: number;
    /**
     * The name of the whole control, for the `<nav>`: "Pages of sessions". Resolved text
     * (ADR-0011).
     */
    label: string;
    /**
     * The range, already formed by the caller with its numbers in it - "17-32 of 91". A range is
     * a plural and a plural is the message catalogue's (i18n-l10n.md §2), so this component
     * neither counts nor phrases.
     */
    rangeLabel: string;
    /** The two arrows. */
    previousLabel: string;
    nextLabel: string;
    /** One page's name, given its number: "Page 4". */
    pageLabel: (page: number) => string;
    /** What a gap hides, given the first and last of them: "Pages 6 to 39". */
    gapLabel: (from: number, to: number) => string;
    onPage: (page: number) => void;
  }

  const {
    total,
    pageSize,
    page,
    label,
    rangeLabel,
    previousLabel,
    nextLabel,
    pageLabel,
    gapLabel,
    onPage,
  }: Props = $props();

  // `pageOf` over an array of nothing: the arithmetic is the same and the clamping comes with it,
  // so a page number past the end of a list that has just shrunk resolves here rather than at
  // every call site.
  const shape = $derived(pageOf(Array.from({ length: Math.max(0, total) }), page, pageSize));
  const steps = $derived(pageSteps(shape.page, shape.pageCount));
  const isFirst = $derived(shape.page <= 1);
  const isLast = $derived(shape.page >= shape.pageCount);
</script>

<nav class="pager" aria-label={label}>
  <!-- The sentence, drawn. It is not a live region: the rows under it changed at the same moment
       and the reader who pressed the control is looking at them. -->
  <p class="range">{rangeLabel}</p>

  {#if shape.pageCount > 1}
    <div class="steps">
      <button
        type="button"
        class="step is-arrow"
        aria-label={previousLabel}
        aria-disabled={isFirst ? 'true' : undefined}
        onclick={() => !isFirst && onPage(shape.page - 1)}
      >
        <Icon name="chevron-left" size="sm" />
      </button>

      <!-- An ordered list, because the pages are in an order and that order is the content.
           Primer's shape; it is also what lets the steps be hidden as a group below `medium`. -->
      <ol class="numbers">
        {#each steps as step (step.kind === 'page' ? `p${step.page}` : `g${step.from}`)}
          <li>
            {#if step.kind === 'page'}
              <button
                type="button"
                class="step"
                aria-current={step.page === shape.page ? 'page' : undefined}
                aria-label={pageLabel(step.page)}
                onclick={() => onPage(step.page)}
              >
                <!-- The number is drawn; the name is the label, because "4" alone is not a
                     destination anybody can hear. -->
                <span aria-hidden="true">{step.page}</span>
              </button>
            {:else}
              <button
                type="button"
                class="step is-gap"
                aria-label={gapLabel(step.from, step.to)}
                onclick={() => onPage(gapTarget(step))}
              >
                <span aria-hidden="true">…</span>
              </button>
            {/if}
          </li>
        {/each}
      </ol>

      <button
        type="button"
        class="step is-arrow"
        aria-label={nextLabel}
        aria-disabled={isLast ? 'true' : undefined}
        onclick={() => !isLast && onPage(shape.page + 1)}
      >
        <Icon name="chevron-right" size="sm" />
      </button>
    </div>
  {/if}
</nav>

<style>
  .pager {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: var(--sp-150);
    padding-block: var(--sp-200);
    /* It measures itself rather than the viewport. A table in a detail pane on a wide screen has
       a phone's room and wants a phone's pager; `PageHeader` folds on the same reasoning. */
    container-type: inline-size;
  }

  .range {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    /* The digits do not change width as the page moves, so the sentence does not twitch. */
    font-variant-numeric: tabular-nums;
    /* The sentence takes its direction from its own first strong character rather than from the
       page. A caller's "1-12 of 91" dropped into an RTL document otherwise comes out as
       "of 91 12-1" - the numbers reordered around a word the paragraph has decided runs the other
       way. `plaintext` is the one declaration that gets a Hebrew range and an English one both
       right, because it asks the string rather than the page. */
    unicode-bidi: plaintext;
  }

  .steps {
    display: flex;
    align-items: center;
    gap: var(--sp-050);
  }

  .numbers {
    display: flex;
    align-items: center;
    gap: var(--sp-025);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* In a narrow pager the numbers go and the sentence and the arrows stay. Seven targets across a
     phone is seven targets nobody can hit, and the range is the part that was answering the
     reader's question anyway. A container query on the pager's own width, not a media query, for
     the reason `TaskRow` gives: what has the room or lacks it is the control, not the window. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a container query cannot read a custom property. */
  @container (inline-size < 600px) {
    .numbers { display: none; }
  }

  .step {
    display: flex;
    align-items: center;
    justify-content: center;
    min-inline-size: var(--density-control-sm-min);
    min-block-size: var(--density-control-sm-min);
    padding-inline: var(--sp-050);
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-variant-numeric: tabular-nums;
    cursor: pointer;
    transition: background-color var(--motion-state-duration) var(--motion-state-easing);
  }

  .step:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  .step:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* Where the reader is. The fill is how it looks; `aria-current` is what it is, and the weight
     is there because a fill alone is a colour carrying meaning on its own (rule 3). */
  .step[aria-current='page'] {
    background: var(--accent-primary);
    color: var(--text-inverse);
    font-weight: var(--fw-semibold);
  }

  .step[aria-current='page']:hover {
    background: var(--accent-primary-hover);
    color: var(--text-inverse);
  }

  .is-gap { color: var(--text-subtle); }

  /* An arrow at the end of the list. It keeps its place in the tab order and says what it is;
     the pointer is the one thing it drops, because there is nothing to point at. */
  .step[aria-disabled='true'] {
    color: var(--text-subtle);
    opacity: 0.4;
    cursor: default;
  }

  .step[aria-disabled='true']:hover { background: none; color: var(--text-subtle); }

  /* Rule 6, and its second half: the attribute alongside the media query, because a preference
     only the operating system can set is not one this product offers (ADR-0037). */
  @media (prefers-reduced-motion: reduce) {
    .step { transition-duration: var(--dur-instant); }
  }

  :global([data-motion='reduced']) .step { transition-duration: var(--dur-instant); }
</style>
