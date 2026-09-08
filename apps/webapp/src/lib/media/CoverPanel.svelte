<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The picture on an entry.
  //
  // **The manifest decides whether there is one at all.** A cover belongs to a type whose profile
  // carries `COVER` — a task by default, and neither a work package nor an activity — and this
  // component reads that rather than naming the types. A type without it is a gate with the
  // server's own reason, never a missing control.
  //
  // **Three steps, and the middle one leaves this origin.** Staging answers where the bytes go,
  // the bytes go there directly (arc42 §8.4), and the confirmation is what judges them and makes
  // the object usable. Only then is it a cover: setting one from a PENDING object is refused, and
  // rightly — the type has been claimed and not yet read.
  //
  // **The image is drawn from the download URL the server answers for a READY object.** That is
  // the one path the sniffed inline allowlist has already judged (C-05). Nothing here draws from a
  // blob the browser was handed, which would be this client rendering bytes nobody looked at.
  //
  // A cancelled upload leaves a staging behind and this says so. The reconciliation job takes it;
  // deleting it here would be guessing at whether the bytes ever arrived.

  import { Button, CapabilityGate, Stack, UploadField } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { acceptFor, coverImageIdOf, imageCover, isWithinUploadLimit, uploadLimitOf } from '../data/media.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item }: { item: WorkItem } = $props();

  const capability = $derived(supports(item.type, 'COVER'));
  const limit = $derived(uploadLimitOf(manifest.value));

  const imageId = $derived(coverImageIdOf(item.cover));
  const isColour = $derived(item.cover?.kind === 'COLOR' && !!item.cover.color_token);

  // `Date.now()` rather than a clock this component holds: what it answers only decides whether the
  // target the cache holds has expired, and the cache asks again when it has.
  const url = $derived(imageId ? media.coverUrl(imageId, Date.now()) : undefined);

  let chosen = $state<File | undefined>(undefined);
  let sent = $state(0);
  let controller = $state<AbortController | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);

  const isUploading = $derived(controller !== undefined);
  const overLimit = $derived(chosen !== undefined && !isWithinUploadLimit(chosen.size, limit));
  const percent = $derived(chosen && chosen.size > 0 ? Math.floor((sent / chosen.size) * 100) : 0);

  const sizeLabel = $derived(
    chosen === undefined
      ? undefined
      : limit === undefined
        ? t('app.media.size', { size: formatBytes(chosen.size, messages.locale) })
        : t('app.media.size_of_limit', {
            size: formatBytes(chosen.size, messages.locale),
            limit: formatBytes(limit, messages.locale),
          }),
  );

  async function choose(file: File) {
    chosen = file;
    failure = undefined;
    sent = 0;
    // Refused before a byte leaves, from the installation's own ceiling. The alternative is a
    // person watching a large file upload and being told at confirmation that it was never
    // acceptable.
    if (!isWithinUploadLimit(file.size, limit)) return;

    const abort = new AbortController();
    controller = abort;
    try {
      const object = await media.upload(file, 'COVER', {
        idempotencyKey: crypto.randomUUID(),
        signal: abort.signal,
        onProgress: (moved) => (sent = moved),
      });
      await media.setCover(item.id, imageCover(object.id), item.version);
      chosen = undefined;
    } catch (error) {
      failure = abort.signal.aborted
        ? t('app.media.cancelled')
        : renderProblem(error as never, messages).message;
    } finally {
      controller = undefined;
    }
  }

  async function remove() {
    failure = undefined;
    try {
      await media.clearCover(item.id, item.version);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<CapabilityGate
  status={capability.status}
  reason={capability.status === 'refused' ? t(capability.code, capability.params) : undefined}
  pendingLabel={t('app.media.deciding')}
>
  <Stack gap="150">
    {#if imageId}
      {#if url}
        <img class="cover" src={url} alt={t('app.media.cover_alt', { title: item.title })} />
      {:else}
        <p class="quiet">{t('app.media.cover_drawing')}</p>
      {/if}
    {:else if isColour}
      <p class="quiet">{t('app.media.cover_colour')}</p>
    {:else}
      <p class="quiet">{t('app.media.cover_none')}</p>
    {/if}

    <UploadField
      label={t('app.media.cover')}
      hint={t('app.media.cover_hint')}
      chooseLabel={imageId ? t('app.media.cover_replace') : t('app.media.cover_choose')}
      dropLabel={t('app.media.drop')}
      cancelLabel={t('app.media.cancel')}
      accept={acceptFor('COVER')}
      file={chosen ?? null}
      {sizeLabel}
      overLimitLabel={overLimit ? t('app.media.over_limit') : undefined}
      progress={isUploading ? percent : undefined}
      progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
      onChoose={(file) => void choose(file)}
      onCancel={() => controller?.abort()}
    />

    {#if failure}<p class="failure">{failure}</p>{/if}

    {#if imageId || isColour}
      <div>
        <Button size="sm" tone="secondary" onclick={() => void remove()}>
          {t('app.media.cover_remove')}
        </Button>
      </div>
    {/if}
  </Stack>
</CapabilityGate>

<style>
  .cover {
    display: block;
    width: 100%;
    max-width: var(--sp-1600);
    height: auto;
    border-radius: var(--r-md);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
