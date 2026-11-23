<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Creating something larger than a dialog holds (page-system §7.7, §5.1): a page like every other
  // kind at the content's full width - the trail, the title, the primary (*Create token*) and
  // *Cancel* in the head; steps, where order matters, under it; then titled sections as regions of
  // `SettingRow`s through `FormLayout` on a page. On a phone ✕ stands where the trail was.
  //
  // Filled, below `medium` the page is the phone's whole screen: one bar with ✕ and the title, the
  // content, and the primary across the width in a bar at the foot, where the thumb is - above the
  // on-screen keyboard while one is open, so typing never hides the act it leads to. The frame
  // draws neither of its own bars beside such a page.
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import PageFrame from './_PageFrame.svelte';
  import { isCompactWidth, watchCompactWidth } from './media.ts';
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
     * their own - two columns of choices too long for one form (the start of a hub). Below
     * `medium` it is the whole screen, its primary in a bar at the foot.
     */
    isFilled?: boolean;
    /** The `FormLayout` on a page, or the panes of a filled one. */
    children: Snippet;
  }

  const { head, close, steps, refused, isFilled = false, children }: Props = $props();

  // How far the on-screen keyboard reaches into the window, which the foot's bar stands on: the
  // keyboard narrows the visual viewport only, and a bar kept at the layout viewport's foot would
  // be under it.
  let keyboardInset = $state(0);
  let isCompact = $state(isCompactWidth());
  $effect(() => (isFilled ? watchCompactWidth((next) => (isCompact = next)) : undefined));
  $effect(() => {
    const seen = window.visualViewport;
    if (!isFilled || !isCompact || !seen) return;
    const measure = () => (keyboardInset = Math.max(0, Math.round(window.innerHeight - seen.height - seen.offsetTop)));
    measure();
    seen.addEventListener('resize', measure);
    seen.addEventListener('scroll', measure);
    return () => {
      keyboardInset = 0;
      seen.removeEventListener('resize', measure);
      seen.removeEventListener('scroll', measure);
    };
  });
</script>

<div class="create-page" data-filled={isFilled ? '' : undefined}>
  {#if close}
    <span class="close"><IconButton icon="x" label={close.label} size="sm" onclick={close.onclose} /></span>
  {/if}
  <PageFrame kind="create" {head} {refused}>
    {#if steps}{@render steps()}{/if}
    {@render children()}
  </PageFrame>
  {#if isFilled && head.primary?.onclick && !refused}
    <!-- The head's primary again, drawn only where the head's is not: one of the two is ever in
         the tree, so the reader meets one *Create*. -->
    <div class="thumb" data-thumb style:inset-block-end={keyboardInset > 0 ? `${keyboardInset}px` : undefined}>
      <Button tone="primary" isFull onclick={head.primary.onclick} disabledReason={head.primary.disabledReason} data-tour={head.primary.tour}>
        {head.primary.label}
      </Button>
    </div>
  {/if}
</div>

<style>
  .create-page { display: flex; flex-direction: column; min-width: 0; }
  .close { display: none; }
  .thumb { display: none; }

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

    /* The phone's whole screen: ✕ and the title in one bar that stays at the top, the content
       scrolling under it, the primary's bar staying at the foot. The page kind's root lends its
       head and its content to this grid. */
    .create-page[data-filled] {
      display: grid;
      grid-template-columns: auto minmax(0, 1fr);
      grid-template-rows: auto minmax(0, 1fr) auto;
      min-block-size: 100dvh;
    }

    .create-page[data-filled] :global([data-page-kind]) { display: contents; }

    .create-page[data-filled] .close,
    .create-page[data-filled] :global([data-page-kind] > header) {
      box-sizing: border-box;
      position: sticky;
      inset-block-start: 0;
      z-index: var(--z-sticky);
      grid-row: 1;
      align-self: stretch;
      margin: 0;
      padding-block: calc(var(--sp-100) + env(safe-area-inset-top, 0)) var(--sp-100);
      background: var(--bg-canvas);
      border-block-end: var(--bw-hairline) solid var(--border-subtle);
    }

    .create-page[data-filled] .close { grid-column: 1; align-items: center; padding-inline: var(--sp-150) 0; }

    .create-page[data-filled] :global([data-page-kind] > header) {
      grid-column: 2;
      justify-content: center;
      min-block-size: var(--layout-appbar-height);
      padding-inline: var(--sp-100) var(--sp-150);
    }

    /* The title as the bar's, and no head actions: ✕ is the way out, the foot holds the primary. */
    .create-page[data-filled] :global([data-page-kind] > header .row) { align-items: center; }
    .create-page[data-filled] :global([data-page-kind] > header .title) { font-size: var(--fs-300); font-weight: var(--fw-semibold); }
    .create-page[data-filled] :global([data-page-kind] > header .actions) { display: none; }

    .create-page[data-filled] :global([data-page-kind] > :not(header)) { grid-column: 1 / -1; grid-row: 2; }

    .create-page[data-filled] .thumb {
      box-sizing: border-box;
      position: sticky;
      inset-block-end: 0;
      z-index: var(--z-sticky);
      grid-column: 1 / -1;
      grid-row: 3;
      display: flex;
      align-items: center;
      min-block-size: var(--layout-bottombar-height);
      padding: var(--sp-100) var(--sp-200) calc(var(--sp-100) + env(safe-area-inset-bottom, 0));
      background: var(--bg-surface);
      border-block-start: var(--bw-hairline) solid var(--border-subtle);
    }
  }
</style>
