<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The mark that says whose app this is, wherever the header's choice is drawn: the header's home
  // link, the sign-in card, the setting's preview (UC-INS-22 checks 3, 5 and 8; ADR-0112). By the
  // choice the product's `Logo`, the workspace's symbol (or two letters) with its name, or the
  // workspace's logo alone; which picture each surface draws is G8's rule (`markDrawings`).
  //
  // **The theme is followed by the stylesheet, not read.** Where the two surfaces draw differently
  // both drawings stand and the document's theme hides one, so a theme switch shows the other at
  // once with nothing to load, and no component reads the theme (design-system.md §11.2). A caller
  // that draws one surface whatever the theme - the preview - names it.
  //
  // **The box is the approved draft's**: a logo at most the bar's mark height and a small field's
  // width, contained, never stretched; the name in the display face up to a list title's width,
  // then an ellipsis and the whole name as its tooltip. The box stands while a picture loads, so
  // nothing beside it moves, and a picture that fails falls back past itself, never a broken image.
  //
  // **Named once**: the product's wordmark, the workspace's visible name (its symbol decorative),
  // or the workspace's name on the logo that stands alone - never both, never an image unnamed.

  import type { WorkspaceHeader, WorkspacePicture } from '@hubtask/sync-engine';

  import { Logo, VisuallyHidden } from '@hubtask/design-system/components';

  import { PRODUCT_NAME } from '../frame/tabtitle.ts';
  import { PICTURES, isSameDrawing, markDrawings, type Drawing, type PictureSource, type Surface } from './branding.ts';
  import WorkspaceSymbol from './WorkspaceSymbol.svelte';

  interface Props {
    /** The workspace's name; absent where none is known. */
    name?: string;
    header: WorkspaceHeader;
    pictures: Partial<Record<WorkspacePicture, PictureSource>>;
    /** Draw this surface whatever the document's theme. */
    surface?: Surface;
    /** False until the choice is known: the box is kept empty rather than another mark drawn and swapped. */
    isKnown?: boolean;
    /** Inside something that names it already (the preview's bar): nothing here is named. */
    isDecorative?: boolean;
  }

  const { name = '', header, pictures, surface, isKnown = true, isDecorative = false }: Props = $props();

  /** Addresses that failed to load: a replaced picture is a new address and is tried afresh. */
  let failedUrls = $state<readonly string[]>([]);
  const failed = $derived(new Set(PICTURES.filter((picture) => {
    const source = pictures[picture];
    return typeof source === 'string' && failedUrls.includes(source);
  })));
  const drawings = $derived(markDrawings(name ? header : 'PRODUCT', pictures, failed));
  const surfaces = $derived<readonly (Surface | undefined)[]>(
    surface ? [surface] : isSameDrawing(drawings.light, drawings.dark) ? [undefined] : ['light', 'dark'],
  );

  function fail(picture: WorkspacePicture) {
    const source = pictures[picture];
    if (typeof source === 'string' && !failedUrls.includes(source)) failedUrls = [...failedUrls, source];
  }
</script>

{#snippet drawn(drawing: Drawing)}
  {#if drawing.kind === 'product'}
    <Logo size="md" wordmark={PRODUCT_NAME} />
  {:else if drawing.kind === 'name'}
    <span class="named">
      <WorkspaceSymbol {name} source={drawing.symbol ? pictures[drawing.symbol] : undefined} onfail={() => fail('SYMBOL')} />
      <span class="word" title={name}>{name}</span>
    </span>
  {:else if drawing.kind === 'symbol'}
    <span class="alone" role={isDecorative ? undefined : 'img'} aria-label={isDecorative ? undefined : name}>
      <WorkspaceSymbol {name} source={pictures[drawing.picture]} size="lg" onfail={() => fail(drawing.picture)} />
    </span>
  {:else}
    {@const source = pictures[drawing.picture]}
    <span class="logo" role={isDecorative ? undefined : 'img'} aria-label={isDecorative ? undefined : name}>
      {#if typeof source === 'string'}
        <img src={source} alt="" onerror={() => fail(drawing.picture)} />
      {/if}
    </span>
  {/if}
{/snippet}

<span class="mark" aria-hidden={isDecorative ? 'true' : undefined}>
  {#if !isKnown}
    <!-- Not known yet: the bar's height kept, and the link still named. -->
    <span class="waiting"></span>
    {#if !isDecorative}<VisuallyHidden>{PRODUCT_NAME}</VisuallyHidden>{/if}
  {:else}
    {#each surfaces as each (each ?? 'both')}
      <span class="surface" data-surface={surface ? undefined : each}>{@render drawn(each === 'dark' ? drawings.dark : drawings.light)}</span>
    {/each}
  {/if}
</span>

<style>
  .mark { display: inline-flex; align-items: center; min-inline-size: 0; max-inline-size: 100%; }

  .surface { display: inline-flex; align-items: center; min-inline-size: 0; }

  /* The document's theme picks the surface; a mark drawn for one surface carries no data-surface. */
  :global(:root[data-theme='dark']) .surface[data-surface='light'],
  :global(:root:not([data-theme='dark'])) .surface[data-surface='dark'] {
    display: none;
  }

  .waiting { display: inline-block; block-size: var(--sp-400); }

  .logo {
    display: inline-flex;
    align-items: center;
    block-size: var(--sp-400);
    max-inline-size: var(--layout-field-sm);
  }

  .logo img { block-size: 100%; inline-size: auto; max-inline-size: 100%; object-fit: contain; }

  .alone { display: inline-flex; }

  .named { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  .word {
    max-inline-size: var(--layout-list-title);
    overflow: hidden;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-bold);
    line-height: var(--lh-tight);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
