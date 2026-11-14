<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An article a service publishes, read in its portal (UC-DOC-13 checks 2 to 4): the page as
  // it stands now, in the reader's language where the server found a variant of it, and whether
  // it helped - one answer per reader, which the publisher counts and never names. The Markdown is
  // shown as written, never as markup a page could inject. A page of a shared space (PO-4) reads
  // with the space's outline beside it and its path above (scene `portalext` 5).

  import { untrack } from 'svelte';
  import { Breadcrumb, Button, PageHeader, DetailPage, Skeleton } from '@hubtask/design-system/components';
  import FailureState from '../lib/frame/FailureState.svelte';
  import type { Article, PortalKnowledge } from '@hubtask/sync-engine';

  import { articlePath, knowledgePath, portal } from '../lib/data/portal.svelte.ts';
  import { articleText, spaceOutline } from '../lib/portal/portal.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const { collectionId, pageId, onnavigate }: { collectionId: string; pageId: string; onnavigate?: (path: string) => void } = $props();

  const article = untrack(() => resource<Article>(articlePath(collectionId, pageId)));
  const read = $derived(article.state.status === 'ready' ? article.state.data : undefined);
  const failure = $derived(article.state.status === 'failed' ? renderProblem(article.state.error, messages) : undefined);
  $effect(() => page.entitle(read?.title ?? t('app.portal.article')));

  const shared = untrack(() => resource<PortalKnowledge>(knowledgePath(collectionId)));
  // The space this page belongs to: its root, or one of the pages under it.
  const space = $derived(shared.state.status === 'ready'
    ? shared.state.data.data.find((each) => each.with_subpages && (each.page_id === pageId || each.pages.some((p) => p.page_id === pageId)))
    : undefined);
  const outline = $derived(space ? spaceOutline(space.page_id, space.pages) : []);
  // The path from the space down to this page, for the crumbs.
  const trail = $derived.by(() => {
    if (!space) return [];
    const byId = new Map(space.pages.map((p) => [p.page_id, p]));
    const out: { id: string; title: string }[] = [];
    for (let at = byId.get(pageId); at && out.length < 16; at = byId.get(at.parent_id)) out.unshift({ id: at.page_id, title: at.title });
    return [{ id: space.page_id, title: space.title }, ...out.filter((each) => each.id !== space.page_id)];
  });
  const href = (id: string) => `/portal/${collectionId}/articles/${id}`;

  let answered = $state<boolean | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);

  async function rate(helpful: boolean) {
    refusal = undefined;
    try {
      await portal.rateArticle(collectionId, pageId, helpful);
      answered = helpful;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }
</script>

<DetailPage>
  {#snippet top()}
    {#if viewport.isCompact || !read}
      <PageHeader title={read ? `${read.icon ? `${read.icon} ` : ''}${read.title}` : t('app.portal.article')} isTitleInBar={viewport.isCompact} />
    {/if}
  {/snippet}
  {#if failure}
    <FailureState problem={failure} onRetry={() => void article.refresh()} />
  {:else if !read}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <!-- Scene `portalext` 5: the space's outline beside one column of crumbs, title, text and the
         question whether it helped. -->
    <div class="read" data-portal-article data-spaced={space ? '' : undefined}>
      {#if space}
        <nav class="outline" aria-label={t('app.portal.space_outline', { name: space.title })} data-portal-outline>
          <a class="root" href={href(space.page_id)} aria-current={space.page_id === pageId ? 'page' : undefined}
            onclick={(event) => { event.preventDefault(); onnavigate?.(href(space.page_id)); }}>{space.icon ? `${space.icon} ` : ''}{space.title}</a>
          <ul>
            {#each outline as row (row.page.page_id)}
              <li data-depth={String(Math.min(row.depth, 4))}>
                <a href={href(row.page.page_id)} aria-current={row.page.page_id === pageId ? 'page' : undefined}
                  onclick={(event) => { event.preventDefault(); onnavigate?.(href(row.page.page_id)); }}>{row.page.title}</a>
              </li>
            {/each}
          </ul>
        </nav>
      {/if}
      <div class="column">
        <Breadcrumb label={t('app.portal.article_path')} expandLabel={t('app.portal.article_path_more')} trail={[
          { id: `/portal/${collectionId}/knowledge`, label: t('app.portal.knowledge'), href: `/portal/${collectionId}/knowledge` },
          ...trail.slice(0, -1).map((each) => ({ id: href(each.id), label: each.title, href: href(each.id) })),
          { id: 'here', label: read.title },
        ]} onnavigate={(path) => onnavigate?.(path)} />
        {#if !viewport.isCompact}<h1 class="title">{read.icon ? `${read.icon} ` : ''}{read.title}</h1>{/if}
        <article class="body" lang={read.content_language ?? undefined}>{articleText(read.body)}</article>
        <section class="helped" aria-labelledby="article-helped">
          <h2 class="ask" id="article-helped">{t('app.portal.helped')}</h2>
          <div class="answers">
            <Button tone="secondary" onclick={() => void rate(true)}>{t('app.portal.yes')}</Button>
            <Button tone="secondary" onclick={() => void rate(false)}>{t('app.portal.no')}</Button>
            <Button tone="primary" onclick={() => onnavigate?.(`/portal/${collectionId}`)}>{t('app.portal.ask_anyway')}</Button>
          </div>
        </section>
        {#if answered !== undefined}
          <p class="quiet" role="status">{answered ? t('app.portal.helped_yes') : t('app.portal.helped_no')}</p>
        {/if}
        {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
      </div>
    </div>
  {/if}
</DetailPage>

<style>
  .column { display: flex; flex-direction: column; gap: var(--sp-200); min-inline-size: 0; max-inline-size: 72ch; }
  .title { margin: 0; font-size: var(--fs-500); font-weight: var(--fw-bold); }
  .helped { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); margin-block-start: var(--sp-200); }
  .ask { margin: 0; flex: 1; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-regular); }
  .answers { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .body { margin: 0; max-inline-size: 72ch; white-space: pre-wrap; overflow-wrap: anywhere; }
  .read { display: grid; gap: var(--sp-300); grid-template-columns: minmax(0, 1fr); container-type: inline-size; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (min-width: 600px) {
    .read[data-spaced] { grid-template-columns: minmax(var(--layout-field-sm), var(--layout-list-title)) minmax(0, 1fr); }
  }
  .outline { display: flex; flex-direction: column; gap: var(--sp-050); font-size: var(--fs-090); }
  .outline ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: var(--sp-025); }
  .outline a { display: block; padding: var(--sp-050) var(--sp-100); border-radius: var(--r-sm); color: inherit; text-decoration: none; }
  .outline a:hover { background: var(--bg-surface-hover); }
  .outline a[aria-current='page'] { background: var(--accent-primary-subtle); font-weight: var(--fw-semibold); }
  .outline .root { font-weight: var(--fw-semibold); }
  .outline li[data-depth='2'] { padding-inline-start: var(--sp-200); }
  .outline li[data-depth='3'], .outline li[data-depth='4'] { padding-inline-start: var(--sp-400); }
</style>
