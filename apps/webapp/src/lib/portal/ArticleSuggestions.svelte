<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The articles a service publishes for the words typed so far (UC-DOC-13 check 3): offered
  // beside the request being written, so a question answered already need not be asked. Keyed by
  // the caller on the words, so each set of words is one read.

  import { untrack } from 'svelte';
  import { ListRow, Stack } from '@hubtask/design-system/components';
  import type { ArticleSuggestionList } from '@hubtask/sync-engine';

  import { suggestionsPath } from '../data/portal.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  const { collectionId, text, onopen }: { collectionId: string; text: string; onopen: (pageId: string) => void } = $props();

  const found = untrack(() => resource<ArticleSuggestionList>(suggestionsPath(collectionId, text)));
  const rows = $derived(found.state.status === 'ready' ? found.state.data.data : []);
</script>

{#if rows.length > 0}
  <section class="suggestions" aria-labelledby="portal-suggestions" aria-live="polite">
    <Stack gap="050">
      <h3 class="heading" id="portal-suggestions">{text === '' ? t('app.portal.articles_read') : t('app.portal.articles_found')}</h3>
      {#each rows as row (row.page_id)}
        <ListRow href={`/portal/${collectionId}/articles/${row.page_id}`} onactivate={() => onopen(row.page_id)}>
          {#if row.icon}<span aria-hidden="true">{row.icon}</span>{/if}
          {row.title}
          {#if row.excerpt}<span class="excerpt">{row.excerpt}</span>{/if}
        </ListRow>
      {/each}
    </Stack>
  </section>
{/if}

<style>
  .suggestions { padding: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); }
  .heading { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); }
  .excerpt { display: block; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
