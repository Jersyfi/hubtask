<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A page's cover (E-18, scene pcover): over the content's width, never under the title's words;
  // a writer changes it in a picker of tabs - the design system's gradients, the shipped gallery, an
  // upload, and an image library where the workspace offers one - moves the picture up or down, or
  // takes it away. The library is asked by the server alone; its picture is uploaded as any other,
  // and who made it is named at the cover's foot (UC-DOC-16 check 5).
  import { Button, IconButton, SearchField, UploadField } from '@hubtask/design-system/components';
  import type { CoverImageSearch } from '@hubtask/sync-engine';
  import { media } from '../data/media.svelte.ts';
  import { pages } from '../data/pages.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { GALLERY, GRADIENTS, coverBackground, draggedPosition, type PageCover } from './covers.ts';

  interface Props {
    pageId: string;
    cover: PageCover;
    isWritable: boolean;
    /** Writes the cover asked for; null takes it away. */
    onchange: (cover: PageCover | null) => Promise<unknown>;
    /** The picker opens with the cover, when the writer has just asked for one. */
    isPicking?: boolean;
  }

  let { pageId, cover, isWritable, onchange, isPicking = $bindable(false) }: Props = $props();

  type Tab = 'colours' | 'gallery' | 'upload' | 'library';
  let isOffered = $state(false);
  const tabs = $derived<readonly Tab[]>(isOffered ? ['colours', 'gallery', 'upload', 'library'] : ['colours', 'gallery', 'upload']);
  let tab = $state<Tab>('colours');
  let query = $state('');
  let found = $state<CoverImageSearch['pictures']>([]);
  let isSearching = $state(false);
  let hasSearched = $state(false);
  let taking = $state<string | undefined>(undefined);

  // Whether the workspace offers a library, asked once the picker opens; the answer asks nothing outside.
  $effect(() => {
    if (!isPicking || !isWritable) return;
    void pages.searchCoverImages(pageId, '').then((answer) => (isOffered = answer.offered), () => (isOffered = false));
  });

  async function search() {
    if (!query.trim()) return;
    isSearching = true;
    try {
      found = (await pages.searchCoverImages(pageId, query)).pictures;
      hasSearched = true;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isSearching = false;
    }
  }

  async function take(pictureId: string) {
    taking = pictureId;
    try {
      const { file, credit } = await pages.fetchCoverImage(pageId, pictureId);
      await upload(file, credit);
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      taking = undefined;
    }
  }
  let isMoving = $state(false);
  let moved = $state<number | undefined>(undefined);
  let isUploading = $state(false);
  let band = $state<HTMLElement | undefined>(undefined);

  $effect(() => {
    if (cover.source === 'IMAGE') media.resolve([cover.key]);
  });
  const imageUrl = $derived(cover.source === 'IMAGE' ? media.coverUrl(cover.key, Date.now()) : undefined);
  const shown = $derived({ ...cover, position: moved ?? cover.position });
  const background = $derived(coverBackground(shown, imageUrl ?? undefined));

  async function choose(next: PageCover) {
    await onchange(next);
  }

  async function upload(file: File, credit?: string) {
    isUploading = true;
    try {
      const object = await media.upload(file, 'COVER', { idempotencyKey: crypto.randomUUID() });
      await pages.attachMedia(pageId, object.id);
      await onchange({ source: 'IMAGE', key: object.id, position: 50, ...(credit ? { credit } : {}) });
      isPicking = false;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isUploading = false;
    }
  }

  // Moving: the picture follows the pointer or the arrow keys; *Done* writes where it stands.
  let dragFrom: { y: number; position: number } | undefined;
  function down(event: PointerEvent) {
    if (!isMoving) return;
    (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
    dragFrom = { y: event.clientY, position: moved ?? cover.position };
  }
  function drag(event: PointerEvent) {
    if (!dragFrom || !band) return;
    moved = draggedPosition(dragFrom.position, event.clientY - dragFrom.y, band.clientHeight);
  }
  function key(event: KeyboardEvent) {
    if (!isMoving || (event.key !== 'ArrowUp' && event.key !== 'ArrowDown')) return;
    event.preventDefault();
    moved = draggedPosition(moved ?? cover.position, event.key === 'ArrowUp' ? 10 : -10, 100);
  }
  async function doneMoving() {
    const position = moved;
    isMoving = false;
    if (position !== undefined && position !== cover.position) await onchange({ ...cover, position });
    moved = undefined;
  }
</script>

<div class="cover" class:moving={isMoving} class:picking={isPicking} data-page-cover={`${cover.source}:${cover.key}`}>
  <!-- While moving, the picture is a vertical slider of its position; otherwise an image. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
  <div
    class="picture"
    bind:this={band}
    style:background={background}
    role={isMoving ? 'slider' : 'img'}
    aria-label={t('app.pages.cover')}
    aria-valuenow={isMoving ? shown.position : undefined}
    aria-valuemin={isMoving ? 0 : undefined}
    aria-valuemax={isMoving ? 100 : undefined}
    aria-orientation={isMoving ? 'vertical' : undefined}
    tabindex={isMoving ? 0 : -1}
    onpointerdown={down}
    onpointermove={drag}
    onpointerup={() => (dragFrom = undefined)}
    onkeydown={key}
  ></div>
  {#if cover.source === 'IMAGE' && cover.credit}
    <span class="credit" data-page-cover-credit>{t('app.pages.cover_credit', { name: cover.credit })}</span>
  {/if}
  {#if isWritable}
    <div class="acts">
      {#if isMoving}
        <span class="hint">{t('app.pages.cover_move_hint')}</span>
        <Button size="sm" tone="primary" onclick={doneMoving}>{t('app.pages.cover_move_done')}</Button>
      {:else}
        <Button size="sm" tone="secondary" icon="image" aria-expanded={isPicking} data-page-cover-change onclick={() => (isPicking = !isPicking)}>{t('app.pages.cover_change')}</Button>
        {#if cover.source === 'IMAGE'}
          <Button size="sm" tone="secondary" icon="arrow-up-down" onclick={() => { isPicking = false; isMoving = true; band?.focus(); }}>{t('app.pages.cover_move')}</Button>
        {/if}
        <IconButton size="sm" tone="secondary" icon="x" label={t('app.pages.cover_remove')} data-page-cover-remove onclick={() => onchange(null)} />
      {/if}
    </div>
  {/if}
  {#if isPicking && isWritable}
    <section class="picker" aria-label={t('app.pages.cover_picker')} data-page-cover-picker>
      <div class="tabs" role="tablist" aria-label={t('app.pages.cover_picker')}>
        {#each tabs as each (each)}
          <button type="button" role="tab" aria-selected={tab === each} class:active={tab === each} onclick={() => (tab = each)}>{t(`app.pages.cover_tab_${each}`)}</button>
        {/each}
      </div>
      <div role="tabpanel">
        {#if tab === 'colours'}
          <p class="note">{t('app.pages.cover_colours_note')}</p>
          <div class="swatches">
            {#each Object.keys(GRADIENTS) as key (key)}
              <button type="button" class="swatch" aria-pressed={cover.source === 'GRADIENT' && cover.key === key} aria-label={key} data-cover-key={key}
                style:background={coverBackground({ source: 'GRADIENT', key, position: 50 })} onclick={() => choose({ source: 'GRADIENT', key, position: 50 })}></button>
            {/each}
          </div>
        {:else if tab === 'gallery'}
          <p class="note">{t('app.pages.cover_gallery_note')}</p>
          <div class="swatches">
            {#each Object.keys(GALLERY) as key (key)}
              <button type="button" class="swatch" aria-pressed={cover.source === 'GALLERY' && cover.key === key} aria-label={key} data-cover-key={key}
                style:background={coverBackground({ source: 'GALLERY', key, position: 50 })} onclick={() => choose({ source: 'GALLERY', key, position: 50 })}></button>
            {/each}
          </div>
        {:else if tab === 'library'}
          <p class="note">{t('app.pages.cover_library_note')}</p>
          <form class="search" onsubmit={(event) => { event.preventDefault(); void search(); }}>
            <SearchField label={t('app.pages.cover_library_search')} clearLabel={t('app.search.clear')} bind:value={query} maxlength={100} />
          </form>
          {#if isSearching}
            <p class="note" role="status">{t('app.pages.cover_library_searching')}</p>
          {:else if hasSearched && found.length === 0}
            <p class="note" role="status">{t('app.pages.cover_library_none')}</p>
          {:else}
            <div class="swatches pictures" data-cover-library>
              {#each found as picture (picture.id)}
                <button type="button" class="swatch" aria-label={t('app.pages.cover_library_by', { name: picture.creator })} title={picture.creator}
                  aria-busy={taking === picture.id} disabled={taking !== undefined}
                  style:background-image={`url(${picture.preview})`} onclick={() => void take(picture.id)}></button>
              {/each}
            </div>
          {/if}
          {#if taking}<p class="note" role="status">{t('app.pages.cover_library_taking')}</p>{/if}
        {:else}
          <UploadField label={t('app.pages.cover_upload')} hint={t('app.pages.cover_upload_note')} chooseLabel={t('app.pages.cover_upload_choose')} accept="image/png,image/jpeg,image/webp" disabledReason={isUploading ? t('app.pages.cover_uploading') : undefined} onChoose={(file) => void upload(file)} />
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .cover { position: relative; margin-block-end: var(--sp-200); }
  .picture { block-size: var(--sp-1600); border-radius: var(--r-lg); background-color: var(--bg-surface-sunken); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width >= 600px) { .picture { block-size: calc(var(--sp-1600) * 1.25); } }
  .moving .picture { cursor: grab; outline: var(--bw-ring) dashed var(--focus-ring); touch-action: none; }
  .acts { position: absolute; inset-block-start: var(--sp-150); inset-inline-end: var(--sp-150); display: flex; align-items: center; gap: var(--sp-050); }
  /* While reading (scene pread) the acts do not stand on the picture: with a pointer they show where
     the cover is pointed at, reached by the keyboard, picked or moved; a touch screen always shows them. */
  @media (hover: hover) {
    .cover:not(:hover, :focus-within, .moving, .picking) .acts { opacity: 0; }
  }
  .hint { padding: var(--sp-050) var(--sp-100); border-radius: var(--r-md); background: var(--bg-raised); font-size: var(--fs-075); }
  .picker {
    position: absolute; z-index: 2; inset-block-start: calc(100% - var(--sp-600)); inset-inline-end: 0;
    display: grid; gap: var(--sp-150); inline-size: min(100%, calc(var(--sp-1600) * 3.25)); padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-raised); box-shadow: var(--shadow-overlay);
  }
  .tabs { display: grid; grid-auto-flow: column; grid-auto-columns: 1fr; padding: var(--sp-025); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .tabs button { padding: var(--sp-050) var(--sp-100); border: 0; border-radius: var(--r-md); background: none; color: var(--text-secondary); font: inherit; font-size: var(--fs-090); cursor: pointer; }
  .tabs button.active { background: var(--bg-raised); color: var(--text-primary); font-weight: var(--fw-semibold); }
  .tabs button:focus-visible, .swatch:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .swatches { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: var(--sp-100); margin-block-start: var(--sp-100); }
  .swatch { aspect-ratio: 3 / 2; border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); cursor: pointer; }
  .pictures { grid-template-columns: repeat(4, minmax(0, 1fr)); }
  .pictures .swatch { background-size: cover; background-position: center; }
  .credit {
    position: absolute; inset-block-end: var(--sp-100); inset-inline-start: var(--sp-150); padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm); background: var(--bg-raised); color: var(--text-secondary); font-size: var(--fs-075);
  }
  .swatch[aria-pressed='true'] { outline: var(--bw-ring) solid var(--accent-primary); outline-offset: var(--bw-hairline); }

  /* A thumb's target (§7): where the pointer is coarse or the window a phone's, the control's press
     reaches layout.target, by its own height or beyond its drawn box. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .tabs button { min-block-size: var(--layout-target); }
    .swatches { grid-template-columns: repeat(4, minmax(0, 1fr)); }
  }
</style>
