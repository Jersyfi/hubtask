<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One of the workspace's three pictures (UC-INS-22 checks 1, 2 and 4): its picture on the surface
  // it is for, *Replace* and *Remove*; a drop area while there is none. One press is one write, as
  // a desk's logo is - choosing a file uploads, confirms and sets it; nothing waits for the save bar.
  //
  // **Honest about the upload.** A file the browser names as another type, or past the limit, is
  // refused before a byte leaves; the server's refusal is shown here, at the picture it concerns,
  // with the focus moved to the control it describes; nothing says "set" before the server answered.
  // *Remove* returns the focus to this row's upload, which is what stands in its place.

  import { tick } from 'svelte';

  import { Button, Icon, UploadField } from '@hubtask/design-system/components';
  import type { WorkspaceHeader, WorkspacePicture } from '@hubtask/sync-engine';

  import { branding, lookOf, type PictureLook, type PictureRead } from '../data/branding.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { PICTURE_TYPES, refusalOf, type Surface } from './branding.ts';

  /** A line under the picture: whether it reads, or that another picture stands in. */
  export interface PictureLine {
    readonly tone: 'reads' | 'hard' | 'quiet';
    readonly text: string;
  }

  interface Props {
    picture: WorkspacePicture;
    /** The row's name, for the upload's own label in the accessibility tree. */
    label: string;
    nameId: string;
    /**
     * Where the stored picture stands, where there is one: its bytes being read, drawn, or not
     * drawable here. Stored is stored - *Replace* and *Remove* stand whatever became of the bytes.
     */
    read?: PictureRead;
    /** Another picture drawn in this one's place (the light logo on dark), with what says so. */
    standIn?: { readonly look: PictureLook; readonly text: string };
    line?: PictureLine;
    limit: number;
    /** The header as stored, for what a removal says. */
    header: WorkspaceHeader;
    /** Whether this is the only picture the workspace has. */
    isLast: boolean;
    disabledReason?: string;
    /** A file chosen and going: its local look drives the preview while it travels. */
    onstage: (look: PictureLook | undefined) => void;
  }

  const { picture, label, nameId, read, standIn, line, limit, header, isLast, disabledReason, onstage }: Props = $props();

  const has = $derived(read !== undefined);
  const surfaces = $derived<Surface[]>(picture === 'SYMBOL' ? ['light', 'dark'] : picture === 'LOGO_DARK' ? ['dark'] : ['light']);
  const kind = $derived(picture === 'SYMBOL' ? 'SYMBOL' : 'LOGO');

  let root = $state<HTMLElement | undefined>(undefined);
  let chosen = $state<File | undefined>(undefined);
  let sent = $state(0);
  let controller = $state<AbortController | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isRemoving = $state(false);

  const isUploading = $derived(controller !== undefined);
  const percent = $derived(chosen && chosen.size > 0 ? Math.floor((sent / chosen.size) * 100) : 0);
  const sizeLabel = $derived(
    chosen === undefined ? undefined : t('app.media.size_of_limit', { size: formatBytes(chosen.size, messages.locale), limit: formatBytes(limit, messages.locale) }),
  );

  /** The focus to this row's upload, after the row has drawn what stands now. */
  async function focusUpload(): Promise<void> {
    await tick();
    root?.querySelector<HTMLButtonElement>('[data-upload] button')?.focus();
  }

  async function choose(file: File): Promise<void> {
    chosen = file;
    failure = undefined;
    sent = 0;
    const refusal = refusalOf(file, limit);
    if (refusal) {
      failure = refusal === 'type' ? t('app.branding.wrong_type', { picture: kind }) : t('app.branding.too_large', { picture: kind, limit: formatBytes(limit, messages.locale) });
      await focusUpload();
      return;
    }

    const local = await lookOf(file);
    onstage(local);
    const abort = new AbortController();
    controller = abort;
    try {
      const object = await media.upload(file, 'WORKSPACE_LOGO', {
        idempotencyKey: crypto.randomUUID(),
        signal: abort.signal,
        onProgress: (moved) => (sent = moved),
      });
      await branding.setPicture(picture, object.id);
      branding.hold(object.id, local);
      toast.show({ text: t('app.branding.set', { picture }), tone: 'success' });
      chosen = undefined;
    } catch (error) {
      URL.revokeObjectURL(local.url);
      failure = abort.signal.aborted ? t('app.media.cancelled') : renderProblem(error as never, messages).message;
      void focusUpload();
    } finally {
      controller = undefined;
      onstage(undefined);
    }
  }

  async function remove(): Promise<void> {
    failure = undefined;
    chosen = undefined;
    isRemoving = true;
    try {
      const after = await branding.removePicture(picture);
      const isBack = isLast && header === 'LOGO' && after.header === 'PRODUCT';
      toast.show({ text: t(isBack ? 'app.branding.removed_back' : 'app.branding.removed', { picture }), tone: 'success' });
      isRefocusing = true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      await focusUpload();
    } finally {
      isRemoving = false;
    }
  }

  // The removal lands when the workspace is read again; the focus follows the upload that then stands.
  let isRefocusing = $state(false);
  $effect(() => {
    if (!isRefocusing || has) return;
    isRefocusing = false;
    void focusUpload();
  });
