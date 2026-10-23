<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A picture of one's own, shown to every member beside the name in place of the initials.
  //
  // **One press is one write**, as a cover's is: choosing a file uploads it, confirms it and sets it,
  // and *Remove* takes it away - neither waits for the save bar, which is for the fields somebody
  // fills in and leaves. The three upload steps are the media store's; the picture is set only from
  // an object the server has judged (T-11), and drawn from the download URL it answers, never from
  // the file the browser was handed.
  //
  // **Refused before a byte leaves** where the file is past the picture's own bound or the
  // installation's lower one; the server refuses the same and says why.

  import { Button, Stack, UploadField } from '@hubtask/design-system/components';

  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { AVATAR_LIMIT, isWithinUploadLimit, pictureLimitOf } from '../data/media.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const hasPicture = $derived(!!actor.account?.avatar_media_id);
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
      : t('app.media.size_of_limit', {
          size: formatBytes(chosen.size, messages.locale),
          limit: formatBytes(limit, messages.locale),
        }),
  );

  async function choose(file: File) {
    chosen = file;
    failure = undefined;
    sent = 0;
    if (!isWithinUploadLimit(file.size, limit)) return;

    const abort = new AbortController();
    controller = abort;
    try {
      const object = await media.upload(file, 'AVATAR', {
        idempotencyKey: crypto.randomUUID(),
        signal: abort.signal,
        onProgress: (moved) => (sent = moved),
      });
      await actor.setPicture(object.id);
      toast.show({ text: t('app.profile.picture_set'), tone: 'success' });
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
      await actor.removePicture();
      toast.show({ text: t('app.profile.picture_removed'), tone: 'success' });
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<Stack gap="100">
  <UploadField
    label={t('app.profile.picture')}
    hint={t('app.profile.picture_hint', { limit: formatBytes(AVATAR_LIMIT, messages.locale) })}
    chooseLabel={hasPicture ? t('app.profile.picture_replace') : t('app.profile.picture_choose')}
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

  {#if hasPicture}
    <div>
      <Button size="sm" tone="secondary" onclick={() => void remove()}>
        {t('app.profile.picture_remove')}
      </Button>
    </div>
  {/if}
</Stack>

<style>
  .failure { margin: 0; color: var(--text-danger); }
</style>
