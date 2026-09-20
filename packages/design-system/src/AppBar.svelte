<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The bar at the top of every page (ADR-0061 decision 2, the first half of wave 5).
  //
  // It holds three things and refuses a fourth. At the start, the way into the navigation: a
  // drawer trigger below `expanded`, the rail toggle above it - the caller says which, because the
  // caller knows the width and this component does not. In the middle, the brand or the page's
  // title: on a phone the title is what the bar carries, because the page head below it has no
  // room to say it twice. At the end, the account menu, or whatever the caller renders there.
  //
  // What it carries **no** slot for is a page action or a search field. A page's actions belong
  // to `PageHeader`, where one is primary and the rest are a menu; the search is a destination of
  // the one navigation list, and a field up here would be a second entry to it - the duplication
  // that list exists to prevent.
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
    /** The controls at the end: the account menu. */
    end?: Snippet;
  }

  const { label, toggle, title, brand, end }: Props = $props();
</script>

<header class="bar" aria-label={label}>
  <div class="row">
    {#if toggle}
      <IconButton
        icon={toggle.kind === 'drawer' ? 'menu' : 'panel-left'}
        label={toggle.label}
        aria-expanded={toggle.isExpanded}
        data-toggle={toggle.kind}
        onclick={toggle.onToggle}
      />
    {/if}
    <div class="lead">
      {#if title}
        <span class="title">{title}</span>
      {:else if brand}
        {@render brand()}
      {/if}
    </div>
    {#if end}
      <div class="end">{@render end()}</div>
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

  .end {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    flex: none;
    margin-inline-start: auto;
  }
</style>
