<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Objects to find and act on (page-system §7.1): the head with the act that creates, the one
  // strip of sibling lists, the `FilterBar`, then one `DataTable` or `ResourceGrid` with its foot.
  // While rows are selected the `BulkBar` stands in the head's place. Every slot is the screen's to
  // fill; nothing goes beside them (PS-1).
  import type { Snippet } from 'svelte';

  import PageFrame from './_PageFrame.svelte';
  import type { PageHead } from './page.ts';

  interface Props {
    head: PageHead;
    /** The one strip: `Tabs` or `PlaceTabs` of the sibling lists (People · Groups · Roles). */
    strip?: Snippet;
    /** The `FilterBar`. */
    filters?: Snippet;
    /** The `BulkBar`, while selecting: it replaces the head. */
    bulk?: Snippet;
    /** What the person may not see, under the head. */
    refused?: Snippet;
    /** The `DataTable` or the `ResourceGrid`. */
    children: Snippet;
  }

  const { head, strip, filters, bulk, refused, children }: Props = $props();
</script>

<PageFrame kind="list" {head} isHeadHidden={bulk !== undefined} {refused}>
  {#if bulk}{@render bulk()}{/if}
  {#if strip}{@render strip()}{/if}
  {#if filters}{@render filters()}{/if}
  {@render children()}
</PageFrame>
