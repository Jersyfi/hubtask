<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A file's picture on a card (ET-4 grid, F-4 board card): a picture as itself, a PDF's first page
  // from the viewer's rendering, anything else nothing - the caller's mark and icon stand instead.
  // Drawn only once the card is on screen, so a grid of forty files is not forty downloads.

  import type { MediaObject } from '@hubtask/sync-engine';

  import { engine } from '../data/engine.ts';
  import { media } from '../data/media.svelte.ts';
  import { transferTimeoutFor } from '../data/media.ts';
  import { drawPage, openPdf } from './pdf.ts';
  import { viewOf } from './viewing.ts';

  interface Props {
    readonly file: MediaObject;
    /** The width the first page is drawn at, in CSS pixels. */
    readonly width?: number;
    /** Told whether a picture stands, so the caller drops its icon. */
    onshown?: (isShown: boolean) => void;
  }
  const { file, width = 240, onshown }: Props = $props();

  /** PDFs past this size keep their icon: a card is not worth a large download. */
  const PREVIEWED_PDF = 15 * 1024 * 1024;
  const kind = $derived(viewOf(file));
  let host = $state<HTMLElement>();
  let canvas = $state<HTMLCanvasElement>();
  let isVisible = $state(false);
  let isDrawn = $state(false);
  const pictureUrl = $derived(kind === 'image' && isVisible ? media.coverUrl(file.id, Date.now()) : undefined);

  $effect(() => {
    if (!host) return;
    const watch = new IntersectionObserver((seen) => { if (seen.some((each) => each.isIntersecting)) { isVisible = true; watch.disconnect(); } });
    watch.observe(host);
    return () => watch.disconnect();
  });

  $effect(() => {
    if (!isVisible || kind !== 'pdf' || file.size > PREVIEWED_PDF || !canvas) return;
    const target = canvas;
    let isCurrent = true;
    void (async () => {
      try {
        const url = await media.downloadUrl(file.id, Date.now());
        if (!url || !isCurrent) return;
        const bytes = await engine.receive({ url, timeoutMs: transferTimeoutFor(file.size) });
        const document = await openPdf(await bytes.arrayBuffer());
        try {
          if (isCurrent) await drawPage(document, 1, target, width);
          if (isCurrent) isDrawn = true;
        } finally {
          void document.destroy();
        }
      } catch {
        // A preview that cannot be drawn leaves the card's icon; the viewer says why.
      }
    })();
    return () => { isCurrent = false; };
  });

  $effect(() => onshown?.(isDrawn || pictureUrl !== undefined));
</script>

<span class="preview" bind:this={host} data-file-preview={kind} data-drawn={isDrawn || pictureUrl ? '' : undefined}>
  {#if pictureUrl}
    <img src={pictureUrl} alt="" />
  {:else if kind === 'pdf'}
    <canvas bind:this={canvas} aria-hidden="true"></canvas>
  {/if}
</span>

<style>
  .preview { position: absolute; inset: 0; display: grid; place-items: center; overflow: hidden; pointer-events: none; }
  .preview:not([data-drawn]) > * { visibility: hidden; }
  img { inline-size: 100%; block-size: 100%; object-fit: cover; }
  canvas { inline-size: 70%; block-size: auto; margin-block-start: var(--sp-200); box-shadow: var(--shadow-raised); background: var(--bg-surface); align-self: start; }
</style>
