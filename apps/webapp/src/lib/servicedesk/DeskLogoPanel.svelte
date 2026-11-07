<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A desk's own logo, for who may change the collection (UC-SVC-05 check 9): drawn at the top of
  // its portal in place of the icon. One press is one write, as a picture of oneself is - choosing
  // a file uploads, confirms and sets it, *Remove* takes it away; the logo is drawn only from an
  // object the server has judged an image (T-11).

  import { Button, Stack, UploadField } from '@hubtask/design-system/components';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { AVATAR_LIMIT, isWithinUploadLimit, pictureLimitOf } from '../data/media.ts';
  import { portal } from '../data/portal.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { collectionId, hasLogo }: { collectionId: string; hasLogo: boolean } = $props();

  const limit = $derived(pictureLimitOf(manifest.value));

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
      : t('app.media.size_of_limit', { size: formatBytes(chosen.size, messages.locale), limit: formatBytes(limit, messages.locale) }),
  );

  async function choose(file: File) {
    chosen = file;
    failure = undefined;
    sent = 0;
    if (!isWithinUploadLimit(file.size, limit)) return;

    const abort = new AbortController();
    controller = abort;
    try {
      const object = await media.upload(file, 'DESK_LOGO', {
        idempotencyKey: crypto.randomUUID(),
        signal: abort.signal,
        onProgress: (moved) => (sent = moved),
      });
      await portal.setLogo(collectionId, object.id);
      toast.show({ text: t('app.desk_logo.set'), tone: 'success' });
      chosen = undefined;
    } catch (error) {
      failure = abort.signal.aborted ? t('app.media.cancelled') : renderProblem(error as never, messages).message;
    } finally {
      controller = undefined;
    }
  }

  async function remove() {
    failure = undefined;
    try {
      await portal.removeLogo(collectionId);
      toast.show({ text: t('app.desk_logo.removed'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<Stack gap="100">
  <UploadField
    label={t('app.desk_logo.title')}
    hint={t('app.desk_logo.hint', { limit: formatBytes(AVATAR_LIMIT, messages.locale) })}
    chooseLabel={hasLogo ? t('app.desk_logo.replace') : t('app.desk_logo.choose')}
    dropLabel={t('app.media.drop')}
    cancelLabel={t('app.media.cancel')}
    accept="image/png,image/jpeg,image/gif,image/webp"
    file={chosen ?? null}
    {sizeLabel}
    overLimitLabel={overLimit ? t('app.media.over_limit') : undefined}
    progress={isUploading ? percent : undefined}
    progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
    onChoose={(file) => void choose(file)}
    onCancel={() => controller?.abort()}
  />

  {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

  {#if hasLogo}
    <div>
      <Button size="sm" tone="secondary" onclick={() => void remove()}>{t('app.desk_logo.remove')}</Button>
    </div>
  {/if}
</Stack>

<style>
  .failure { margin: 0; color: var(--text-danger); }
</style>
