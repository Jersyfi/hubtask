<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Objects to find and act on (page-system §7.1): the head with the act that creates, the one
  // strip of sibling lists, the `FilterBar`, then one `DataTable` or `ResourceGrid` with its foot.
  // While rows are selected the `BulkBar` stands in the head's place. Every slot is the screen's to
  // fill; nothing goes beside them (PS-1).
  import type { Snippet } from 'svelte';

  import HelpTip from './HelpTip.svelte';
  import PageFrame from './_PageFrame.svelte';
  import type { PageHead } from './page.ts';

  interface Props {
    head: PageHead;
    /** A product home's own head instead of `PageHeader`: its `ProductBand` (WB-2, WB-5). */
    top?: Snippet;
    /** The one strip: `Tabs` or `ViewBar` of the sibling lists (People · Groups · Roles). */
    strip?: Snippet;
    /**
     * What the page is for: under the head and the strip, above the `FilterBar`. Given an ⓘ where
     * the bar carries the title, to render at the end of its sentence.
     */
    intro?: Snippet<[Snippet | undefined]>;
    /** The `FilterBar`. */
    filters?: Snippet;
    /** The `BulkBar`, while selecting: it replaces the head. */
    bulk?: Snippet;
    /** What the person may not see, under the head. */
    refused?: Snippet;
    /** The `DataTable` or the `ResourceGrid`. */
    children: Snippet;
  }

  const { head, top, strip, intro, filters, bulk, refused, children: content }: Props = $props();
</script>

<PageFrame kind="list" {head} {top} isHeadHidden={bulk !== undefined} {refused} hasIntro={intro !== undefined}>
  {#snippet children(more)}
    {#snippet tip()}<HelpTip subject={head.title} text={more ?? ''} />{/snippet}
    {#if bulk}{@render bulk()}{/if}
    {#if strip}{@render strip()}{/if}
    {#if intro}{@render intro(more === undefined ? undefined : tip)}{/if}
    {#if filters}{@render filters()}{/if}
    {@render content()}
  {/snippet}
</PageFrame>
