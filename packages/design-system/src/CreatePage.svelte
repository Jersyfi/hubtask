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
    /** The `FormLayout` on a page. */
    children: Snippet;
  }

  const { head, close, steps, refused, children }: Props = $props();
</script>

<div class="create-page">
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

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .close { display: inline-flex; align-self: flex-start; margin-block-end: var(--sp-100); }
    .create-page :global([data-page-kind] > header .top) { display: none; }
  }
</style>
