<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The root every page kind shares: `data-page-kind`, the gaps between its parts, the head drawn
  // from `PageHead` - on a phone its primary floating above the bottom bar, labelled, the one main
  // action of the screen (the navigation concept §9) - and a refusal that takes the content's
  // place while the head stays (page-system §4.1).
  import type { Snippet } from 'svelte';

  import PageHeader from './PageHeader.svelte';
  import { pageBar, type PageHead, type PageKind } from './page.ts';

  interface Props {
    kind: PageKind;
    head?: PageHead;
    /** A head of the kind's own instead of `PageHeader` - the record's. */
    top?: Snippet;
    /** Hides the head - a section or an object opened below `expanded` stands alone. */
    isHeadHidden?: boolean;
    /** Whether the application's bar carries the title, the way up and the ⋯ (`PageHeader`'s). */
    isTitleInBar?: boolean;
    /** What the person may not see, in the content's place. */
    refused?: Snippet;
    children: Snippet;
  }

  const { kind, head, top, isHeadHidden = false, isTitleInBar = false, refused, children }: Props = $props();

  const bar = pageBar();
</script>

<div class="page" data-page-kind={kind}>
  {#if top && !isHeadHidden}
    {@render top()}
  {:else if head && !isHeadHidden}
    <PageHeader
      title={head.title}
      subtitle={head.subtitle}
      breadcrumb={head.breadcrumb}
      primary={head.primary}
      secondary={head.secondary}
      menu={head.menu}
      figure={head.figure}
      {isTitleInBar}
      isMenuInBar={isTitleInBar && bar !== undefined}
      onmenu={isTitleInBar ? bar?.offer : undefined}
    />
  {/if}
  {#if refused}
    <div class="refused">{@render refused()}</div>
  {:else}
    {@render children()}
  {/if}
</div>

<style>
  /* One rhythm for every kind and width, the navigation concept's mockup: the head stands
     closer to what follows it than the parts below it do to each other. Margins, not `gap`, so
     that a head which spaces its own parts (`PlaceHead`) is not spaced twice. */
  .page {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .page > :global(* + *) { margin-block-start: var(--sp-200); }
  .page > :global(:is(header, [data-head]):first-child + *) { margin-block-start: var(--sp-150); }

  .refused {
    padding: var(--sp-400) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }
</style>
