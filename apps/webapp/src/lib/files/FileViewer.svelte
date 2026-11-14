<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one file viewer (F-1, scene `viewer`): full screen over everything - a modal dialog in the
  // top layer, so no z-index is chosen here -, the name and "2 of 9" with download, link and edit
  // on top, the details on the right, every file of the object at the foot; ← → page, Esc closes.
  //
  // **Nothing is opened as a document** (F-5): see `viewing.ts` for the two ways onto the screen.
  // Every drawing is elements and text - Markdown through the editor's parser, never `{@html}`.

  import type { DocNode } from '@hubtask/editor';
  import { Button, IconButton, Kbd, LabelChip } from '@hubtask/design-system/components';
  import { TransportError, type DocumentServer, type MediaObject } from '@hubtask/sync-engine';
  import type { PDFDocumentProxy } from 'pdfjs-dist';

  import { accounts } from '../data/accounts.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { media } from '../data/media.svelte.ts';
  import { transferTimeoutFor } from '../data/media.ts';
  import { formatFileSize } from '../i18n/bytes.ts';
  import { formatDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { documentServerPath } from './documentServer.svelte.ts';
  import { EDITOR_LAUNCH, launchEditor } from './launchEditor.ts';
  import { drawPage, openPdf } from './pdf.ts';
  import { viewer } from './viewer.svelte.ts';
  import { TEXT_LIMIT, isDrawnFromBytes, markColourOf, markOf, neighbour, officeShownAs, parseCsv, viewOf, zipListing, type ViewKind, type ZipEntry } from './viewing.ts';

  /** A conversion waits on the document server; its own deadline on the server is a minute. */
  const CONVERSION_TIMEOUT = 75_000;

  let dialog = $state<HTMLDialogElement>();
  let stage = $state<HTMLElement>();
  const opened = $derived(viewer.opened);
  const files = $derived(opened?.files ?? []);
  const file = $derived<MediaObject | undefined>(files[viewer.index]);
  const kind = $derived<ViewKind>(file ? viewOf(file) : 'card');
  // *Edit* only where the server says the document server takes this file for this reader (F-3),
  // and only once the launch is decided (`launchEditor.ts`).
  const isEditable = $derived(EDITOR_LAUNCH && file?.editable === true);
  // Whether an office file shows as the server's PDF: every member reads the choice.
  const server = resource<DocumentServer>(documentServerPath);
  const officeAs = $derived(officeShownAs(server.state.status === 'ready' ? server.state.data : undefined));

  /** What the stage draws, once the bytes or the URL arrived. */
  type Drawn =
    | { kind: 'loading' }
    | { kind: 'failed'; message: string }
    | { kind: 'url'; url: string }
    | { kind: 'svg'; url: string }
    | { kind: 'pdf'; document: PDFDocumentProxy; pages: number }
    | { kind: 'text'; text: string; isCut: boolean }
    | { kind: 'markdown'; doc: DocNode }
    | { kind: 'csv'; rows: string[][]; isCut: boolean }
    | { kind: 'zip'; entries: ZipEntry[]; total: number }
    | { kind: 'card' };
  let drawn = $state<Drawn>({ kind: 'loading' });
  let zoom = $state(1);
  let isDetailsOpen = $state(false);
  let notice = $state<string>();
  let failure = $state<string>();

  $effect(() => {
    if (opened && dialog && !dialog.open) dialog.showModal();
    if (!opened && dialog?.open) dialog.close();
  });

  // A new file: fetch what it needs, and let go of what the last one held.
  $effect(() => {
    const current = file;
    if (!current) return;
    const currentKind = kind;
    const asOffice = officeAs;
    let isCurrent = true;
    let made: string | undefined;
    zoom = 1;
    notice = undefined;
    failure = undefined;
    drawn = { kind: 'loading' };
    void (async () => {
      try {
        if (currentKind === 'office' && asOffice === 'pdf') {
          drawn = await converted(current);
          return;
        }
        if (currentKind === 'card' || currentKind === 'office') {
          drawn = { kind: 'card' };
          return;
        }
        const url = await media.downloadUrl(current.id, Date.now());
        if (!isCurrent) return;
        if (!url) {
          drawn = { kind: 'failed', message: t('app.media.download_unavailable') };
          return;
        }
        if (!isDrawnFromBytes(currentKind)) {
          drawn = { kind: 'url', url };
          return;
        }
        const bytes = await engine.receive({ url, timeoutMs: transferTimeoutFor(current.size) });
        if (!isCurrent) return;
        drawn = await draw(currentKind, bytes, (blobUrl) => (made = blobUrl));
      } catch (error) {
        if (isCurrent) drawn = { kind: 'failed', message: renderProblem(error as never, messages).message };
      }
    })();
    return () => {
      isCurrent = false;
      if (made) URL.revokeObjectURL(made);
      if (drawn.kind === 'pdf') void drawn.document.destroy();
    };
  });

  /** An office file as the PDF its document server makes; a server that converts nothing, the card. */
  async function converted(current: MediaObject): Promise<Drawn> {
    try {
      const answer = await engine.document(`/media/${encodeURIComponent(current.id)}:pdf`, {}, {
        accept: 'application/pdf', timeoutMs: Math.max(CONVERSION_TIMEOUT, transferTimeoutFor(current.size)),
      });
      const document = await openPdf(await answer.body.arrayBuffer());
      return { kind: 'pdf', document, pages: document.numPages };
    } catch (error) {
      if (error instanceof TransportError && error.detailCode === 'media.conversion_unavailable') return { kind: 'card' };
      throw error;
    }
  }

  async function draw(as: ViewKind, bytes: Blob, keep: (url: string) => void): Promise<Drawn> {
    if (as === 'svg') {
      // From the bytes, typed as SVG, into an <img>: an image document runs no script and loads
      // nothing, whatever the file holds.
      const url = URL.createObjectURL(new Blob([bytes], { type: 'image/svg+xml' }));
      keep(url);
      return { kind: 'svg', url };
    }
    if (as === 'pdf') {
      const document = await openPdf(await bytes.arrayBuffer());
      return { kind: 'pdf', document, pages: document.numPages };
    }
    if (as === 'zip') {
      const listing = zipListing(new Uint8Array(await bytes.arrayBuffer()));
      return listing ? { kind: 'zip', ...listing } : { kind: 'card' };
    }
    const isCut = bytes.size > TEXT_LIMIT;
    const text = await bytes.slice(0, TEXT_LIMIT).text();
    if (as === 'csv') return { kind: 'csv', ...parseCsv(text) };
    if (as === 'markdown') {
      const { fromMarkdown } = await import('@hubtask/editor');
      return { kind: 'markdown', doc: fromMarkdown(text) };
    }
    return { kind: 'text', text, isCut };
  }

  // The pages, drawn into their canvases once they are on the stage and whenever the zoom changes.
  let canvases = $state<HTMLCanvasElement[]>([]);
  $effect(() => {
    if (drawn.kind !== 'pdf' || !stage) return;
    const { document, pages } = drawn;
    const width = Math.min(stage.clientWidth - 32, 820) * zoom;
    for (let page = 1; page <= pages && page <= canvases.length; page++) {
      const canvas = canvases[page - 1];
      if (canvas) void drawPage(document, page, canvas, width);
    }
  });

  function go(step: -1 | 1) {
    if (files.length > 1) viewer.index = neighbour(viewer.index, files.length, step);
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.target instanceof HTMLElement && event.target.closest('input, textarea, [contenteditable]')) return;
    if (event.key === 'ArrowLeft') { event.preventDefault(); go(-1); }
    if (event.key === 'ArrowRight') { event.preventDefault(); go(1); }
  }

  // A swipe on a phone pages like ← →.
  let swipeFrom: number | undefined;
  const onpointerdown = (event: PointerEvent) => { if (event.pointerType === 'touch') swipeFrom = event.clientX; };
  function onpointerup(event: PointerEvent) {
    if (swipeFrom === undefined) return;
    const moved = event.clientX - swipeFrom;
    swipeFrom = undefined;
    if (Math.abs(moved) > 60) go(moved < 0 ? 1 : -1);
  }

  async function download() {
    if (!file) return;
    failure = undefined;
    try {
      const url = await media.downloadUrl(file.id, Date.now());
      if (url) platform.openDownload(url);
      else failure = t('app.media.download_unavailable');
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  /** The address of this view: the page it was opened on, with the file named. */
  async function copyLink() {
    if (!file) return;
    const address = new URL(globalThis.location.href);
    address.searchParams.set('file', file.id);
    try {
      await navigator.clipboard.writeText(address.href);
      notice = t('app.viewer.link_copied');
    } catch {
      notice = address.href;
    }
  }

  async function edit() {
    if (!file) return;
    failure = undefined;
    try {
      // How the session reaches the server is the launch the owner decides (ADR-0104).
      await launchEditor(file.id, () => {});
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  const zooms = [1, 1.5, 2, 0.75];
  const nextZoom = () => (zoom = zooms[(zooms.indexOf(zoom) + 1) % zooms.length] ?? 1);
  const canZoom = $derived(drawn.kind === 'pdf' || drawn.kind === 'url' && kind === 'image' || drawn.kind === 'svg');

  const kindWord = (each: MediaObject) => t(`app.viewer.kind.${viewOf(each)}`);
  $effect(() => { if (file) accounts.resolve([file.created_by]); });
</script>

{#snippet inline(nodes: DocNode[] | undefined)}
  {#each nodes ?? [] as node, i (i)}
    {#if node.type === 'text'}
      {@const marks = (node.marks ?? []).map((mark) => mark.type)}
      {@const href = String((node.marks ?? []).find((mark) => mark.type === 'link')?.attrs?.href ?? '')}
      {#if href && /^(https?:|mailto:)/i.test(href)}<a href={href} target="_blank" rel="noopener noreferrer nofollow">{node.text}</a>
      {:else if marks.includes('code')}<code>{node.text}</code>
      {:else if marks.includes('bold')}<strong>{node.text}</strong>
      {:else if marks.includes('italic')}<em>{node.text}</em>
      {:else}{node.text}{/if}
    {:else if node.type === 'hardBreak'}<br />
    {:else if node.type === 'image'}<span class="quiet">{String(node.attrs?.alt ?? '')}</span>
    {:else}{@render inline(node.content)}{/if}
  {/each}
{/snippet}

{#snippet blocks(nodes: DocNode[] | undefined)}
  {#each nodes ?? [] as node, i (i)}
    {#if node.type === 'heading'}
      <svelte:element this={`h${Math.min(Math.max(Number(node.attrs?.level ?? 2), 1), 6)}`}>{@render inline(node.content)}</svelte:element>
    {:else if node.type === 'paragraph'}<p>{@render inline(node.content)}</p>
    {:else if node.type === 'bulletList' || node.type === 'taskList'}<ul>{@render blocks(node.content)}</ul>
    {:else if node.type === 'orderedList'}<ol>{@render blocks(node.content)}</ol>
    {:else if node.type === 'listItem' || node.type === 'taskItem'}<li>{@render blocks(node.content)}</li>
    {:else if node.type === 'blockquote'}<blockquote>{@render blocks(node.content)}</blockquote>
    {:else if node.type === 'codeBlock'}<pre><code>{(node.content ?? []).map((each) => each.text ?? '').join('')}</code></pre>
    {:else if node.type === 'horizontalRule'}<hr />
    {:else if node.type === 'table'}
      <table><tbody>{#each node.content ?? [] as row, r (r)}<tr>{#each row.content ?? [] as cell, c (c)}{#if cell.type === 'tableHeader'}<th>{@render blocks(cell.content)}</th>{:else}<td>{@render blocks(cell.content)}</td>{/if}{/each}</tr>{/each}</tbody></table>
    {:else}{@render blocks(node.content)}{@render inline(node.content?.filter((each) => each.type === 'text'))}{/if}
  {/each}
{/snippet}

<dialog bind:this={dialog} class="viewer" aria-label={t('app.viewer.label')} onclose={() => viewer.close()} {onkeydown} data-file-viewer>
  {#if opened && file}
    <header class="top">
      <span class="phone-only"><IconButton icon="x" label={t('app.viewer.close')} onclick={() => viewer.close()} /></span>
      {#if markOf(file)}<span class="mark wide-only"><LabelChip name={markOf(file)} colorToken={markColourOf(file)} /></span>{/if}
      <h2 class="name" title={file.file_name ?? undefined}>{file.file_name ?? t('app.media.unnamed')}</h2>
      <span class="where wide-only" data-viewer-position>
        · {t('app.viewer.position', { n: String(viewer.index + 1), m: String(files.length) })}{#if opened.owner?.key || opened.owner?.title} · {t('app.viewer.on', { name: opened.owner?.key ?? opened.owner?.title ?? '' })}{/if}
      </span>
      <div class="acts">
        {#if canZoom}<span class="wide-only"><Button tone="secondary" size="sm" icon="search" onclick={nextZoom}>{t('app.viewer.zoom', { percent: String(Math.round(zoom * 100)) })}</Button></span>{/if}
        <span class="wide-only"><Button tone="secondary" size="sm" icon="download" onclick={() => void download()}>{t('app.media.download')}</Button></span>
        <span class="wide-only"><Button tone="secondary" size="sm" icon="link" onclick={() => void copyLink()}>{t('app.viewer.link')}</Button></span>
        {#if isEditable}<span class="wide-only" data-viewer-edit><Button tone="primary" size="sm" icon="pencil" onclick={() => void edit()}>{t('app.viewer.edit')}</Button></span>{/if}
        <span class="narrow-only"><IconButton icon="ellipsis" label={t('app.viewer.details')} onclick={() => (isDetailsOpen = !isDetailsOpen)} /></span>
        <span class="wide-only"><IconButton icon="x" label={t('app.viewer.close')} onclick={() => viewer.close()} /></span>
      </div>
    </header>

    <div class="body">
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <main class="stage" bind:this={stage} data-zoom={String(zoom)} data-kind={kind} {onpointerdown} {onpointerup}>
        {#if drawn.kind === 'loading'}
          <p class="quiet center" role="status">{t('app.viewer.loading')}</p>
        {:else if drawn.kind === 'failed'}
          <p class="failure center" role="alert">{drawn.message}</p>
        {:else if drawn.kind === 'url' && kind === 'image'}
          <img class="picture" src={drawn.url} alt={file.file_name ?? ''} />
        {:else if drawn.kind === 'url' && kind === 'video'}
          <!-- svelte-ignore a11y_media_has_caption -->
          <video class="picture" src={drawn.url} controls preload="metadata"></video>
        {:else if drawn.kind === 'url' && kind === 'audio'}
          <audio src={drawn.url} controls preload="metadata"></audio>
        {:else if drawn.kind === 'svg'}
          <img class="picture" src={drawn.url} alt={file.file_name ?? ''} />
        {:else if drawn.kind === 'pdf'}
          <div class="pages" data-viewer-pdf>
            {#each Array.from({ length: Math.min(drawn.pages, 50) }, (_, i) => i) as page (page)}
              <canvas bind:this={canvases[page]} class="sheet" aria-label={t('app.viewer.page_of', { n: String(page + 1), m: String(drawn.pages) })}></canvas>
            {/each}
            {#if drawn.pages > 50}<p class="quiet">{t('app.viewer.pages_cut', { count: '50' })}</p>{/if}
          </div>
        {:else if drawn.kind === 'text'}
          <pre class="sheet text">{drawn.text}</pre>
          {#if drawn.isCut}<p class="quiet">{t('app.viewer.text_cut')}</p>{/if}
        {:else if drawn.kind === 'markdown'}
          <article class="sheet prose" data-viewer-markdown>{@render blocks(drawn.doc.content)}</article>
        {:else if drawn.kind === 'csv'}
          <div class="sheet table-wrap"><table class="csv">
            {#if drawn.rows[0]}<thead><tr>{#each drawn.rows[0] as cell, c (c)}<th>{cell}</th>{/each}</tr></thead>{/if}
            <tbody>{#each drawn.rows.slice(1) as row, r (r)}<tr>{#each row as cell, c (c)}<td>{cell}</td>{/each}</tr>{/each}</tbody>
          </table></div>
          {#if drawn.isCut}<p class="quiet">{t('app.viewer.csv_cut', { count: String(drawn.rows.length) })}</p>{/if}
        {:else if drawn.kind === 'zip'}
          <div class="sheet listing">
            <p class="quiet">{t('app.viewer.zip_entries', { count: String(drawn.total) })}</p>
            <ul>{#each drawn.entries as entry, i (i)}<li><span>{entry.name}</span>{#if !entry.isFolder}<span class="quiet">{formatFileSize(entry.size, messages.locale, t)}</span>{/if}</li>{/each}</ul>
          </div>
        {:else}
          <div class="card" data-viewer-card>
            {#if markOf(file)}<LabelChip name={markOf(file)} colorToken={markColourOf(file)} />{/if}
            <strong>{file.file_name ?? t('app.media.unnamed')}</strong>
            <span class="quiet">{kindWord(file)} · {formatFileSize(file.size, messages.locale, t)}</span>
            <p>{kind === 'office' ? (isEditable ? t('app.viewer.office_in_server') : t('app.viewer.office_without_server')) : t('app.viewer.cannot_show')}</p>
            <Button tone={isEditable ? 'secondary' : 'primary'} size="sm" icon="download" onclick={() => void download()}>{t('app.media.download')}</Button>
          </div>
        {/if}
      </main>
      <!-- After the stage, so they paint over a page without a layer of their own. -->
      {#if files.length > 1}
        <span class="page prev wide-only"><IconButton icon="chevron-left" label={t('app.viewer.previous')} onclick={() => go(-1)} /></span>
        <span class="page next wide-only"><IconButton icon="chevron-right" label={t('app.viewer.next')} onclick={() => go(1)} /></span>
      {/if}

      <aside class="details" data-open={isDetailsOpen ? '' : undefined} aria-label={t('app.viewer.details')} data-viewer-details>
        <h3>{t('app.viewer.file')}</h3>
        <p>{file.file_name ?? t('app.media.unnamed')}<br />{formatFileSize(file.size, messages.locale, t)} · {kindWord(file)}</p>
        <h3>{t('app.viewer.uploaded')}</h3>
        <p>{accounts.nameOf(file.created_by) ?? '—'} · {formatDay(file.created_at, messages.locale)}</p>
        {#if opened.owner}
          <h3>{t('app.viewer.attached_to')}</h3>
          <p>{#if opened.owner.key}<span class="key">{opened.owner.key}</span> {/if}{opened.owner.title}</p>
        {/if}
        {#if isEditable}
          <h3>{t('app.viewer.editing')}</h3>
          <p>{t('app.viewer.editing_lead')}</p>
          <span class="narrow-only"><Button tone="primary" size="sm" icon="pencil" onclick={() => void edit()}>{t('app.viewer.edit')}</Button></span>
        {/if}
        <span class="narrow-only"><Button tone="secondary" size="sm" icon="link" onclick={() => void copyLink()}>{t('app.viewer.link')}</Button></span>
        {#if notice}<p class="quiet" role="status">{notice}</p>{/if}
        {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
      </aside>
    </div>

    <footer class="foot">
      <ul class="strip wide-only" aria-label={t('app.viewer.files')} data-viewer-strip>
        {#each files as each, i (each.id)}
          <li><button type="button" class="tile" aria-current={i === viewer.index ? 'true' : undefined} title={each.file_name ?? undefined} onclick={() => (viewer.index = i)}>
            {#if markOf(each)}<LabelChip name={markOf(each)} colorToken={markColourOf(each)} />{/if}
            <span class="tile-name">{each.file_name ?? t('app.media.unnamed')}</span>
          </button></li>
        {/each}
      </ul>
      <p class="keys wide-only"><Kbd size="sm">←</Kbd> <Kbd size="sm">→</Kbd> {t('app.viewer.keys_page')} · <Kbd size="sm">Esc</Kbd> {t('app.viewer.keys_close')}</p>
      <div class="phone-bar narrow-only">
        <IconButton icon="chevron-left" label={t('app.viewer.previous')} onclick={() => go(-1)} />
        <span>{t('app.viewer.position', { n: String(viewer.index + 1), m: String(files.length) })} · {t('app.viewer.swipe')}</span>
        <IconButton icon="download" label={t('app.media.download')} onclick={() => void download()} />
        <IconButton icon="chevron-right" label={t('app.viewer.next')} onclick={() => go(1)} />
      </div>
    </footer>
  {/if}
</dialog>

<style>
  .viewer { inline-size: 100vw; block-size: 100dvh; max-inline-size: none; max-block-size: none; margin: 0; padding: 0; border: 0; background: var(--stage-surface); color: var(--stage-ink); }
  .viewer[open] { display: grid; grid-template-rows: auto 1fr auto; }
  .viewer::backdrop { background: var(--bg-scrim); }
  .top { display: flex; align-items: center; gap: var(--sp-100); padding: var(--sp-100) var(--sp-300); border-block-end: var(--bw-hairline) solid var(--stage-border); }
  .name { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .where { white-space: nowrap; opacity: 0.8; }
  .acts { display: flex; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }
  .body { display: grid; grid-template-columns: minmax(0, 1fr) calc(var(--sp-1600) * 2.5); min-block-size: 0; }
  .stage { position: relative; display: grid; justify-items: center; align-content: start; gap: var(--sp-200); padding: var(--sp-300); overflow: auto; min-block-size: 0; }
  .stage[data-kind='image'], .stage[data-kind='video'], .stage[data-kind='svg'], .stage[data-kind='audio'] { align-content: center; }
  .body { position: relative; }
  .page { position: absolute; inset-block-start: 50%; translate: 0 -50%; }
  .prev { inset-inline-start: var(--sp-200); }
  .next { inset-inline-end: calc(var(--sp-1600) * 2.5 + var(--sp-200)); }
  .picture { max-inline-size: 100%; max-block-size: 70dvh; object-fit: contain; }
  .stage[data-zoom='1.5'] .picture { max-inline-size: none; max-block-size: none; inline-size: 150%; }
  .stage[data-zoom='2'] .picture { max-inline-size: none; max-block-size: none; inline-size: 200%; }
  .stage[data-zoom='0.75'] .picture { max-block-size: 50dvh; }
  .pages { display: grid; gap: var(--sp-200); justify-items: center; }
  .sheet { background: var(--bg-surface); color: var(--text-primary); box-shadow: var(--shadow-overlay); max-inline-size: 100%; }
  canvas.sheet { inline-size: min(100%, calc(var(--sp-1600) * 13)); block-size: auto; }
  .stage[data-zoom='1.5'] canvas.sheet { inline-size: calc(var(--sp-1600) * 19); max-inline-size: none; }
  .stage[data-zoom='2'] canvas.sheet { inline-size: calc(var(--sp-1600) * 26); max-inline-size: none; }
  .stage[data-zoom='0.75'] canvas.sheet { inline-size: calc(var(--sp-1600) * 10); }
  .text, .prose, .table-wrap, .listing { inline-size: min(100%, calc(var(--sp-1600) * 13)); padding: var(--sp-400); margin: 0; overflow: auto; }
  .text { white-space: pre-wrap; font-family: var(--font-mono); font-size: var(--fs-090); }
  .csv { border-collapse: collapse; font-size: var(--fs-090); }
  .csv th, .csv td, .prose th, .prose td { border: var(--bw-hairline) solid var(--border-subtle); padding: var(--sp-050) var(--sp-100); text-align: start; }
  .listing ul { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--sp-050); }
  .listing li { display: flex; justify-content: space-between; gap: var(--sp-200); font-family: var(--font-mono); font-size: var(--fs-090); }
  .card { display: grid; justify-items: center; gap: var(--sp-100); padding: var(--sp-500); border-radius: var(--r-lg); background: var(--bg-surface); color: var(--text-primary); text-align: center; max-inline-size: calc(var(--sp-1600) * 7); align-self: center; }
  .card p { margin: 0; }
  .details { padding: var(--sp-300); background: var(--stage-raised); border-inline-start: var(--bw-hairline) solid var(--stage-border); overflow: auto; }
  .details h3 { margin: var(--sp-200) 0 var(--sp-050); font-size: var(--fs-075);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em; text-transform: uppercase; opacity: 0.75; }
  .details h3:first-child { margin-block-start: 0; }
  .details p, .details h3, .where, .keys { color: var(--stage-ink-subtle); }
  .details p { margin: 0; }
  .key { font-family: var(--font-mono); opacity: 0.75; }
  .foot { display: flex; align-items: center; gap: var(--sp-200); padding: var(--sp-100) var(--sp-300); border-block-start: var(--bw-hairline) solid var(--stage-border); }
  .strip { display: flex; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; overflow-x: auto; flex: 1; }
  .tile { display: grid; justify-items: center; gap: var(--sp-050); inline-size: calc(var(--sp-1000) + var(--sp-500)); padding: var(--sp-100) var(--sp-050); border: var(--bw-thick) solid transparent; border-radius: var(--r-md); background: var(--stage-control); color: inherit; font: inherit; font-size: var(--fs-075); cursor: pointer; }
  .tile[aria-current='true'] { border-color: var(--accent-primary); }
  .tile-name { max-inline-size: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .keys { margin: 0; white-space: nowrap; font-size: var(--fs-090); }
  .quiet { opacity: 0.75; }
  .center { align-self: center; }
  .failure { color: var(--text-danger); }
  .narrow-only, .phone-only { display: none; }
  .phone-bar { flex: 1; align-items: center; justify-content: space-between; }

  /* Tablet and phone: the details fold behind ⋯; the phone pages from its foot (§7). */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px) less one; a media query cannot read a custom property. */
  @media (max-width: 904px) {
    .body { grid-template-columns: 1fr; }
    .details { display: none; }
    .next { inset-inline-end: var(--sp-200); }
    .details[data-open] { display: block; position: absolute; inset-inline-end: 0; inset-block: var(--sp-600) var(--sp-600); inline-size: min(100%, calc(var(--sp-1600) * 5)); }
    .narrow-only { display: inline-flex; }
  }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px) less one; a media query cannot read a custom property. */
  @media (max-width: 599px) {
    .wide-only { display: none; }
    .phone-only { display: inline-flex; }
    .top { padding-inline: var(--sp-100); }
    .stage { padding: var(--sp-100); }
    .text, .prose, .table-wrap, .listing { padding: var(--sp-200); }
  }
</style>
