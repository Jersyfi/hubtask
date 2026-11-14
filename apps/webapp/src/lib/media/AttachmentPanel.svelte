<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The files on an entry.
  //
  // **An attachment is never opened as a document.** T-11 serves the bytes as a download from a
  // sandboxed answer; pressing a file opens the file viewer (F-1), which draws it itself or hands
  // the browser only the decoders the server allows (`files/viewing.ts`). A card's picture is the
  // viewer's own rendering (ET-4).
  //
  // **The list mints no download target.** `GET /items/{id}/attachments` answers media records;
  // a URL comes from `GET /media/{id}` and expires. Twenty rows are therefore one read and no
  // capabilities, and the target is minted when somebody actually asks for the file.
  //
  // **Detaching drops a reference and says exactly that.** The bytes are shared, the reference
  // count is what decides, and the reconciliation job removes an object once nothing points at it.
  // Promising the file is gone would be promising something this call did not do.

  import { untrack, type Snippet } from 'svelte';

  import {
    Icon,
    IconButton,
    LabelChip,
    ListRow,
    LoadMore,
    Stack,
    UploadField,
  } from '@hubtask/design-system/components';
  import type { MediaObject, MediaPage, WorkItem } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { attachmentsPath, ciFilesPath, media } from '../data/media.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { acceptFor, fileKindOf, isReady, isWithinUploadLimit, keepsOffline, uploadLimitOf } from '../data/media.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatBytes, formatFileSize } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import FilePreview from '../files/FilePreview.svelte';
  import { viewer } from '../files/viewer.svelte.ts';
  import { renderProblem } from '../problem.ts';

  /**
   * `isChangeable`: whether the reader may change the entry. Reading and downloading are anybody's
   * who reads it; attaching and detaching are a change the server refuses otherwise (P-05).
   */
  interface Props {
    /** The entry the files are on; or `ci`, the inventory object (UC-AST-01 check 12). */
    item?: WorkItem;
    ci?: { readonly id: string; readonly name: string };
    isChangeable: boolean;
    /** A grid of cards or a list of rows (ET-4); the entry keeps the reader's choice. */
    view?: 'grid' | 'list';
    /** How many rows to draw: the entry's section folds its files (ET-3); absent, all of them. */
    shownRows?: number;
    /** The linked files' cells, drawn first in the same grid or list (ET-4), and how many they are. */
    lead?: Snippet;
    leadCount?: number;
    /** The entry's key ("TAMA-1"), for the viewer's "2 of 9 · on TAMA-1". */
    ownerKey?: string;
  }

  const { item, ci, isChangeable, view = 'list', shownRows, lead, leadCount = 0, ownerKey }: Props = $props();

  /** Whose files these are: an entry's, kept offline where the app holds files, or an object's, online only. */
  const holder = $derived(
    item
      ? { path: attachmentsPath(item.id), title: item.title, attach: (id: string) => media.attach(item.id, id), detach: (id: string) => media.detach(item.id, id) }
      : { path: ciFilesPath(ci!.id), title: ci!.name, attach: (id: string) => media.attachToCi(ci!.id, id), detach: (id: string) => media.detachFromCi(ci!.id, id) },
  );

  /**
   * A card's picture is its kind - a mark and the name's ending - never the file's content: an
   * attachment is not rendered inline (T-11 above); a preview is the file viewer's (§6.13 F-2).
   */
  function kindOf(object: MediaObject): { icon: 'image' | 'file-spreadsheet' | 'file-archive' | 'file-text'; colour: string } {
    const type = object.content_type ?? '';
    if (type.startsWith('image/')) return { icon: 'image', colour: 'violet' };
    if (type.includes('pdf')) return { icon: 'file-text', colour: 'red' };
    if (type.includes('sheet') || type.includes('excel') || type.includes('csv')) return { icon: 'file-spreadsheet', colour: 'green' };
    if (type.includes('zip') || type.includes('compressed') || type.includes('tar')) return { icon: 'file-archive', colour: 'amber' };
    return { icon: 'file-text', colour: 'blue' };
  }

  /** The name's ending as the card's tag: *PDF*, *PNG*; none where the name has no ending. */
  function endingOf(object: MediaObject): string | undefined {
    const match = /\.([A-Za-z0-9]{1,5})$/.exec(object.file_name ?? '');
    return match?.[1]?.toUpperCase();
  }

  /** A file dropped anywhere on the entry comes here, one after another (ET-4). */
  export async function attach(dropped: readonly File[]): Promise<void> {
    for (const file of dropped) await choose(file);
  }

  const limit = $derived(uploadLimitOf(manifest.value));

  // The path is taken once, as every other subscription here takes one: the view is rebuilt per
  // entry, so a path that re-derived would resubscribe on every unrelated change.
  const page = resource<MediaPage>({ path: untrack(() => holder.path) });
  const rows = $derived(page.state.status === 'ready' ? (page.state.data.data ?? []) : []);

  /** Opens the viewer on one file with every file of the entry beside it (F-1). */
  function openFile(object: MediaObject) {
    viewer.open({
      files: rows,
      index: rows.findIndex((each) => each.id === object.id),
      owner: { key: ownerKey, title: holder.title },
    });
  }

  // An address naming a file (`?file=`, the viewer's *link*) opens it once the files are read.
  let isLinkedOpened = false;
  $effect(() => {
    if (isLinkedOpened || rows.length === 0) return;
    const named = new URL(globalThis.location.href).searchParams.get('file');
    const linked = named ? rows.find((each) => each.id === named) : undefined;
    isLinkedOpened = true;
    if (linked) untrack(() => openFile(linked));
  });
  let previewed = $state<Record<string, boolean>>({});
  const hasMore = $derived(page.state.status === 'ready' && (page.state.data.page?.has_more ?? false));
  /** What the last press of *Show more* brought; empty until asked, as the entry's history is. */
  let arrived = $state<string | undefined>(undefined);

  /** Why the last press of *Show more* brought nothing; cleared by the next press. */
  let moreFailed = $state<string | undefined>(undefined);

  async function more(): Promise<void> {
    moreFailed = undefined;
    try {
      await page.loadMore();
      arrived = t('app.media.arrived', { count: rows.length });
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    }
  }
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

  /** The second line of a row: what kind of file it is and how big, in the reader's words and units. */
  function detailOf(object: MediaObject): string {
    const size = formatFileSize(object.size, messages.locale, t);
    if (isReady(object)) return `${t(`app.media.kind.${fileKindOf(object.content_type, object.file_name)}`)} · ${size}`;
    // Held for the installation's scanner, or found malicious since: neither is downloaded.
    if (object.status === 'PENDING_SCAN') return `${t('app.media.held')} · ${size}`;
    if (object.status === 'INFECTED') return t('app.media.infected');
    if (object.status === 'UNSCANNABLE') return `${t('app.media.unscannable')} · ${size}`;
    return `${t('app.media.pending')} · ${size}`;
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
      await holder.attach(object.id);
      chosen = undefined;
    } catch (error) {
      let cause = error;
      // Without the server, the installed apps keep the file and send it later, like any change.
      if (item && !abort.signal.aborted && keepsOffline(error, engine.holdsFiles)) {
        try {
          await media.keep(item.id, file);
          chosen = undefined;
          return;
        } catch (refused) {
          cause = refused;
        }
      }
      failure = abort.signal.aborted
        ? t('app.media.cancelled')
        : renderProblem(cause as never, messages).message;
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
      await holder.detach(object.id);
      notice = t('app.media.detached');
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

{#snippet upload(variant: 'button' | 'tile')}
      <UploadField
        {variant}
        label={t('app.media.attachments')}
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
{/snippet}

<!-- No gate here: the entry page draws this panel only where the type carries the capability
     (`carries`, ItemView), so a refusal could never reach it - a refused capability is an absent
     row, not a dead one (design-system.md §11.8). -->
  <Stack gap="150">
    {#if readFailure}<p class="failure" role="alert">{readFailure.message}</p>{/if}

    {#if rows.length === 0 && leadCount === 0}
      {#if !isChangeable}<p class="quiet">{t('app.media.attachments_none')}</p>{/if}
    {:else}
      <ul class="files" data-view={view}>
        {@render lead?.()}
        {#each rows.slice(0, shownRows ?? rows.length) as object (object.id)}
          <li data-attachment={object.id}>
            {#if view === 'grid'}
              {@const kind = kindOf(object)}
              {@const ending = endingOf(object)}
              <div class="card">
                <button type="button" class="picture" tabindex="-1" aria-hidden="true" onclick={() => openFile(object)}>
                  {#if !previewed[object.id]}<Icon name={kind.icon} />{/if}
                  {#if isReady(object)}<FilePreview file={object} onshown={(shown) => { if (previewed[object.id] !== shown) previewed = { ...previewed, [object.id]: shown }; }} />{/if}
                  {#if ending}<span class="ending"><LabelChip name={ending} colorToken={kind.colour} /></span>{/if}
                </button>
                <button type="button" class="name card-name open" title={object.file_name ?? undefined} onclick={() => openFile(object)}>{object.file_name ?? t('app.media.unnamed')}</button>
                <span class="detail">{detailOf(object)}</span>
                <div class="actions card-acts">
                  <IconButton icon="download" size="sm" tone="subtle" label={t('app.media.download')} onclick={() => void download(object)} />
                  {#if isChangeable}
                    <IconButton icon="x" size="sm" tone="subtle" label={t('app.media.detach')} onclick={() => void detach(object)} />
                  {/if}
                </div>
              </div>
            {:else}
            <ListRow>
              {#snippet trailing()}
                <!-- One row a file (ET-4): its acts as icons, named for a screen reader. -->
                <div class="actions">
                  <IconButton icon="download" size="sm" tone="subtle" label={t('app.media.download')} onclick={() => void download(object)} />
                  {#if isChangeable}
                    <IconButton icon="x" size="sm" tone="subtle" label={t('app.media.detach')} onclick={() => void detach(object)} />
                  {/if}
                </div>
              {/snippet}
              <button type="button" class="name open" onclick={() => openFile(object)}>{object.file_name ?? t('app.media.unnamed')}</button>
              <span class="detail">{detailOf(object)}</span>
            </ListRow>
            {/if}
          </li>
        {/each}
        {#if isChangeable && view === 'grid'}<li class="add">{@render upload('tile')}</li>{/if}
      </ul>
      {#if hasMore || arrived}
        <LoadMore
          label={t('app.media.more')}
          {hasMore}
          arrivedLabel={arrived}
          failedLabel={moreFailed}
          onLoadMore={() => void more()}
        />
      {/if}
    {/if}

    <!-- In a grid the drop area is the last card (the scene's dashed tile); otherwise a button. -->
    {#if isChangeable && (view !== 'grid' || (rows.length === 0 && leadCount === 0))}{@render upload('button')}{/if}

    {#if notice}<p class="quiet">{notice}</p>{/if}
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  </Stack>

<style>
  .files { margin: 0; padding: 0; list-style: none; }
  /* The scene's grid: four across on a desktop's main column, fewer as it narrows. */
  .files[data-view='grid'] { display: grid; grid-template-columns: repeat(auto-fill, minmax(calc(var(--sp-1600) + var(--sp-400)), 1fr)); gap: var(--sp-150); }
  .card { position: relative; display: grid; gap: var(--sp-025); padding-block-end: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface); overflow: hidden; }
  .card > .name, .card > .detail { padding-inline: var(--sp-100); }
  .picture { position: relative; display: grid; place-items: center; block-size: var(--sp-1000); margin-block-end: var(--sp-050); background: var(--bg-canvas); color: var(--text-subtle); }
  .ending { position: absolute; inset-block-start: var(--sp-050); inset-inline-start: var(--sp-050); }
  .card-name { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-weight: var(--fw-medium); }
  .card-acts { position: absolute; inset-block-start: var(--sp-050); inset-inline-end: var(--sp-050); border-radius: var(--r-md); background: var(--bg-surface); opacity: 0; }
  .card:hover .card-acts, .card:focus-within .card-acts { opacity: 1; }

  .name { display: block; overflow-wrap: anywhere; }
  .open { padding: 0; border: 0; background: none; color: inherit; font-family: inherit; font-size: inherit; text-align: start; cursor: pointer; }
  .open:hover { text-decoration: underline; }
  button.picture { inline-size: 100%; padding: 0; border: 0; cursor: pointer; overflow: hidden; }

  .detail { display: block; color: var(--text-secondary); font-size: var(--fs-075); }

  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .add { display: grid; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    button.name { min-block-size: var(--layout-target); }
  }
</style>
