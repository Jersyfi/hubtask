<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One object read and edited in place - an entry, a page, a goal (page-system §7.8, as built,
  // design-system §11.8): its head, then the main column and the details column. The main column
  // comes first in the document, so reading and tab order go from the head into the content before
  // the properties; from `expanded` the details stand beside it at the inline end, at the pane's
  // width, and stay in place while it scrolls (PS-17). Below `expanded` a page draws them under the
  // head (a phone's *All properties*); in a side panel they follow the content.
  import type { Snippet } from 'svelte';

  import PageFrame from './_PageFrame.svelte';
  import type { PageHead } from './page.ts';

  interface Props {
    head?: PageHead;
    /** A head of the object's own instead of `head`: its title is the field it is edited in. */
    top?: Snippet;
    /** The details column. */
    details?: Snippet;
    /**
     * Drawn inside a work page's side panel: one column, and no page kind of its own - the panel
     * is a part of the work page it opens beside (PS-1).
     */
    isInPane?: boolean;
    refused?: Snippet;
    /** The main column. */
    children: Snippet;
  }

  const { head, top, details, isInPane = false, refused, children }: Props = $props();
</script>

{#snippet columns()}
  <div class="columns" data-pane={isInPane ? '' : undefined}>
    <div class="main">{@render children()}</div>
    {#if details}<div class="details">{@render details()}</div>{/if}
  </div>
{/snippet}

{#if isInPane}
  <div class="pane">
    {#if top}{@render top()}{/if}
    {@render columns()}
  </div>
{:else}
  <PageFrame kind="detail" {head} {top} {refused}>
    {@render columns()}
  </PageFrame>
{/if}

<style>
  .pane,
  .columns,
  .main {
    display: flex;
    flex-direction: column;
    gap: var(--sp-300);
    min-width: 0;
  }

  .main { flex: 1; }

  .details { min-width: 0; }
  .columns:not([data-pane]) .details { order: -1; }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) {
    .columns:not([data-pane]) { flex-direction: row; align-items: flex-start; }
    .columns:not([data-pane]) .details { order: 0; }

    .columns:not([data-pane]) .details {
      flex: none;
      inline-size: var(--layout-rail);
      position: sticky;
      inset-block-start: calc(var(--page-sticky-top, var(--layout-appbar-height)) + var(--sp-200));
    }
  }
</style>
