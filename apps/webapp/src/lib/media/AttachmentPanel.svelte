<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The files on an entry.
  //
  // **An attachment is a download, never an inline render.** T-11 puts uploaded files on a
  // sandboxed origin and serves them with `Content-Disposition: attachment`, and a client that
  // embedded one — a PDF in an iframe, an image beside its name — would undo that on its own
  // screen. So a row is a name, a type and a size, and pressing it fetches.
  //
  // **The list mints no download target.** `GET /items/{id}/attachments` answers media records;
  // a URL comes from `GET /media/{id}` and expires. Twenty rows are therefore one read and no
  // capabilities, and the target is minted when somebody actually asks for the file.
  //
  // **Detaching drops a reference and says exactly that.** The bytes are shared, the reference
  // count is what decides, and the reconciliation job removes an object once nothing points at it.
  // Promising the file is gone would be promising something this call did not do.

  import { untrack } from 'svelte';

  import {
    Button,
    CapabilityGate,
    ListRow,
    LoadMore,
    Stack,
    UploadField,
  } from '@hubtask/design-system/components';
  import type { MediaObject, MediaPage, WorkItem } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { attachmentsPath, media } from '../data/media.svelte.ts';
  import { acceptFor, isReady, isWithinUploadLimit, uploadLimitOf } from '../data/media.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';

  const { item }: { item: WorkItem } = $props();

  const capability = $derived(supports(item.type, 'ATTACHMENTS'));
  const limit = $derived(uploadLimitOf(manifest.value));

  // The path is taken once, as every other subscription here takes one: the view is rebuilt per
  // entry, so a path that re-derived would resubscribe on every unrelated change.
  const page = resource<MediaPage>({ path: untrack(() => attachmentsPath(item.id)) });
  const rows = $derived(page.state.status === 'ready' ? (page.state.data.data ?? []) : []);
  const hasMore = $derived(page.state.status === 'ready' && (page.state.data.page?.has_more ?? false));
  const readFailure = $derived(
    page.state.status === 'failed' ? renderProblem(page.state.error, messages) : undefined,
  );

  let chosen = $state<File | undefined>(undefined);
  let sent = $state(0);
  let controller = $state<AbortController | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let notice = $state<string | undefined>(undefined);

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

  /** The second line of a row: what it is and how big, in the reader's units. */
  function detailOf(object: MediaObject): string {
    const size = formatBytes(object.size, messages.locale);
    return isReady(object) ? `${object.content_type} · ${size}` : `${t('app.media.pending')} · ${size}`;
  }

  async function choose(file: File) {
    chosen = file;
    failure = undefined;
    notice = undefined;
    sent = 0;
    if (!isWithinUploadLimit(file.size, limit)) return;

    const abort = new AbortController();
    controller = abort;
    try {
      const object = await media.upload(file, 'ATTACHMENT', {
        idempotencyKey: crypto.randomUUID(),
        signal: abort.signal,
        onProgress: (moved) => (sent = moved),
      });
      await media.attach(item.id, object.id);
      chosen = undefined;
    } catch (error) {
      failure = abort.signal.aborted
        ? t('app.media.cancelled')
        : renderProblem(error as never, messages).message;
    } finally {
      controller = undefined;
    }
  }

  async function download(object: MediaObject) {
    failure = undefined;
    notice = undefined;
    try {
      const url = await media.downloadUrl(object.id, Date.now());
      // No target means the object is not READY, or this reader may not have it. Following
      // nothing would land on a signature error, which says nothing about either.
      if (!url) {
        failure = t('app.media.download_unavailable');
        return;
      }
      platform.openDownload(url);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  async function detach(object: MediaObject) {
    failure = undefined;
    notice = undefined;
    try {
      await media.detach(item.id, object.id);
      notice = t('app.media.detached');
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
    {#if readFailure}<p class="failure">{readFailure.message}</p>{/if}

    {#if rows.length === 0}
      <p class="quiet">{t('app.media.attachments_none')}</p>
    {:else}
      <ul class="files">
        {#each rows as object (object.id)}
          <li>
            <ListRow>
              {#snippet trailing()}
                <div class="actions">
                  <Button size="sm" tone="secondary" onclick={() => void download(object)}>
                    {t('app.media.download')}
                  </Button>
                  <Button size="sm" tone="secondary" onclick={() => void detach(object)}>
                    {t('app.media.detach')}
                  </Button>
                </div>
              {/snippet}
              <span class="name">{object.file_name ?? t('app.media.unnamed')}</span>
              <span class="detail">{detailOf(object)}</span>
            </ListRow>
          </li>
        {/each}
      </ul>
      {#if hasMore}
        <LoadMore
          label={t('app.media.more')}
          arrivedLabel={t('app.media.arrived', { count: rows.length })}
          onLoadMore={() => page.loadMore()}
        />
      {/if}
    {/if}

    <UploadField
      label={t('app.media.attachments')}
      hint={t('app.media.attach_hint')}
      chooseLabel={t('app.media.attach_choose')}
      dropLabel={t('app.media.drop')}
      cancelLabel={t('app.media.cancel')}
      accept={acceptFor('ATTACHMENT')}
      file={chosen ?? null}
      {sizeLabel}
      overLimitLabel={overLimit ? t('app.media.over_limit') : undefined}
      progress={isUploading ? percent : undefined}
      progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
      onChoose={(file) => void choose(file)}
      onCancel={() => controller?.abort()}
    />

    {#if notice}<p class="quiet">{notice}</p>{/if}
    {#if failure}<p class="failure">{failure}</p>{/if}
  </Stack>
</CapabilityGate>

<style>
  .files { margin: 0; padding: 0; list-style: none; }

  .name { display: block; overflow-wrap: anywhere; }

  .detail { display: block; color: var(--text-secondary); font-size: var(--fs-075); }

  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
