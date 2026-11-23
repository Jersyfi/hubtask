<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One bar of the header preview (UC-INS-22 check 3): the header as it would stand on one surface,
  // under that surface's own theme whatever the reader uses now - the generated tokens select
  // `data-theme` on any element, so the bar is the real colours, not a picture of them. The mark is
  // the header's own (`WorkspaceMark`), drawn for this surface; the bar names what it shows.

  import type { WorkspaceHeader, WorkspacePicture } from '@hubtask/sync-engine';

  import { Icon } from '@hubtask/design-system/components';

  import { PRODUCT_NAME } from '../frame/tabtitle.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { markDrawings, type PictureSource, type Surface } from './branding.ts';
  import WorkspaceMark from './WorkspaceMark.svelte';

  interface Props {
    surface: Surface;
    header: WorkspaceHeader;
    /** Each present picture's local address, `null` until it can be drawn. */
    pictures: Partial<Record<WorkspacePicture, PictureSource>>;
    /** The workspace's name, as typed. */
    name: string;
    caption: string;
  }

  const { surface, header, pictures, name, caption }: Props = $props();

  const kind = $derived(markDrawings(name ? header : 'PRODUCT', pictures, new Set())[surface].kind);
  const shows = $derived(kind === 'product' ? PRODUCT_NAME : kind === 'name' ? name : t('app.branding.preview_logo', { name }));
</script>

<figure class="preview" data-theme={surface} data-preview={surface}>
  <div class="bar" role="img" aria-label={shows}>
    <Icon name="panel-left" />
    <WorkspaceMark {name} {header} {pictures} {surface} isDecorative />
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

  .caption { padding: var(--sp-050) var(--sp-150); color: var(--text-subtle); font-size: var(--fs-065); }
</style>
