<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One of a page's pictures and files where the body draws it (UC-DOC-14): an image the server
  // serves inline as itself, anything else as a card that opens the file. An object the reader may
  // not read is a card saying so - never its name.

  import { media } from '../data/media.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    mediaId: string;
  }

  const { mediaId }: Props = $props();

  $effect(() => media.resolve([mediaId]));

  const record = $derived(media.recordOf(mediaId));
  const name = $derived(record?.file_name ?? '');
  const isPicture = $derived((record?.content_type ?? '').startsWith('image/') && record?.content_type !== 'image/svg+xml');
  const source = $derived(isPicture ? media.coverUrl(mediaId, Date.now()) : undefined);

  async function open() {
    const url = await media.downloadUrl(mediaId, Date.now());
    if (url) window.open(url, '_blank', 'noopener');
    else toast.show({ text: t('app.pages.file_unavailable'), tone: 'danger' });
  }
</script>

{#if source}
  <img class="picture" src={source} alt={name} />
{:else if record}
  <button type="button" class="file" onclick={() => void open()} aria-label={t('app.pages.file_open', { name })}>{name}</button>
{:else}
  <span class="file file--quiet">{t('app.pages.file_unavailable')}</span>
{/if}

<style>
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
