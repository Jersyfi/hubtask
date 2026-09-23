<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The bar at the top of every page (ADR-0061 decision 2, the first half of wave 5).
  //
  // It holds four things and refuses a fifth. At the start, the way into the navigation: a
  // drawer trigger below `expanded`, the rail toggle above it - the caller says which, because the
  // caller knows the width and this component does not. In the middle, the brand or the page's
  // title: on a phone the title is what the bar carries, because the page head below it has no
  // room to say it twice. Then the entry to search, where the caller has room for one. At the
  // end, the account menu, or whatever the caller renders there.
  //
  // **The search slot reverses ADR-0061's "the bar carries no search field" (ADR-0063 decision
  // 4).** What changed is what the field *is*: not a second destination, but the entry to the one
  // that exists - typing in it leads to the search page rather than doing the searching here. The
  // rule it was protecting still holds, and the caller keeps it: one visible entry to search on
  // every width, so a caller that fills this slot takes the destination out of its list.
  //
  // What it still carries **no** slot for is a page *action*. A page's actions belong to
  // `PageHeader`, where one is primary and the rest are a menu — and it is that menu, folded,
  // which the last slot draws: ADR-0061 decision 1's table says the compact bar holds
  // ☰ · the page title · the page menu, and the head hands its own folded list up rather than
  // drawing a second one.
  //
  // Sticky on `layer.sticky` and flat: a bar is not a standalone element in the sense of rule 1,
  // so it takes a hairline and no shadow. The top safe-area inset is *read* into its padding
  // rather than written anywhere, and is 0 in a browser.
  //
  // The title is a span, never a heading. Every screen holds exactly one `h1` and it belongs to
  // the content; a bar that drew a second would be a second landmark of the same rank. A page
  // head that hides its heading on a phone hides it visually only, and the reader is told the
  // title once, by the heading.

  import type { Snippet } from 'svelte';

  import IconButton from './IconButton.svelte';

  export interface NavigationToggle {
    /** A drawer that opens over the page, or a pinned navigation that folds to its marks. */
    kind: 'drawer' | 'rail';
    /** What the control does, in words: "Open the navigation", "Collapse the navigation". */
    label: string;
    /** Whether what it controls is open. Announced through `aria-expanded`. */
    isExpanded?: boolean;
    onToggle: () => void;
    /** For the tour and for tests: the attribute the caller finds the control by. */
    tour?: string;
  }

  interface Props {
    /** What the bar is called, for the landmark. Resolved text (ADR-0011). */
    label: string;
    /** The way into the navigation. Omitted when there is nothing to open - signed out. */
    toggle?: NavigationToggle;
    /** The page's title, shown in place of the brand: what the bar says on a phone. */
    title?: string;
    /** The brand - the wordmark link. Rendered when there is no title. */
    brand?: Snippet;
    /**
     * The entry to search, between the lead and the end (ADR-0063 decision 4).
     *
     * A slot rather than a field, because the bar knows nothing about what is searched - the same
     * reason `SearchField` knows nothing about when a request is sent. Omitted where there is no
     * room for it: on a phone the bar is a title and two controls, and search is a destination.
     */
    search?: Snippet;
    /** The controls at the end: the account menu. */
    end?: Snippet;
    /**
     * The page's own menu, at the very end (ADR-0061 decision 1: on `compact` the bar holds
     * ☰ · the page title · the page menu).
     *
     * A slot rather than a list, because what the page's actions are is the page's business and
     * this component draws frames rather than pages. The head hands its folded list over through
     * the caller, which is what keeps one folding rather than two.
     */
    menu?: Snippet;
  }

  const { label, toggle, title, brand, search, end, menu }: Props = $props();
</script>

<header class="bar" aria-label={label}>
  <div class="row">
    {#if toggle}
      <IconButton
        icon={toggle.kind === 'drawer' ? 'menu' : 'panel-left'}
        label={toggle.label}
        aria-expanded={toggle.isExpanded}
        data-toggle={toggle.kind}
        data-tour={toggle.tour}
        onclick={toggle.onToggle}
      />
    {/if}
    <div class="lead">
      {#if title}
        <!-- Named, because a bar with a sheet opened from it holds a second `.title` — the
             sheet's own heading — and "the bar's title" has to be a question with one answer. -->
        <span class="title" data-bar="title">{title}</span>
      {:else if brand}
        {@render brand()}
      {/if}
    </div>
    {#if search}
      <div class="search">{@render search()}</div>
    {/if}
    {#if end}
      <div class="end">{@render end()}</div>
    {/if}
    {#if menu}
      <div class="page-menu" data-bar="menu">{@render menu()}</div>
    {/if}
  </div>
</header>

<style>
  .bar {
    position: sticky;
    inset-block-start: 0;
    z-index: var(--z-sticky);
    /* Read, not written: the notch's inset on a phone, nothing in a browser. A unitless zero is
       the fallback because a length here would be a value outside tokens.json. */
    padding-block-start: env(safe-area-inset-top, 0);
    background: var(--bg-surface);
    color: var(--text-primary);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    block-size: var(--layout-appbar-height);
    padding-inline: var(--sp-200);
  }

  .lead {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    flex: 1;
    min-width: 0;
  }

  /* One line, and the end of a long title is what goes, not the controls beside it (rule 4). */
  .title {
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Centred in the row and bounded: the field is the bar's, not the page's, so it takes a few
     words' worth and gives the rest back. `min-inline-size: 0` because a flex item's automatic
     minimum size would otherwise hold the input's default width and push the account menu out. */
  .search {
    flex: 1 1 var(--layout-barsearch-width);
    max-inline-size: var(--layout-barsearch-width);
    min-inline-size: 0;
  }

  .end {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    flex: none;
    margin-inline-start: auto;
  }

  /* After the end, and pushed there itself when the end is absent: the page's menu is the last
     thing in the row on every width that draws it. */
  .page-menu {
    display: flex;
    align-items: center;
    flex: none;
    margin-inline-start: auto;
  }

  /* With both present the end has already taken the free space, so the menu sits against it. */
  .end + .page-menu { margin-inline-start: 0; }
</style>
