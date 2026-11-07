<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The root every page kind shares: `data-page-kind`, the gaps between its parts, the head drawn
  // from `PageHead` with its primary in the flow, and a refusal that takes the content's place
  // while the head stays (page-system §4.1).
  import type { Snippet } from 'svelte';

  import PageHeader from './PageHeader.svelte';
  import type { PageHead, PageKind } from './page.ts';

  interface Props {
    kind: PageKind;
    head?: PageHead;
    /** A head of the kind's own instead of `PageHeader` - the record's. */
    top?: Snippet;
    /** Hides the head - a section or an object opened below `expanded` stands alone. */
    isHeadHidden?: boolean;
    /** What the person may not see, in the content's place. */
    refused?: Snippet;
    children: Snippet;
  }

  const { kind, head, top, isHeadHidden = false, refused, children }: Props = $props();
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
      isPrimaryInFlow
    />
  {/if}
  {#if refused}
    <div class="refused">{@render refused()}</div>
  {:else}
    {@render children()}
  {/if}
</div>

<style>
  .page {
    --page-head-title-size: var(--fs-600);
    display: flex;
    flex-direction: column;
    gap: var(--sp-250);
    min-width: 0;
  }

  .page[data-page-kind='record'] { gap: var(--sp-300); }

  .refused {
    padding: var(--sp-400) var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .page { --page-head-title-size: var(--fs-500); gap: var(--sp-200); }
  }
</style>