</script>

<div class="picture" role="group" aria-labelledby={nameId} data-picture={picture} bind:this={root}>
  {#if has && read}
    <div class="has">
      {#each surfaces as surface (surface)}
        <!-- The picture's box whatever its bytes do: a placeholder while they are read, the
             picture's mark where they cannot be drawn here (the line below says why). -->
        <span class="thumb" data-theme={surface} data-square={picture === 'SYMBOL' ? '' : undefined} data-read={read.status}>
          {#if read.status === 'ready'}<img src={read.look.url} alt="" />{:else if read.status === 'reading'}<span class="pending"></span>{:else}<Icon name="image" size="sm" />{/if}
        </span>
      {/each}
      <span class="upload" data-upload>
        <UploadField
          variant="button"
          size="sm"
          {label}
          chooseLabel={t('app.branding.replace')}
          accept={PICTURE_TYPES.join(',')}
          error={failure}
          progress={isUploading ? percent : undefined}
          progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
          cancelLabel={t('app.media.cancel')}
          {disabledReason}
          onChoose={(file) => void choose(file)}
          onCancel={() => controller?.abort()}
        />
      </span>
      <Button tone="subtle" size="sm" isBusy={isRemoving} busyLabel={t('app.branding.removing')} disabledReason={isUploading ? t('app.branding.uploading') : disabledReason} onclick={() => void remove()}>
        {t('app.branding.remove')}
      </Button>
    </div>
  {:else}
    {#if standIn}
      <div class="has">
        <span class="thumb" data-theme="dark"><img src={standIn.look.url} alt="" /></span>
        <span class="quiet">{standIn.text}</span>
      </div>
    {/if}
    <div class="upload target" data-upload>
      <UploadField
        size="sm"
        {label}
        chooseLabel={t(picture === 'SYMBOL' ? 'app.branding.choose_symbol' : 'app.branding.choose_logo')}
        dropLabel={t('app.media.drop')}
        accept={PICTURE_TYPES.join(',')}
        file={chosen ?? null}
        {sizeLabel}
        error={failure}
        progress={isUploading ? percent : undefined}
        progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
        cancelLabel={t('app.media.cancel')}
        {disabledReason}
        onChoose={(file) => void choose(file)}
        onCancel={() => controller?.abort()}
      />
    </div>
  {/if}
  <!-- A status, there from the start and filled when the picture is judged: it warns, never
       blocks, and says nothing again while the verdict stands. -->
  <p class="line" role="status" class:reads={line?.tone === 'reads'} class:hard={line?.tone === 'hard'} class:quiet={line?.tone === 'quiet'}>
    {#if line}
      {#if line.tone !== 'quiet'}<Icon name={line.tone === 'reads' ? 'circle-check' : 'triangle-alert'} size="sm" />{/if}
      <span>{line.text}</span>
    {/if}
  </p>
</div>

<style>
  /* No gap: the status line is there while empty, and an empty one takes no room. */
  .picture { display: flex; flex-direction: column; min-inline-size: 0; inline-size: 100%; }
  .picture > :global(* + *:not(:empty)) { margin-block-start: var(--sp-100); }

  .has { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .thumb {
    display: grid;
    box-sizing: border-box;
    place-items: center;
    inline-size: calc(var(--sp-600) * 3);
    block-size: var(--sp-600);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .thumb[data-square] { inline-size: var(--sp-600); }

  .thumb img { max-inline-size: 100%; max-block-size: 100%; object-fit: contain; }

  .thumb[data-read='unreadable'] { color: var(--text-subtle); }

  /* The bytes on their way: the box's inside, quiet; no motion, so nothing to reduce. */
  .pending { inline-size: 100%; block-size: 100%; border-radius: var(--r-sm); background: var(--bg-recess); }

  .quiet { color: var(--text-subtle); font-size: var(--fs-075); }

  /* The row's name is the upload's visible name; the field's own label stays for the tree only. */
  .target :global(.field > .label) {
    position: absolute;
    inline-size: var(--bw-hairline);
    block-size: var(--bw-hairline);
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }

  .line { display: flex; align-items: flex-start; gap: var(--sp-050); margin: 0; font-size: var(--fs-075); }
  .line :global(.icon) { flex: none; margin-block-start: var(--sp-025); }
  /* A sentence with its mark, not a status surface: it warns and never blocks. */
  .line.reads { color: var(--text-success); }
  .line.hard { color: var(--text-warning); }
  .line.quiet { color: var(--text-subtle); }
</style>
