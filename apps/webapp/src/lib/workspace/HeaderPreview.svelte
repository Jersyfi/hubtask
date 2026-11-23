<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One bar of the header preview (UC-INS-22 check 3): the header as it would stand on one surface,
  // under that surface's own theme whatever the reader uses now - the generated tokens select
  // `data-theme` on any element, so the bar is the real colours, not a picture of them. The mark's
  // box is the approved draft's: a logo at most the field's small width and the bar's mark height,
  // contained; the name in the display face up to a list title's width, then an ellipsis.

  import { Icon, Logo } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { initialsOf, type Drawing, type Surface } from './branding.ts';

  interface Props {
    surface: Surface;
    drawing: Drawing;
    /** The local address of each picture that can be drawn now. */
    urls: Readonly<Partial<Record<string, string>>>;
    /** The workspace's name, as typed. */
    name: string;
    caption: string;
  }

  const { surface, drawing, urls, name, caption }: Props = $props();

  const shows = $derived(
    drawing.kind === 'product' ? 'Hubtask' : drawing.kind === 'name' ? name : t('app.branding.preview_logo', { name }),
  );
</script>

<figure class="preview" data-theme={surface} data-preview={surface}>
  <div class="bar" role="img" aria-label={shows}>
    <Icon name="panel-left" />
    {#if drawing.kind === 'product'}
      <Logo size="md" wordmark="Hubtask" />
    {:else if drawing.kind === 'logo'}
      <span class="logo">{#if urls[drawing.picture]}<img src={urls[drawing.picture]} alt="" />{/if}</span>
    {:else if drawing.kind === 'symbol'}
      <span class="symbol" data-size="lg">{#if urls[drawing.picture]}<img src={urls[drawing.picture]} alt="" />{/if}</span>
    {:else}
      <span class="named">
        <span class="symbol">
          {#if drawing.symbol && urls[drawing.symbol]}<img src={urls[drawing.symbol]} alt="" />{:else}{initialsOf(name)}{/if}
        </span>
        <span class="word" title={name}>{name}</span>
      </span>
    {/if}
  </div>
  <figcaption class="caption">{caption}</figcaption>
</figure>

<style>
  .preview {
    margin: 0;
    min-inline-size: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    overflow: hidden;
    background: var(--bg-canvas);
    color: var(--text-primary);
  }

  .bar {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
    block-size: var(--layout-appbar-height);
    padding-inline: var(--sp-150);
    background: var(--bg-surface);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    overflow: hidden;
  }

  .logo {
    display: inline-flex;
    align-items: center;
    block-size: var(--sp-400);
    max-inline-size: var(--layout-field-sm);
  }

  .logo img { block-size: 100%; inline-size: auto; max-inline-size: 100%; object-fit: contain; }

  .symbol {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    border-radius: var(--r-sm);
    overflow: hidden;
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    line-height: 1;
  }

  .symbol[data-size='lg'] { inline-size: var(--sp-400); block-size: var(--sp-400); border-radius: var(--r-md); font-size: var(--fs-075); }

  .symbol img { inline-size: 100%; block-size: 100%; object-fit: contain; }

  .named { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  .word {
    max-inline-size: var(--layout-list-title);
    overflow: hidden;
    color: var(--text-primary);
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-bold);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .caption { padding: var(--sp-050) var(--sp-150); color: var(--text-subtle); font-size: var(--fs-065); }
</style>
