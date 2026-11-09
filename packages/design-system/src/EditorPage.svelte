<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A composite object built from blocks (page-system §7.4): the list of objects with its search
  // beside the selected one; the draft bar - *Draft · 2 changes · Discard · Try · Publish…* - sticky
  // at the editor's top; then the object's head and its `BlockList`s, each block opening its
  // dialog. Below `expanded` the list is the page and an object opens alone with *‹ back*.
  import type { Snippet } from 'svelte';

  import type { SectionLink } from './SectionNav.svelte';
  import PageFrame from './_PageFrame.svelte';
  import TwoPane from './_TwoPane.svelte';
  import type { PageBack, PageHead } from './page.ts';

  interface Props {
    head: PageHead;
    /** The objects: `SectionNav`'s props, with the search over them. */
    nav: { label: string; sections: readonly SectionLink[]; current?: string; onselect?: (id: string) => void; search?: Snippet };
    view?: 'list' | 'detail';
    back: PageBack;
    /** The draft bar: what is unpublished, and its acts. */
    draft?: { label: string; actions: Snippet };
    refused?: Snippet;
    /** The selected object's head and its `BlockList`s. */
    children: Snippet;
  }

  const { head, nav, view = 'detail', back, draft, refused, children }: Props = $props();
</script>

<div class="editor-page" data-view={view}>
  <PageFrame kind="editor" {head} {refused}>
    <TwoPane {nav} {view} {back}>
      {#if draft}
        <div class="draft" role="status">
          <span>{draft.label}</span>
          <span class="draft-acts">{@render draft.actions()}</span>
        </div>
      {/if}
      {@render children()}
    </TwoPane>
  </PageFrame>
</div>

<style>
  .draft {
    position: sticky;
    inset-block-start: calc(var(--page-sticky-top, var(--layout-appbar-height)) + var(--sp-100));
    z-index: var(--z-sticky);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-150);
    padding: var(--sp-100) var(--sp-200);
    border: var(--bw-hairline) solid var(--status-info-border);
    border-radius: var(--r-lg);
    background: var(--status-info-surface);
    color: var(--status-info-text);
    font-size: var(--fs-100);
  }

  .draft-acts { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .editor-page[data-view='detail'] :global([data-page-kind] > header) { display: none; }
  }
</style>
