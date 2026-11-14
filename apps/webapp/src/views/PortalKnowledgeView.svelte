<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a portal shares of the knowledge base (UC-DOC-13 check 5, PO-4, scene `portalext` 3):
  // each space with its pages as an outline, each single article as a card. Only what the server
  // answers the requester - a restricted page and what lies under it never reach here.

  import { untrack } from 'svelte';
  import { Icon, PageHeader, Skeleton, Stack, WorkPage } from '@hubtask/design-system/components';
  import type { PortalKnowledge, RequestCatalogue } from '@hubtask/sync-engine';

  import { cataloguePath, knowledgePath } from '../lib/data/portal.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import FailureState from '../lib/frame/FailureState.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { spaceOutline } from '../lib/portal/portal.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ProductEmpty from '../lib/product/ProductEmpty.svelte';

  const { collectionId, onnavigate }: { collectionId: string; onnavigate?: (path: string) => void } = $props();

  const catalogue = untrack(() => resource<RequestCatalogue>(cataloguePath(collectionId)));
  const shared = untrack(() => resource<PortalKnowledge>(knowledgePath(collectionId)));
  const deskName = $derived(catalogue.state.status === 'ready' ? catalogue.state.data.desk?.name ?? '' : '');
  const spaces = $derived(shared.state.status === 'ready' ? shared.state.data.data : []);
  const failure = $derived(shared.state.status === 'failed' ? renderProblem(shared.state.error, messages) : undefined);
  $effect(() => page.entitle(t('app.portal.knowledge')));

  const articleHref = (pageId: string) => `/portal/${collectionId}/articles/${pageId}`;
  const go = (path: string) => (event: MouseEvent) => {
    if (!onnavigate) return;
    event.preventDefault();
    onnavigate(path);
  };
</script>

<WorkPage>
  {#snippet top()}
    <PageHeader title={t('app.portal.knowledge')} subtitle={deskName ? t('app.portal.knowledge_for', { name: deskName }) : undefined} isTitleInBar={viewport.isCompact} />
  {/snippet}
  {#if failure}
    <FailureState problem={failure} onRetry={() => void shared.refresh()} />
  {:else if shared.state.status !== 'ready'}
    <div aria-busy="true"><Skeleton lines={5} /></div>
  {:else if spaces.length === 0}
    <ProductEmpty product="PAGES" kind="unused" title={t('app.portal.knowledge_none')} />
  {:else}
    <ul class="spaces" data-portal-spaces>
      {#each spaces as space (space.page_id)}
        <li class="card" data-portal-space={space.page_id}>
          <Stack gap="100">
            <a class="title" href={articleHref(space.page_id)} onclick={go(articleHref(space.page_id))}>
              <span aria-hidden="true">{#if space.icon}{space.icon}{:else}<Icon name="book-open" size="sm" />{/if}</span>
              {space.title}
            </a>
            <span class="sub">{space.with_subpages ? t('app.portal.space_count', { count: String(space.pages.length + 1) }) : t('app.portal.article_single')}</span>
            {#if space.pages.length > 0}
              <ul class="outline">
                {#each spaceOutline(space.page_id, space.pages).slice(0, 12) as row (row.page.page_id)}
                  <li data-depth={row.depth > 1 ? '' : undefined}>
                    <a href={articleHref(row.page.page_id)} onclick={go(articleHref(row.page.page_id))}>{row.page.icon ? `${row.page.icon} ` : ''}{row.page.title}</a>
                  </li>
                {/each}
              </ul>
              {#if space.pages.length > 12}
                <a class="sub" href={articleHref(space.page_id)} onclick={go(articleHref(space.page_id))}>{t('app.portal.space_more', { count: String(space.pages.length - 12) })}</a>
              {/if}
            {/if}
          </Stack>
        </li>
      {/each}
    </ul>
  {/if}
</WorkPage>

<style>
  .spaces { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--sp-200); grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-list-title)), 1fr)); }
  .card { padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); }
  .title { display: inline-flex; gap: var(--sp-100); align-items: center; font-weight: var(--fw-semibold); color: inherit; text-decoration: none; }
  .title:hover { text-decoration: underline; }
  .sub { color: var(--text-secondary); font-size: var(--fs-075); }
  .outline { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-050); font-size: var(--fs-075); }
  .outline li[data-depth] { padding-inline-start: var(--sp-200); }
</style>
