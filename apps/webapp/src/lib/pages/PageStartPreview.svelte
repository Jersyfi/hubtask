<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A start's preview beside the new page (scene pnew 4): what it is, its text drawn as the page
  // will draw it, and the one act that uses it.
  import { Button, IconButton } from '@hubtask/design-system/components';
  import { t } from '../i18n/i18n.svelte.ts';
  import PageVersionText from './PageVersionText.svelte';

  interface Props {
    title: string;
    icon?: string | null;
    lead: string;
    body: string;
    /** Further pages a whole space's start makes beneath the page. */
    subpages?: number;
    isBusy?: boolean;
    onuse: () => void;
    onclose: () => void;
  }

  const { title, icon, lead, body, subpages = 0, isBusy = false, onuse, onclose }: Props = $props();
</script>

<section class="preview" aria-labelledby="page-start-preview" data-page-start-preview>
  <header class="head">
    <h2 class="name" id="page-start-preview">{#if icon}<span aria-hidden="true">{icon}</span> {/if}{title}</h2>
    <IconButton icon="x" size="sm" tone="secondary" label={t('app.pages.start_close')} onclick={onclose} />
  </header>
  <p class="lead">{lead}</p>
  <div class="sample">
    {#key body}<PageVersionText {body} label={t('app.pages.start_preview', { title })} />{/key}
  </div>
  {#if subpages > 0}<p class="lead">{t('app.pages.space_pages', { count: String(subpages) })}</p>{/if}
  <div><Button size="sm" tone="secondary" {isBusy} data-page-start-use onclick={onuse}>{t('app.pages.start_use')}</Button></div>
</section>

<style>
  .preview {
    display: grid; gap: var(--sp-150); padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-raised);
  }
  .head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .name { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .lead { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .sample { padding: var(--sp-150); border-radius: var(--r-md); background: var(--bg-surface-sunken); font-size: var(--fs-075); }
</style>
