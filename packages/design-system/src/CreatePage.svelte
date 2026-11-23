<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Creating something larger than a dialog holds (page-system §7.7, §5.1): a page like every other
  // kind at the content's full width - the trail, the title, the primary (*Create token*) and
  // *Cancel* in the head; steps, where order matters, under it; then titled sections as regions of
  // `SettingRow`s through `FormLayout` on a page. On a phone ✕ stands where the trail was.
  import type { Snippet } from 'svelte';

  import IconButton from './IconButton.svelte';
  import PageFrame from './_PageFrame.svelte';
  import type { PageHead } from './page.ts';

  interface Props {
    head: PageHead;
    /** ✕ on a phone, in the trail's place: leaves without creating. */
    close?: { label: string; onclose: () => void };
    /** A numbered segmented control, where order matters. */
    steps?: Snippet;
    refused?: Snippet;
    /**
     * Whether the page takes the region the frame gives a page that fills it: the head edge to edge
     * over a hairline, the content the rest of the height, so that panes inside it scroll on
     * their own - two columns of choices too long for one form (the start of a hub).
     */
    isFilled?: boolean;
    /** The `FormLayout` on a page, or the panes of a filled one. */
    children: Snippet;
  }

  const { head, close, steps, refused, isFilled = false, children }: Props = $props();
</script>

<div class="create-page" data-filled={isFilled ? '' : undefined}>
  {#if close}
    <span class="close"><IconButton icon="x" label={close.label} size="sm" onclick={close.onclose} /></span>
  {/if}
  <PageFrame kind="create" {head} {refused}>
    {#if steps}{@render steps()}{/if}
    {@render children()}
  </PageFrame>
</div>

<style>
  .create-page { display: flex; flex-direction: column; min-width: 0; }
  .close { display: none; }

  /* Filled, the head and the content share the region's height and nothing of the page kind's
     rhythm stands between them: the head's own padding and hairline part it from the panes. */
  .create-page[data-filled] { flex: 1 1 auto; block-size: 100%; min-block-size: 0; }
  .create-page[data-filled] :global([data-page-kind]) { flex: 1 1 auto; min-block-size: 0; }
  /* Twice the class: the page kind's own rhythm after its head is as specific as one. */
  .create-page.create-page[data-filled] :global([data-page-kind] > *) { margin-block-start: 0; }
  .create-page[data-filled] :global([data-page-kind] > :not(header)) { flex: 1 1 auto; min-block-size: 0; }
  .create-page[data-filled] :global([data-page-kind] > header) {
    padding: var(--sp-200) var(--sp-300);
    background: var(--bg-canvas);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .close { display: inline-flex; align-self: flex-start; margin-block-end: var(--sp-100); }
    .create-page :global([data-page-kind] > header .top) { display: none; }
    .create-page[data-filled] .close { margin: var(--sp-150) var(--sp-200) 0; }
    .create-page[data-filled] :global([data-page-kind] > header) { padding: var(--sp-100) var(--sp-200); }
  }
</style>
