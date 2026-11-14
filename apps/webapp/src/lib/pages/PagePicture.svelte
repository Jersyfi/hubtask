<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One of a page's pictures and files where the body draws it (UC-DOC-14): an image the server
  // serves inline as itself, anything else as a card. Either opens the one viewer (F-1) with every
  // file the page draws at its foot. An object the reader may not read is a card saying so - never
  // its name.

  import type { MediaObject } from '@hubtask/sync-engine';

  import { media } from '../data/media.svelte.ts';
  import { viewer, type ViewerOwner } from '../files/viewer.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    mediaId: string;
    /** Every file the page draws, this one among them, in the body's order. */
    siblings?: () => readonly string[];
    /** The page, for "2 of 5 · on <page>". */
    owner?: () => ViewerOwner;
  }

  const { mediaId, siblings = () => [mediaId], owner }: Props = $props();

  $effect(() => media.resolve([mediaId]));

  const record = $derived(media.recordOf(mediaId));
  const name = $derived(record?.file_name ?? '');
  const isPicture = $derived((record?.content_type ?? '').startsWith('image/') && record?.content_type !== 'image/svg+xml');
  const source = $derived(isPicture ? media.coverUrl(mediaId, Date.now()) : undefined);

  function open() {
    const ids = siblings();
    media.resolve(ids);
    const files = ids.map((id) => media.recordOf(id)).filter((each): each is MediaObject => each !== undefined);
    viewer.open({ files, index: files.findIndex((each) => each.id === mediaId), owner: owner?.() });
  }

  // An address naming this file (`?file=`, the viewer's *link*) opens it once its record is read.
  let isLinkedOpened = false;
  $effect(() => {
    if (isLinkedOpened || !record) return;
    isLinkedOpened = true;
    if (new URL(globalThis.location.href).searchParams.get('file') === mediaId) open();
  });
</script>

{#if source}
  <button type="button" class="picture-open" onclick={open} aria-label={t('app.pages.file_open', { name })}>
    <img class="picture" src={source} alt={name} />
  </button>
{:else if record}
  <button type="button" class="file" onclick={open} aria-label={t('app.pages.file_open', { name })}>{name}</button>
{:else}
  <span class="file file--quiet">{t('app.pages.file_unavailable')}</span>
{/if}

<style>
  .picture-open {
    display: block;
    padding: 0;
    border: 0;
    background: none;
    cursor: zoom-in;
  }
  .picture {
    display: block;
    max-inline-size: 100%;
    block-size: auto;
    border-radius: var(--r-md);
  }
  .file {
    display: inline-flex;
    align-items: center;
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-primary);
    font: inherit;
    font-weight: var(--fw-medium);
    cursor: pointer;
  }
  .file--quiet { color: var(--text-secondary); cursor: default; }
</style>
