<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The articles every department one may ask offers for the words (PO-1): each portal's own
  // suggestions (UC-DOC-13 check 2), merged, the department named beside each. Keyed by the
  // caller on the words, so each set of words is one read per portal.

  import { untrack } from 'svelte';
  import { Icon } from '@hubtask/design-system/components';
  import type { ArticleSuggestionList } from '@hubtask/sync-engine';

  import { suggestionsPath } from '../data/portal.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { PortalPlace } from './portal.ts';

  const { portals, text, onopen }: { portals: readonly PortalPlace[]; text: string; onopen: (path: string) => void } = $props();

  const asked = untrack(() => portals.map((each) => ({ portal: each, found: resource<ArticleSuggestionList>(suggestionsPath(each.collection_id, text)) })));
  const rows = $derived(asked.flatMap(({ portal, found }) =>
    found.state.status === 'ready' ? found.state.data.data.slice(0, 3).map((hit) => ({ portal, hit })) : []).slice(0, 6));
</script>

{#if rows.length > 0}
  <p class="group">{t('app.portal_dir.found_knowledge')}</p>
  {#each rows as { portal, hit } (`${portal.collection_id}/${hit.page_id}`)}
    {@const path = `/portal/${portal.collection_id}/articles/${hit.page_id}`}
    <a class="hit" href={path} data-portal-dir-article={hit.page_id} onclick={(event) => { event.preventDefault(); onopen(path); }}>
      <Icon name="book-open" size="sm" />
      <span lang={hit.content_language ?? undefined}>{hit.title}</span><span class="where">· {portal.name}</span>
    </a>
  {/each}
{/if}

<style>
  .group { margin: var(--sp-100) 0 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); color: var(--text-secondary); text-transform: uppercase; }
  .hit { display: flex; align-items: center; gap: var(--sp-100); padding: var(--sp-100); border-radius: var(--r-sm); color: inherit; text-decoration: none; }
  .hit:hover, .hit:focus-visible { background: var(--accent-primary-subtle); }
  .where { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
