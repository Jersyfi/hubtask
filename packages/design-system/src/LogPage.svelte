<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What happened (page-system §7.6): a list whose `FilterBar` starts with the time range, whose
  // rows are grouped by day (`DataTable`'s `group`) and say words instead of keys; a row opens its
  // record in a Dialog M, the key under *Details for support*. Its own kind so a screen says what
  // it is (`data-page-kind="log"`), with no bulk selection: a record of what happened is not acted
  // on in bulk.
  import type { Snippet } from 'svelte';

  import HelpTip from './HelpTip.svelte';
  import PageFrame from './_PageFrame.svelte';
  import type { PageHead } from './page.ts';

  interface Props {
    head: PageHead;
    /**
     * What the page is for: under the head, above the `FilterBar`. Given an ⓘ where the bar carries
     * the title, to render at the end of its sentence.
     */
    intro?: Snippet<[Snippet | undefined]>;
    /** The `FilterBar`, the time range first. */
    filters?: Snippet;
    refused?: Snippet;
    /** The `DataTable`, grouped by day. */
    children: Snippet;
  }

  const { head, intro, filters, refused, children: content }: Props = $props();
</script>

<PageFrame kind="log" {head} {refused} hasIntro={intro !== undefined}>
  {#snippet children(more)}
    {#snippet tip()}<HelpTip subject={head.title} text={more ?? ''} />{/snippet}
    {#if intro}{@render intro(more === undefined ? undefined : tip)}{/if}
    {#if filters}{@render filters()}{/if}
    {@render content()}
  {/snippet}
</PageFrame>
