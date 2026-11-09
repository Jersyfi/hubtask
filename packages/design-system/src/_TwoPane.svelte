<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // List and detail (page-system §4.4, D-4): from `expanded` up the `SectionNav` beside the open
  // section, sticky under the bar and scrolling on its own when it is longer than the window; below
  // `expanded` one pane - the list is the page, and an open section stands alone with its way back.
  // Which one shows is the address's question, so the caller says it (`view`).
  import type { Snippet } from 'svelte';

  import Icon from './Icon.svelte';
  import SectionNav, { type SectionLink } from './SectionNav.svelte';
  import type { PageBack } from './page.ts';

  interface Props {
    nav: { label: string; sections: readonly SectionLink[]; current?: string; onselect?: (id: string) => void; search?: Snippet };
    view: 'list' | 'detail';
    back: PageBack;
    /** Whether the application's bar carries the way back, so the pane draws none. */
    isBackHidden?: boolean;
    children: Snippet;
  }

  const { nav, view, back, isBackHidden = false, children }: Props = $props();
</script>

<div class="two-pane" data-view={view}>
  <div class="list">
    <SectionNav label={nav.label} sections={nav.sections} current={nav.current} onselect={nav.onselect} search={nav.search} />
  </div>
  <div class="detail">
    {#if !isBackHidden}
      <button type="button" class="back" onclick={back.onback}>
        <Icon name="chevron-left" size="sm" /><span>{back.label}</span>
      </button>
    {/if}
    {@render children()}
  </div>
</div>

<style>
  .two-pane {
    display: grid;
    grid-template-columns: var(--layout-settings-nav) minmax(0, 1fr);
    align-items: start;
    gap: var(--sp-500);
  }

  .list {
    position: sticky;
    inset-block-start: calc(var(--page-sticky-top, var(--layout-appbar-height)) + var(--sp-300));
    max-block-size: calc(100vh - var(--page-sticky-top, var(--layout-appbar-height)) - var(--sp-600));
    overflow: auto;
  }

  .detail { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }

  .back {
    display: none;
    align-self: flex-start;
    align-items: center;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    border: 0;
    background: none;
    color: var(--text-brand);
    font: inherit;
    font-size: var(--fs-100);
    cursor: pointer;
  }

  .back:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  /* One pane below `expanded`: the list or the section, never both (D-4). */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .two-pane { grid-template-columns: minmax(0, 1fr); }
    .list { position: static; max-block-size: none; overflow: visible; }
    .two-pane[data-view='list'] .detail,
    .two-pane[data-view='detail'] .list { display: none; }
    .back { display: inline-flex; }
  }
</style>
